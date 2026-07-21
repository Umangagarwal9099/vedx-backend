package controller

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
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
	batchRepo        *repository.BatchRepository
	notificationRepo *repository.NotificationRepository
	auditLogRepo     *repository.AuditLogRepository
}

func NewExamAttemptController(attemptRepo *repository.ExamAttemptRepository, assessmentRepo *repository.AssessmentRepository, questionBankRepo *repository.QuestionBankRepository, batchRepo *repository.BatchRepository, notificationRepo *repository.NotificationRepository, auditLogRepo *repository.AuditLogRepository) *ExamAttemptController {
	return &ExamAttemptController{attemptRepo: attemptRepo, assessmentRepo: assessmentRepo, questionBankRepo: questionBankRepo, batchRepo: batchRepo, notificationRepo: notificationRepo, auditLogRepo: auditLogRepo}
}

// computeEndsAt derives an attempt's hard deadline: the earlier of the
// assessment's own end_at and startedAt+duration_minutes. Either bound may be
// absent (static/untimed assessments) — if both are absent there's no
// deadline, and the attempt is never auto-submitted.
func computeEndsAt(startedAt time.Time, assessment *models.Assessment) *time.Time {
	var durationEnd *time.Time
	if assessment.DurationMinutes != nil && *assessment.DurationMinutes > 0 {
		t := startedAt.Add(time.Duration(*assessment.DurationMinutes) * time.Minute)
		durationEnd = &t
	}
	switch {
	case assessment.EndAt != nil && durationEnd != nil:
		if assessment.EndAt.Before(*durationEnd) {
			return assessment.EndAt
		}
		return durationEnd
	case assessment.EndAt != nil:
		return assessment.EndAt
	default:
		return durationEnd
	}
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

	if assessment.CancelledAt != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this assessment has been cancelled"})
		return
	}

	if assessment.BatchShortID != "" {
		enrolled, err := ctrl.batchRepo.IsStudentEnrolled(c.Request.Context(), assessment.BatchShortID, studentID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify batch enrollment"})
			return
		}
		if !enrolled {
			c.JSON(http.StatusForbidden, gin.H{"error": "you are not enrolled in this assessment's batch"})
			return
		}
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

	grantCount, err := ctrl.attemptRepo.CountReattemptGrants(c.Request.Context(), assessmentShortID, studentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not check reattempt grants"})
		return
	}
	// A granted reattempt is an explicit staff override — it lifts the
	// already-passed block and extends the attempt ceiling by one each.
	if hasPassed && !assessment.AllowAttemptsAfterPassing && grantCount == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "you've already passed this assessment"})
		return
	}
	maxAttempts := assessment.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	maxAttempts += grantCount
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

	endsAt := computeEndsAt(now, assessment)
	attempt, err := ctrl.attemptRepo.CreateAttempt(c.Request.Context(), assessmentShortID, studentID, len(pastAttempts)+1, maxScore, order, now, endsAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not start attempt: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, models.AttemptDetail{
		ExamAttempt: *attempt,
		Questions:   buildQuestionViews(attempt, order, questions, nil, false),
	})
}

// deriveCompletedDisplayStatus maps a finished (submitted/evaluated) attempt
// to the student-facing display_status the spec defines, respecting result
// publication — an "evaluated" attempt still reads as "evaluation_pending"
// to the student until the assessment's results are published.
func deriveCompletedDisplayStatus(attempt *models.ExamAttempt, resultsVisible bool) string {
	switch attempt.Status {
	case "evaluated":
		if resultsVisible {
			return "graded"
		}
		return "evaluation_pending"
	case "cancelled":
		return "cancelled"
	default: // "submitted"
		if attempt.AutoSubmitted {
			return "auto_submitted"
		}
		return "submitted"
	}
}

// GetAccessStatus godoc
//
//	@Summary		Resolve exam access/display status
//	@Description	The single source of truth the student frontend must render Start/Resume/Closed/Missed/Submitted state from — do not recompute this client-side. Returns can_start, can_resume, display_status, reason, attempt_status, starts_at, ends_at.
//	@Tags			exam-attempts
//	@Produce		json
//	@Param			short_id	path		string	true	"Assessment short ID"
//	@Success		200			{object}	models.ExamAccessStatus
//	@Failure		404			{object}	map[string]string	"Assessment not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id}/access-status [get]
func (ctrl *ExamAttemptController) GetAccessStatus(c *gin.Context) {
	assessmentShortID := c.Param("short_id")
	studentID := c.GetString("user_id")

	assessment, err := ctrl.assessmentRepo.FindByShortID(c.Request.Context(), assessmentShortID)
	if err != nil || assessment == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assessment not found"})
		return
	}

	secCfg, secCfgFound := ctrl.assessmentRepo.GetSecurityConfig(c.Request.Context(), assessmentShortID)
	resp := models.ExamAccessStatus{
		StartsAt:       assessment.StartAt,
		EndsAt:         assessment.EndAt,
		MaxAttempts:    assessment.MaxAttempts,
		ResultsVisible: assessment.ResultsVisibleWith(secCfg.ResultPublishedAt, secCfgFound),
	}

	if assessment.CancelledAt != nil {
		resp.DisplayStatus = "cancelled"
		resp.Reason = "This examination has been cancelled."
		c.JSON(http.StatusOK, resp)
		return
	}

	if assessment.BatchShortID != "" {
		enrolled, err := ctrl.batchRepo.IsStudentEnrolled(c.Request.Context(), assessment.BatchShortID, studentID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify batch enrollment"})
			return
		}
		if !enrolled {
			resp.DisplayStatus = "not_available"
			resp.Reason = "You are not enrolled in this assessment's batch."
			c.JSON(http.StatusOK, resp)
			return
		}
	}

	attempts, err := ctrl.attemptRepo.FindMyAttempts(c.Request.Context(), assessmentShortID, studentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not check previous attempts"})
		return
	}
	resp.AttemptNumber = len(attempts)

	// An in_progress attempt always takes priority — resumable regardless of
	// the window, since the sweep (or the student) will close it out.
	for i := range attempts {
		if attempts[i].Status == "in_progress" {
			resp.CanResume = true
			resp.AttemptStatus = "in_progress"
			resp.DisplayStatus = "in_progress"
			resp.AttemptShortID = attempts[i].ShortID
			c.JSON(http.StatusOK, resp)
			return
		}
	}

	hasPassed := false
	var mostRecent *models.ExamAttempt
	for i := range attempts {
		if attempts[i].Passed != nil && *attempts[i].Passed {
			hasPassed = true
		}
		if mostRecent == nil || attempts[i].AttemptNumber > mostRecent.AttemptNumber {
			mostRecent = &attempts[i]
		}
	}

	grantCount, err := ctrl.attemptRepo.CountReattemptGrants(c.Request.Context(), assessmentShortID, studentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not check reattempt grants"})
		return
	}
	maxAttempts := assessment.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	maxAttempts += grantCount
	resp.MaxAttempts = maxAttempts
	attemptsExhausted := len(attempts) >= maxAttempts
	passedBlocked := hasPassed && !assessment.AllowAttemptsAfterPassing && grantCount == 0

	now := time.Now()
	beforeStart := assessment.StartAt != nil && now.Before(*assessment.StartAt)
	afterEnd := assessment.EndAt != nil && now.After(*assessment.EndAt)

	canStartMore := !beforeStart && !afterEnd && !attemptsExhausted && !passedBlocked

	if afterEnd {
		if mostRecent != nil {
			resp.AttemptStatus = mostRecent.Status
			resp.DisplayStatus = deriveCompletedDisplayStatus(mostRecent, resp.ResultsVisible)
			resp.Reason = "The examination window has closed."
		} else {
			resp.DisplayStatus = "missed"
			resp.Reason = "The examination window has closed. You did not submit this examination."
		}
		c.JSON(http.StatusOK, resp)
		return
	}

	if canStartMore {
		resp.CanStart = true
		resp.DisplayStatus = "not_started"
		c.JSON(http.StatusOK, resp)
		return
	}

	if beforeStart {
		resp.DisplayStatus = "not_started"
		resp.Reason = "This examination has not started yet."
		c.JSON(http.StatusOK, resp)
		return
	}

	// Window is open but the student can't start another attempt (exhausted
	// or already passed) — show their most recent completed attempt's state.
	if mostRecent != nil {
		resp.AttemptStatus = mostRecent.Status
		resp.DisplayStatus = deriveCompletedDisplayStatus(mostRecent, resp.ResultsVisible)
		if passedBlocked {
			resp.Reason = "You've already passed this assessment."
		} else {
			resp.Reason = "You've used all your attempts for this assessment."
		}
	} else {
		// No attempts at all, yet can't start — only reachable if
		// max_attempts is 0/misconfigured; report plainly rather than crash.
		resp.DisplayStatus = "not_available"
		resp.Reason = "You cannot start this assessment."
	}
	c.JSON(http.StatusOK, resp)
}

// checkAttemptAccess enforces that only the student who owns this attempt, or
// staff with access to its batch (mentor/employee scoped via checkBatchAccess,
// super_admin/team_lead unrestricted), may view or act on it. Without this,
// any authenticated student could read or mutate another student's attempt
// just by guessing/enumerating its short_id.
func (ctrl *ExamAttemptController) checkAttemptAccess(c *gin.Context, attempt *models.ExamAttempt) bool {
	role := c.GetString("role")
	if role == string(models.RoleStudent) {
		if attempt.StudentID != c.GetString("user_id") {
			c.JSON(http.StatusForbidden, gin.H{"error": "you can only access your own attempt"})
			return false
		}
		return true
	}
	assessment, err := ctrl.assessmentRepo.FindByShortID(c.Request.Context(), attempt.AssessmentShortID)
	if err != nil || assessment == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not resolve assessment"})
		return false
	}
	return checkBatchAccess(c, ctrl.batchRepo, assessment.BatchShortID)
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

	role := c.GetString("role")
	isStudent := role == string(models.RoleStudent)
	secCfg, secCfgFound := ctrl.assessmentRepo.GetSecurityConfig(c.Request.Context(), attempt.AssessmentShortID)
	resultsVisible := assessment.ResultsVisibleWith(secCfg.ResultPublishedAt, secCfgFound)

	// Correct answers/explanations are only ever revealed once the attempt is
	// finished, the assessment allows it, and — for the student themselves —
	// only once results are actually published. Staff reviewing/grading
	// always see them regardless of publish status.
	reveal := attempt.Status != "in_progress" && assessment.ShowCorrectAnswers && (!isStudent || resultsVisible)

	sanitized := *attempt
	if isStudent && attempt.Status == "evaluated" && !resultsVisible {
		sanitized.TotalScore = nil
		sanitized.Passed = nil
	}

	status := http.StatusOK
	if notFoundIfMissing {
		status = http.StatusCreated
	}
	c.JSON(status, models.AttemptDetail{
		ExamAttempt: sanitized,
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

	attempt, err := ctrl.attemptRepo.FindAttemptByShortID(c.Request.Context(), attemptShortID)
	if err != nil || attempt == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "attempt not found"})
		return
	}
	if !ctrl.checkAttemptAccess(c, attempt) {
		return
	}
	if attempt.Status != "in_progress" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "attempt is no longer in progress"})
		return
	}
	if attempt.EndsAt != nil && time.Now().After(*attempt.EndsAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "time is up for this attempt"})
		return
	}

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

// RecordViolation godoc
//
//	@Summary		Report an exam-integrity violation
//	@Description	Records a proctoring-integrity event (tab switch, fullscreen exit, etc.) for an in-progress attempt. Applies the assessment's configured thresholds — allowed_warning_count and auto_submit_on_violation — and auto-submits the attempt once the threshold is reached. A normal browser cannot fully prevent a student from leaving the exam; this only detects and responds to what the browser can observe.
//	@Tags			exam-attempts
//	@Accept			json
//	@Produce		json
//	@Param			short_id			path		string							true	"Assessment short ID"
//	@Param			attempt_short_id	path		string							true	"Attempt short ID"
//	@Param			body				body		models.RecordViolationInput	true	"Violation details"
//	@Success		200					{object}	models.RecordViolationResponse
//	@Failure		400					{object}	map[string]string	"Validation error, or attempt is no longer in progress"
//	@Failure		404					{object}	map[string]string	"Attempt not found"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id}/attempts/{attempt_short_id}/violations [post]
func (ctrl *ExamAttemptController) RecordViolation(c *gin.Context) {
	attemptShortID := c.Param("attempt_short_id")

	attempt, err := ctrl.attemptRepo.FindAttemptByShortID(c.Request.Context(), attemptShortID)
	if err != nil || attempt == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "attempt not found"})
		return
	}
	if !ctrl.checkAttemptAccess(c, attempt) {
		return
	}
	if attempt.Status != "in_progress" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "attempt is no longer in progress"})
		return
	}

	assessment, err := ctrl.assessmentRepo.FindByShortID(c.Request.Context(), attempt.AssessmentShortID)
	if err != nil || assessment == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not resolve assessment"})
		return
	}

	var input models.RecordViolationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	priorCount, err := ctrl.attemptRepo.CountViolations(c.Request.Context(), attemptShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not check violation history"})
		return
	}
	warningNumber := priorCount + 1

	allowedWarnings := assessment.AllowedWarningCount
	if allowedWarnings <= 0 {
		allowedWarnings = 1
	}
	shouldAutoSubmit := assessment.AutoSubmitOnViolation && warningNumber > allowedWarnings
	actionTaken := "warned"
	if shouldAutoSubmit {
		actionTaken = "auto_submitted"
	}

	if _, err := ctrl.attemptRepo.RecordViolation(c.Request.Context(), attemptShortID, attempt.StudentID, input.ViolationType, input.BrowserInfo, warningNumber, actionTaken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not record violation: " + err.Error()})
		return
	}

	resp := models.RecordViolationResponse{WarningNumber: warningNumber, ActionTaken: actionTaken, AttemptStatus: attempt.Status}

	if shouldAutoSubmit {
		if err := ctrl.attemptRepo.MarkSubmitted(c.Request.Context(), attemptShortID, true, "violation"); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not auto-submit after violation"})
			return
		}
		if err := ctrl.GradeObjectiveQuestions(c.Request.Context(), attempt, assessment); err != nil {
			log.Printf("grade after violation auto-submit %s: %v", attemptShortID, err)
		}
		resp.AttemptStatus = "submitted"
	}

	c.JSON(http.StatusOK, resp)
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
	if !ctrl.checkAttemptAccess(c, attempt) {
		return
	}

	assessment, err := ctrl.assessmentRepo.FindByShortID(c.Request.Context(), attempt.AssessmentShortID)
	if err != nil || assessment == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not resolve assessment"})
		return
	}

	if err := ctrl.attemptRepo.MarkSubmitted(c.Request.Context(), attemptShortID, false, "manual"); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not submit attempt"})
			return
		}
		// already submitted — fall through to (re)grade idempotently
	}

	if err := ctrl.GradeObjectiveQuestions(c.Request.Context(), attempt, assessment); err != nil {
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

// GradeObjectiveQuestions auto-grades every objective answer in an attempt and,
// if there are no manual (short_answer/descriptive/coding) questions, finalizes
// the attempt's score and pass/fail immediately. Shared by the student-facing
// SubmitAttempt handler and the auto-submit sweep (scheduler/exam_attempt_sweep.go),
// which is why it takes a plain context.Context rather than a *gin.Context.
func (ctrl *ExamAttemptController) GradeObjectiveQuestions(ctx context.Context, attempt *models.ExamAttempt, assessment *models.Assessment) error {
	questions, err := ctrl.questionBankRepo.GetQuestions(ctx, attempt.AssessmentShortID)
	if err != nil {
		return err
	}
	answers, err := ctrl.attemptRepo.GetAnswers(ctx, attempt.ShortID)
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
		if err := ctrl.attemptRepo.SetAnswerGrade(ctx, attempt.ShortID, q.ShortID, &isCorrect, marks, ""); err != nil {
			return err
		}
	}

	if anyManual {
		return nil // leave status "submitted" — pending manual grading
	}

	total, err := ctrl.attemptRepo.SumMarks(ctx, attempt.ShortID)
	if err != nil {
		return err
	}
	passed := attempt.MaxScore > 0 && float64(total)/float64(attempt.MaxScore)*100 >= assessment.PassingPercentage
	return ctrl.attemptRepo.FinalizeAttempt(ctx, attempt.ShortID, total, passed)
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

	// This is the calling student's own list — redact score/pass-fail on any
	// evaluated attempt until the assessment's results are published.
	assessment, assErr := ctrl.assessmentRepo.FindByShortID(c.Request.Context(), assessmentShortID)
	resultsVisible := true
	if assErr == nil && assessment != nil {
		secCfg, secCfgFound := ctrl.assessmentRepo.GetSecurityConfig(c.Request.Context(), assessmentShortID)
		resultsVisible = assessment.ResultsVisibleWith(secCfg.ResultPublishedAt, secCfgFound)
	}
	if !resultsVisible {
		for i := range attempts {
			if attempts[i].Status == "evaluated" {
				attempts[i].TotalScore = nil
				attempts[i].Passed = nil
			}
		}
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
	if !ctrl.checkAttemptAccess(c, attempt) {
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

	assessment, err := ctrl.assessmentRepo.FindByShortID(c.Request.Context(), assessmentShortID)
	if err != nil || assessment == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assessment not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, assessment.BatchShortID) {
		return
	}

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

// GetAllAttemptsGlobal godoc
//
//	@Summary		List every submitted/evaluated exam attempt (cross-assessment)
//	@Description	Returns every submitted or evaluated exam attempt across every assessment, newest first — mentors see only attempts in batches they manage (plus any global assessments); team_lead/super_admin see everything. Powers the unified Submissions workspace.
//	@Tags			assessments
//	@Produce		json
//	@Success		200	{array}		models.ExamAttempt
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/submissions/assessments [get]
func (ctrl *ExamAttemptController) GetAllAttemptsGlobal(c *gin.Context) {
	mentorID := ""
	role := c.GetString("role")
	if role == string(models.RoleMentor) || role == string(models.RoleEmployee) {
		mentorID = c.GetString("user_id")
	}

	attempts, err := ctrl.attemptRepo.FindAllAttemptsForMentor(c.Request.Context(), mentorID)
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
	if !checkBatchAccess(c, ctrl.batchRepo, assessment.BatchShortID) {
		return
	}

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

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "grade", EntityType: "exam_answer",
		EntityShortID: attemptShortID, EntityLabel: assessment.Name,
		BatchShortID: assessment.BatchShortID,
		Metadata:     map[string]interface{}{"question_short_id": questionShortID, "marks_awarded": input.MarksAwarded},
	})

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
					if err := ctrl.attemptRepo.FinalizeAttempt(c.Request.Context(), attemptShortID, total, passed); err == nil {
						ctrl.notifyResultReady(c.Request.Context(), attempt, assessment, c.GetString("user_id"))
					}
				}
			}
		}
	}

	c.Status(http.StatusNoContent)
}

// ── Notifications ────────────────────────────────────────────────────────────
// ExamAttemptController previously had no notification triggers at all — these
// four cover the attempt-lifecycle events the gap analysis flagged as missing:
// auto-submitted, result ready, reattempt granted, exam cancelled.

func (ctrl *ExamAttemptController) notifyResultReady(ctx context.Context, attempt *models.ExamAttempt, assessment *models.Assessment, actorID string) {
	if err := ctrl.notificationRepo.NotifyUsers(ctx,
		"Result ready: "+assessment.Name,
		fmt.Sprintf("Your result for %q is ready.", assessment.Name),
		"assessment_result", "assessment", assessment.ShortID, actorID,
		[]string{attempt.StudentID},
	); err != nil {
		log.Printf("notify result ready: %v", err)
	}
}

func (ctrl *ExamAttemptController) notifyAutoSubmitted(ctx context.Context, attempt *models.ExamAttempt, assessment *models.Assessment) {
	if err := ctrl.notificationRepo.NotifyUsers(ctx,
		"Time's up: "+assessment.Name,
		fmt.Sprintf("Your time for %q ran out — your answers were submitted automatically.", assessment.Name),
		"assessment_auto_submit", "assessment", assessment.ShortID, attempt.StudentID,
		[]string{attempt.StudentID},
	); err != nil {
		log.Printf("notify auto-submitted: %v", err)
	}
}

func (ctrl *ExamAttemptController) notifyReattemptGranted(ctx context.Context, assessment *models.Assessment, studentID, grantedBy string) {
	if err := ctrl.notificationRepo.NotifyUsers(ctx,
		"Reattempt granted: "+assessment.Name,
		fmt.Sprintf("You've been granted another attempt at %q.", assessment.Name),
		"assessment_reattempt", "assessment", assessment.ShortID, grantedBy,
		[]string{studentID},
	); err != nil {
		log.Printf("notify reattempt granted: %v", err)
	}
}

func (ctrl *ExamAttemptController) notifyCancelled(c *gin.Context, assessment *models.Assessment, actorID string) {
	title := "Cancelled: " + assessment.Name
	message := fmt.Sprintf("%q has been cancelled.", assessment.Name)

	if assessment.BatchShortID != "" {
		students, err := ctrl.batchRepo.GetStudents(c.Request.Context(), assessment.BatchShortID)
		if err != nil {
			log.Printf("fetch batch students for cancel notify: %v", err)
		}
		recipients := make([]string, 0, len(students))
		for _, s := range students {
			recipients = append(recipients, s.UserID)
		}
		if err := ctrl.notificationRepo.NotifyUsers(c.Request.Context(), title, message, "assessment_cancelled", "assessment", assessment.ShortID, actorID, recipients); err != nil {
			log.Printf("notify cancelled (students): %v", err)
		}
		return
	}
	if err := ctrl.notificationRepo.NotifyRoles(c.Request.Context(), title, message, "assessment_cancelled", "assessment", assessment.ShortID, actorID, []string{"student"}); err != nil {
		log.Printf("notify cancelled (broadcast): %v", err)
	}
}

// ── Auto-submit sweep ────────────────────────────────────────────────────────

// AutoSubmitExpired force-submits every attempt whose deadline has passed and
// grades its objective answers, exactly as a manual submit would. Called on a
// ticker by scheduler/exam_attempt_sweep.go — this is the piece that makes
// auto_submit / duration_minutes actually enforce a deadline server-side,
// instead of relying on the student's browser to call submit in time.
func (ctrl *ExamAttemptController) AutoSubmitExpired(ctx context.Context) (int, error) {
	expired, err := ctrl.attemptRepo.FindExpiredInProgress(ctx)
	if err != nil {
		return 0, fmt.Errorf("find expired attempts: %w", err)
	}

	processed := 0
	for i := range expired {
		attempt := &expired[i]
		if err := ctrl.attemptRepo.MarkSubmitted(ctx, attempt.ShortID, true, "timer_expired"); err != nil {
			log.Printf("auto-submit: mark submitted %s: %v", attempt.ShortID, err)
			continue
		}
		assessment, err := ctrl.assessmentRepo.FindByShortID(ctx, attempt.AssessmentShortID)
		if err != nil || assessment == nil {
			log.Printf("auto-submit: resolve assessment for %s: %v", attempt.ShortID, err)
			continue
		}
		if err := ctrl.GradeObjectiveQuestions(ctx, attempt, assessment); err != nil {
			log.Printf("auto-submit: grade %s: %v", attempt.ShortID, err)
		}
		ctrl.notifyAutoSubmitted(ctx, attempt, assessment)
		processed++
	}
	return processed, nil
}

// ── Reattempt & cancel ───────────────────────────────────────────────────────

// GrantReattempt godoc
//
//	@Summary		Grant a reattempt
//	@Description	Grants a student one extra attempt beyond the assessment's max_attempts, or lets them retry after already passing. Previous attempts are untouched — this only raises the ceiling by one and records who granted it and why. Restricted to super_admin / team_lead / mentor (of a batch they manage).
//	@Tags			exam-attempts
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string						true	"Assessment short ID"
//	@Param			body		body		models.GrantReattemptInput	true	"Student and reason"
//	@Success		201			{object}	models.ReattemptGrant
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Assessment not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id}/reattempts [post]
func (ctrl *ExamAttemptController) GrantReattempt(c *gin.Context) {
	assessmentShortID := c.Param("short_id")

	assessment, err := ctrl.assessmentRepo.FindByShortID(c.Request.Context(), assessmentShortID)
	if err != nil || assessment == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assessment not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, assessment.BatchShortID) {
		return
	}

	var input models.GrantReattemptInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pastAttempts, err := ctrl.attemptRepo.FindMyAttempts(c.Request.Context(), assessmentShortID, input.StudentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not check previous attempts"})
		return
	}

	grantedBy := c.GetString("user_id")
	grant, err := ctrl.attemptRepo.CreateReattemptGrant(c.Request.Context(), assessmentShortID, input.StudentID, grantedBy, input.Reason, len(pastAttempts)+1)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not grant reattempt: " + err.Error()})
		return
	}

	ctrl.notifyReattemptGranted(c.Request.Context(), assessment, input.StudentID, grantedBy)

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "grant_reattempt", EntityType: "exam_attempt",
		EntityShortID: input.StudentID, EntityLabel: assessment.Name,
		BatchShortID: assessment.BatchShortID,
		Metadata:     map[string]interface{}{"reason": input.Reason, "attempt_number": len(pastAttempts) + 1},
	})

	c.JSON(http.StatusCreated, grant)
}

// CancelAssessment godoc
//
//	@Summary		Cancel an assessment
//	@Description	Cancels an assessment — blocks any new attempt and marks every currently in-progress attempt as cancelled. Existing submitted/evaluated attempts are untouched. Restricted to super_admin / team_lead / mentor (of a batch they manage).
//	@Tags			assessments
//	@Produce		json
//	@Param			short_id	path	string	true	"Assessment short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Assessment not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id}/cancel [post]
func (ctrl *ExamAttemptController) CancelAssessment(c *gin.Context) {
	assessmentShortID := c.Param("short_id")

	assessment, err := ctrl.assessmentRepo.FindByShortID(c.Request.Context(), assessmentShortID)
	if err != nil || assessment == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assessment not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, assessment.BatchShortID) {
		return
	}

	actorID := c.GetString("user_id")
	if err := ctrl.assessmentRepo.Cancel(c.Request.Context(), assessmentShortID, actorID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "already cancelled, or assessment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not cancel assessment"})
		return
	}
	if err := ctrl.attemptRepo.CancelAssessmentAttempts(c.Request.Context(), assessmentShortID); err != nil {
		log.Printf("cancel in-progress attempts for %s: %v", assessmentShortID, err)
	}

	ctrl.notifyCancelled(c, assessment, actorID)

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "cancel", EntityType: "assessment",
		EntityShortID: assessmentShortID, EntityLabel: assessment.Name,
		BatchShortID: assessment.BatchShortID,
	})

	c.Status(http.StatusNoContent)
}
