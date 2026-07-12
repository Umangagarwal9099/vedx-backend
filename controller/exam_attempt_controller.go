package controller

import (
	"errors"
	"hash/fnv"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type ExamAttemptController struct {
	attemptRepo      *repository.ExamAttemptRepository
	assessmentRepo   *repository.AssessmentRepository
	questionBankRepo *repository.QuestionBankRepository
}

func NewExamAttemptController(attemptRepo *repository.ExamAttemptRepository, assessmentRepo *repository.AssessmentRepository, questionBankRepo *repository.QuestionBankRepository) *ExamAttemptController {
	return &ExamAttemptController{attemptRepo: attemptRepo, assessmentRepo: assessmentRepo, questionBankRepo: questionBankRepo}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func isAutoGradable(questionType string) bool {
	switch questionType {
	case "mcq", "multi_select", "true_false", "fill_blank":
		return true
	default:
		return false
	}
}

func effectiveMarks(aq models.AssessmentQuestion) int {
	if aq.MarksOverride != nil {
		return *aq.MarksOverride
	}
	return aq.Marks
}

func wasAttempted(ans models.StudentAnswer) bool {
	return len(ans.SelectedOptionIDs) > 0 || strings.TrimSpace(ans.TextAnswer) != ""
}

func sortedEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aCopy, bCopy := append([]string{}, a...), append([]string{}, b...)
	sort.Strings(aCopy)
	sort.Strings(bCopy)
	for i := range aCopy {
		if aCopy[i] != bCopy[i] {
			return false
		}
	}
	return true
}

// gradeObjectiveAnswer auto-grades mcq/multi_select/true_false/fill_blank answers.
func gradeObjectiveAnswer(q models.AssessmentQuestion, ans models.StudentAnswer, negativeMarking bool) (isCorrect bool, marks int) {
	switch q.QuestionType {
	case "mcq", "true_false":
		isCorrect = len(ans.SelectedOptionIDs) == 1 && len(q.CorrectOptionIDs) == 1 && ans.SelectedOptionIDs[0] == q.CorrectOptionIDs[0]
	case "multi_select":
		isCorrect = sortedEqual(ans.SelectedOptionIDs, q.CorrectOptionIDs)
	case "fill_blank":
		isCorrect = ans.TextAnswer != "" && strings.EqualFold(strings.TrimSpace(ans.TextAnswer), strings.TrimSpace(q.CorrectText))
	}

	marksVal := effectiveMarks(q)
	if isCorrect {
		return true, marksVal
	}
	if negativeMarking && wasAttempted(ans) {
		return false, -q.NegativeMarks
	}
	return false, 0
}

func shuffleStrings(ids []string) {
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
}

// seededShuffleOptions deterministically shuffles a question's options based on
// the attempt+question pair, so the order is stable across repeated fetches
// within the same attempt without needing to persist it.
func seededShuffleOptions(options []models.QuestionOption, seed string) []models.QuestionOption {
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	src := rand.New(rand.NewSource(int64(h.Sum64())))
	shuffled := make([]models.QuestionOption, len(options))
	copy(shuffled, options)
	src.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	return shuffled
}

// buildQuestionViews assembles the ordered, sanitized (or revealed) question
// list for an attempt.
func buildQuestionViews(attempt *models.ExamAttempt, order []string, questions []models.AssessmentQuestion, answers []models.StudentAnswer, reveal bool) []models.AttemptQuestionView {
	byID := make(map[string]models.AssessmentQuestion, len(questions))
	for _, q := range questions {
		byID[q.ShortID] = q
	}
	answerByQuestion := make(map[string]models.StudentAnswer, len(answers))
	for _, a := range answers {
		answerByQuestion[a.QuestionShortID] = a
	}

	views := make([]models.AttemptQuestionView, 0, len(order))
	for _, qid := range order {
		q, ok := byID[qid]
		if !ok {
			continue
		}
		options := q.Options
		if len(options) > 0 {
			options = seededShuffleOptions(options, attempt.ShortID+":"+q.ShortID)
		}
		view := models.AttemptQuestionView{
			ShortID:               q.ShortID,
			QuestionType:          q.QuestionType,
			QuestionText:          q.QuestionText,
			Options:               options,
			Marks:                 effectiveMarks(q),
			CodingQuestionShortID: q.CodingQuestionShortID,
			CodingQuestionTitle:   q.CodingQuestionTitle,
		}
		if a, ok := answerByQuestion[qid]; ok {
			aCopy := a
			view.MyAnswer = &aCopy
		}
		if reveal {
			view.CorrectOptionIDs = q.CorrectOptionIDs
			view.CorrectText = q.CorrectText
			view.Explanation = q.Explanation
		}
		views = append(views, view)
	}
	return views
}

// ── Handlers ─────────────────────────────────────────────────────────────────

// StartAttempt godoc
//
//	@Summary		Start (or resume) an exam attempt
//	@Description	Starts a new timed attempt at an assessment, or resumes the caller's existing in-progress attempt. Enforces the assessment's start/end window and max_attempts. Returns the attempt with its (possibly randomized) question list, sanitized of correct answers.
//	@Tags			exam-attempts
//	@Produce		json
//	@Param			short_id	path		string	true	"Assessment short ID"
//	@Success		201			{object}	models.AttemptDetail
//	@Failure		400			{object}	map[string]string	"Outside the exam window, or attempts exhausted"
//	@Failure		404			{object}	map[string]string	"Assessment not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id}/attempts [post]
func (ctrl *ExamAttemptController) StartAttempt(c *gin.Context) {
	assessmentShortID := c.Param("short_id")
	studentID := c.GetString("user_id")

	assessment, err := ctrl.assessmentRepo.FindByShortID(c.Request.Context(), assessmentShortID)
	if err != nil || assessment == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assessment not found"})
		return
	}

	now := time.Now()
	if assessment.StartAt != nil && now.Before(*assessment.StartAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this assessment has not started yet"})
		return
	}
	if assessment.EndAt != nil && now.After(*assessment.EndAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this assessment's window has closed"})
		return
	}

	if existing, err := ctrl.attemptRepo.FindInProgressAttempt(c.Request.Context(), assessmentShortID, studentID); err == nil && existing != nil {
		ctrl.respondAttemptDetail(c, existing, false)
		return
	}

	pastAttempts, err := ctrl.attemptRepo.FindMyAttempts(c.Request.Context(), assessmentShortID, studentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not check previous attempts"})
		return
	}
	hasPassed := false
	for _, a := range pastAttempts {
		if a.Passed != nil && *a.Passed {
			hasPassed = true
		}
	}
	if hasPassed && !assessment.AllowAttemptsAfterPassing {
		c.JSON(http.StatusBadRequest, gin.H{"error": "you've already passed this assessment"})
		return
	}
	maxAttempts := assessment.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if len(pastAttempts) >= maxAttempts {
		c.JSON(http.StatusBadRequest, gin.H{"error": "maximum attempts reached"})
		return
	}

	questions, err := ctrl.questionBankRepo.GetQuestions(c.Request.Context(), assessmentShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch questions"})
		return
	}
	if len(questions) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this assessment has no questions yet"})
		return
	}

	order := make([]string, len(questions))
	maxScore := 0
	for i, q := range questions {
		order[i] = q.ShortID
		maxScore += effectiveMarks(q)
	}
	if assessment.RandomizeQuestions {
		shuffleStrings(order)
	}

	attempt, err := ctrl.attemptRepo.CreateAttempt(c.Request.Context(), assessmentShortID, studentID, len(pastAttempts)+1, maxScore, order)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not start attempt: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, models.AttemptDetail{
		ExamAttempt: *attempt,
		Questions:   buildQuestionViews(attempt, order, questions, nil, false),
	})
}

// respondAttemptDetail builds and returns the full detail view for an attempt.
func (ctrl *ExamAttemptController) respondAttemptDetail(c *gin.Context, attempt *models.ExamAttempt, notFoundIfMissing bool) {
	assessment, err := ctrl.assessmentRepo.FindByShortID(c.Request.Context(), attempt.AssessmentShortID)
	if err != nil || assessment == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not resolve assessment"})
		return
	}
	order, err := ctrl.attemptRepo.GetQuestionOrder(c.Request.Context(), attempt.ShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch question order"})
		return
	}
	questions, err := ctrl.questionBankRepo.GetQuestions(c.Request.Context(), attempt.AssessmentShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch questions"})
		return
	}
	answers, err := ctrl.attemptRepo.GetAnswers(c.Request.Context(), attempt.ShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch answers"})
		return
	}

	reveal := attempt.Status != "in_progress" && assessment.ShowCorrectAnswers
	status := http.StatusOK
	if notFoundIfMissing {
		status = http.StatusCreated
	}
	c.JSON(status, models.AttemptDetail{
		ExamAttempt: *attempt,
		Questions:   buildQuestionViews(attempt, order, questions, answers, reveal),
	})
}

// SubmitAnswer godoc
//
//	@Summary		Autosave an answer
//	@Description	Saves (or overwrites) the caller's answer to one question within their in-progress attempt. Call this on every change — it's cheap to call repeatedly.
//	@Tags			exam-attempts
//	@Accept			json
//	@Produce		json
//	@Param			short_id			path	string							true	"Assessment short ID"
//	@Param			attempt_short_id	path	string							true	"Attempt short ID"
//	@Param			body				body	models.SubmitAnswerInput	true	"Answer"
//	@Success		204					"No Content"
//	@Failure		400					{object}	map[string]string	"Validation error, or attempt is no longer in progress"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id}/attempts/{attempt_short_id}/answers [post]
func (ctrl *ExamAttemptController) SubmitAnswer(c *gin.Context) {
	attemptShortID := c.Param("attempt_short_id")

	var input models.SubmitAnswerInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.attemptRepo.UpsertAnswer(c.Request.Context(), attemptShortID, input.QuestionShortID, input.SelectedOptionIDs, input.TextAnswer); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save answer: " + err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// SubmitAttempt godoc
//
//	@Summary		Submit an attempt
//	@Description	Finalizes the caller's attempt. Objective questions (mcq/multi_select/true_false/fill_blank) are auto-graded immediately; if the assessment has no other question types, the attempt is fully evaluated right away. Otherwise it's marked "submitted" pending manual grading of descriptive/coding answers.
//	@Tags			exam-attempts
//	@Produce		json
//	@Param			short_id			path		string	true	"Assessment short ID"
//	@Param			attempt_short_id	path		string	true	"Attempt short ID"
//	@Success		200					{object}	models.AttemptDetail
//	@Failure		404					{object}	map[string]string	"Attempt not found"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id}/attempts/{attempt_short_id}/submit [post]
func (ctrl *ExamAttemptController) SubmitAttempt(c *gin.Context) {
	attemptShortID := c.Param("attempt_short_id")

	attempt, err := ctrl.attemptRepo.FindAttemptByShortID(c.Request.Context(), attemptShortID)
	if err != nil || attempt == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "attempt not found"})
		return
	}

	assessment, err := ctrl.assessmentRepo.FindByShortID(c.Request.Context(), attempt.AssessmentShortID)
	if err != nil || assessment == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not resolve assessment"})
		return
	}

	if err := ctrl.attemptRepo.MarkSubmitted(c.Request.Context(), attemptShortID, false); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not submit attempt"})
			return
		}
		// already submitted — fall through to (re)grade idempotently
	}

	if err := ctrl.gradeObjectiveQuestions(c, attempt, assessment); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not grade attempt: " + err.Error()})
		return
	}

	updated, err := ctrl.attemptRepo.FindAttemptByShortID(c.Request.Context(), attemptShortID)
	if err != nil || updated == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch submitted attempt"})
		return
	}
	ctrl.respondAttemptDetail(c, updated, false)
}

func (ctrl *ExamAttemptController) gradeObjectiveQuestions(c *gin.Context, attempt *models.ExamAttempt, assessment *models.Assessment) error {
	questions, err := ctrl.questionBankRepo.GetQuestions(c.Request.Context(), attempt.AssessmentShortID)
	if err != nil {
		return err
	}
	answers, err := ctrl.attemptRepo.GetAnswers(c.Request.Context(), attempt.ShortID)
	if err != nil {
		return err
	}
	answerByQuestion := make(map[string]models.StudentAnswer, len(answers))
	for _, a := range answers {
		answerByQuestion[a.QuestionShortID] = a
	}

	anyManual := false
	for _, q := range questions {
		if !isAutoGradable(q.QuestionType) {
			anyManual = true
			continue
		}
		ans := answerByQuestion[q.ShortID] // zero value if unanswered — treated as incorrect/unattempted
		isCorrect, marks := gradeObjectiveAnswer(q, ans, assessment.NegativeMarking)
		if err := ctrl.attemptRepo.SetAnswerGrade(c.Request.Context(), attempt.ShortID, q.ShortID, &isCorrect, marks, ""); err != nil {
			return err
		}
	}

	if anyManual {
		return nil // leave status "submitted" — pending manual grading
	}

	total, err := ctrl.attemptRepo.SumMarks(c.Request.Context(), attempt.ShortID)
	if err != nil {
		return err
	}
	passed := attempt.MaxScore > 0 && float64(total)/float64(attempt.MaxScore)*100 >= assessment.PassingPercentage
	return ctrl.attemptRepo.FinalizeAttempt(c.Request.Context(), attempt.ShortID, total, passed)
}

// GetMyAttempts godoc
//
//	@Summary		List my attempts
//	@Description	Returns the calling student's attempts at an assessment, newest first.
//	@Tags			exam-attempts
//	@Produce		json
//	@Param			short_id	path		string	true	"Assessment short ID"
//	@Success		200			{array}		models.ExamAttempt
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id}/attempts/me [get]
func (ctrl *ExamAttemptController) GetMyAttempts(c *gin.Context) {
	assessmentShortID := c.Param("short_id")
	studentID := c.GetString("user_id")

	attempts, err := ctrl.attemptRepo.FindMyAttempts(c.Request.Context(), assessmentShortID, studentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch attempts"})
		return
	}
	if attempts == nil {
		attempts = []models.ExamAttempt{}
	}
	c.JSON(http.StatusOK, attempts)
}

// GetAttempt godoc
//
//	@Summary		Get attempt detail
//	@Description	Returns a single attempt with its full question list. Correct answers are only included once the attempt is finished and the assessment allows revealing them.
//	@Tags			exam-attempts
//	@Produce		json
//	@Param			short_id			path		string	true	"Assessment short ID"
//	@Param			attempt_short_id	path		string	true	"Attempt short ID"
//	@Success		200					{object}	models.AttemptDetail
//	@Failure		404					{object}	map[string]string	"Attempt not found"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id}/attempts/{attempt_short_id} [get]
func (ctrl *ExamAttemptController) GetAttempt(c *gin.Context) {
	attemptShortID := c.Param("attempt_short_id")
	attempt, err := ctrl.attemptRepo.FindAttemptByShortID(c.Request.Context(), attemptShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch attempt"})
		return
	}
	if attempt == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "attempt not found"})
		return
	}
	ctrl.respondAttemptDetail(c, attempt, false)
}

// GetAllAttempts godoc
//
//	@Summary		List all attempts
//	@Description	Returns every attempt at an assessment across all students, for monitoring and grading. Restricted to super_admin / team_lead / mentor.
//	@Tags			exam-attempts
//	@Produce		json
//	@Param			short_id	path		string	true	"Assessment short ID"
//	@Success		200			{array}		models.ExamAttempt
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id}/attempts [get]
func (ctrl *ExamAttemptController) GetAllAttempts(c *gin.Context) {
	assessmentShortID := c.Param("short_id")
	attempts, err := ctrl.attemptRepo.FindAllAttempts(c.Request.Context(), assessmentShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch attempts"})
		return
	}
	if attempts == nil {
		attempts = []models.ExamAttempt{}
	}
	c.JSON(http.StatusOK, attempts)
}

// GradeAnswer godoc
//
//	@Summary		Manually grade an answer
//	@Description	Records marks and feedback for a short_answer/descriptive/coding answer within a submitted attempt. Once every answer in the attempt has a grade, the attempt automatically flips to "evaluated" with its total score and pass/fail computed. Restricted to super_admin / team_lead / mentor.
//	@Tags			exam-attempts
//	@Accept			json
//	@Produce		json
//	@Param			short_id			path	string							true	"Assessment short ID"
//	@Param			attempt_short_id	path	string							true	"Attempt short ID"
//	@Param			question_short_id	path	string							true	"Question short ID"
//	@Param			body				body	models.GradeAnswerInput	true	"Grade details"
//	@Success		204					"No Content"
//	@Failure		400					{object}	map[string]string	"Validation error"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id}/attempts/{attempt_short_id}/answers/{question_short_id}/grade [patch]
func (ctrl *ExamAttemptController) GradeAnswer(c *gin.Context) {
	attemptShortID := c.Param("attempt_short_id")
	questionShortID := c.Param("question_short_id")

	var input models.GradeAnswerInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	isCorrect := input.MarksAwarded > 0
	if err := ctrl.attemptRepo.SetAnswerGrade(c.Request.Context(), attemptShortID, questionShortID, &isCorrect, input.MarksAwarded, input.Feedback); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not grade answer: " + err.Error()})
		return
	}

	ungraded, err := ctrl.attemptRepo.CountUngradedAnswers(c.Request.Context(), attemptShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not check grading completion"})
		return
	}
	if ungraded == 0 {
		attempt, err := ctrl.attemptRepo.FindAttemptByShortID(c.Request.Context(), attemptShortID)
		if err == nil && attempt != nil {
			assessment, err := ctrl.assessmentRepo.FindByShortID(c.Request.Context(), attempt.AssessmentShortID)
			if err == nil && assessment != nil {
				total, err := ctrl.attemptRepo.SumMarks(c.Request.Context(), attemptShortID)
				if err == nil {
					passed := attempt.MaxScore > 0 && float64(total)/float64(attempt.MaxScore)*100 >= assessment.PassingPercentage
					_ = ctrl.attemptRepo.FinalizeAttempt(c.Request.Context(), attemptShortID, total, passed)
				}
			}
		}
	}

	c.Status(http.StatusNoContent)
}
