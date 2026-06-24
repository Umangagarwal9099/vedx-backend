package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type FeedbackFormController struct {
	repo *repository.FeedbackFormRepository
}

func NewFeedbackFormController(repo *repository.FeedbackFormRepository) *FeedbackFormController {
	return &FeedbackFormController{repo: repo}
}

// CreateFeedbackForm godoc
//
//	@Summary		Create feedback form
//	@Description	Step 1 + 2: pick a form_type and a template. No title or description needed — the title is auto-generated from the template. Valid combos: session_feedback → blank_form | trainer_performance | course_content | overall_satisfaction; link_to_course → blank_form | content_rating | csat | course_rating; general_survey → blank_form only. Restricted to super_admin, mentor, and team_lead.
//	@Tags			feedback-forms
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateFeedbackFormInput	true	"form_type + template only"
//	@Success		201		{object}	models.FeedbackForm
//	@Failure		400		{object}	map[string]string	"Validation error or invalid form_type/template combo"
//	@Failure		403		{object}	map[string]string	"Forbidden"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/feedback-forms [post]
func (ctrl *FeedbackFormController) Create(c *gin.Context) {
	var input models.CreateFeedbackFormInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !models.ValidTemplatesFor[input.FormType][input.Template] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "template '" + input.Template + "' is not valid for form_type '" + input.FormType + "'"})
		return
	}

	createdBy := c.GetString("user_id")
	form, err := ctrl.repo.Create(c.Request.Context(), input, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create feedback form"})
		return
	}
	c.JSON(http.StatusCreated, form)
}

// GetAllFeedbackForms godoc
//
//	@Summary		List feedback forms
//	@Description	Returns all non-deleted feedback forms ordered newest first.
//	@Tags			feedback-forms
//	@Produce		json
//	@Success		200	{array}		models.FeedbackForm
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/feedback-forms [get]
func (ctrl *FeedbackFormController) GetAll(c *gin.Context) {
	forms, err := ctrl.repo.FindAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch feedback forms"})
		return
	}
	if forms == nil {
		forms = []models.FeedbackForm{}
	}
	c.JSON(http.StatusOK, forms)
}

// GetFeedbackForm godoc
//
//	@Summary		Get feedback form with questions
//	@Description	Returns a single non-deleted feedback form along with all its questions ordered by order_index.
//	@Tags			feedback-forms
//	@Produce		json
//	@Param			short_id	path		string	true	"Form short ID"
//	@Success		200			{object}	models.FeedbackFormWithQuestions
//	@Failure		404			{object}	map[string]string	"Form not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/feedback-forms/{short_id} [get]
func (ctrl *FeedbackFormController) GetByShortID(c *gin.Context) {
	shortID := c.Param("short_id")
	form, err := ctrl.repo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch feedback form"})
		return
	}
	if form == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "feedback form not found"})
		return
	}
	c.JSON(http.StatusOK, form)
}

// UpdateFeedbackForm godoc
//
//	@Summary		Update feedback form
//	@Description	Partially update a feedback form by its short_id. Send only the fields you want to change. Use is_active: false to disable the form and is_active: true to re-enable it. Restricted to super_admin, mentor, and team_lead.
//	@Tags			feedback-forms
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"Form short ID"
//	@Param			body		body		models.UpdateFeedbackFormInput	true	"Fields to update (all optional)"
//	@Success		200			{object}	models.FeedbackForm
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		403			{object}	map[string]string	"Forbidden"
//	@Failure		404			{object}	map[string]string	"Form not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/feedback-forms/{short_id} [patch]
func (ctrl *FeedbackFormController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateFeedbackFormInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.repo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "feedback form not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update feedback form"})
		return
	}

	form, err := ctrl.repo.FindFormByShortID(c.Request.Context(), shortID)
	if err != nil || form == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated form"})
		return
	}
	c.JSON(http.StatusOK, form)
}

// DeleteFeedbackForm godoc
//
//	@Summary		Delete feedback form
//	@Description	Soft-delete a feedback form by its short_id (sets deleted_at; the row is retained). Restricted to super_admin, mentor, and team_lead.
//	@Tags			feedback-forms
//	@Produce		json
//	@Param			short_id	path	string	true	"Form short ID"
//	@Success		204			"No Content"
//	@Failure		403			{object}	map[string]string	"Forbidden"
//	@Failure		404			{object}	map[string]string	"Form not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/feedback-forms/{short_id} [delete]
func (ctrl *FeedbackFormController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")
	if err := ctrl.repo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "feedback form not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete feedback form"})
		return
	}
	c.Status(http.StatusNoContent)
}

// AddQuestion godoc
//
//	@Summary		Add question to form
//	@Description	Add a new question to an existing feedback form. Supported types: session_rating, trainer_rating, single_choice, multiple_choice, star_rating, linear_scale, date, number, short_answer, long_answer. For scale types supply scale_min/scale_max and optional start_label/end_label. For choice types supply options[]. Restricted to super_admin, mentor, and team_lead.
//	@Tags			feedback-forms
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string									true	"Form short ID"
//	@Param			body		body		models.CreateFeedbackFormQuestionInput	true	"Question details"
//	@Success		201			{object}	models.FeedbackFormQuestion
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		403			{object}	map[string]string	"Forbidden"
//	@Failure		404			{object}	map[string]string	"Form not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/feedback-forms/{short_id}/questions [post]
func (ctrl *FeedbackFormController) AddQuestion(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.CreateFeedbackFormQuestionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	question, err := ctrl.repo.AddQuestion(c.Request.Context(), shortID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "feedback form not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not add question"})
		return
	}
	c.JSON(http.StatusCreated, question)
}

// UpdateQuestion godoc
//
//	@Summary		Update a question
//	@Description	Partially update a question in a feedback form. Send only the fields you want to change. To clear the options array send "options": []. To clear scale labels send "start_label": "" or "end_label": "". Restricted to super_admin, mentor, and team_lead.
//	@Tags			feedback-forms
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string									true	"Form short ID"
//	@Param			q_short_id	path		string									true	"Question short ID"
//	@Param			body		body		models.UpdateFeedbackFormQuestionInput	true	"Fields to update (all optional)"
//	@Success		200			{object}	models.FeedbackFormQuestion
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		403			{object}	map[string]string	"Forbidden"
//	@Failure		404			{object}	map[string]string	"Form or question not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/feedback-forms/{short_id}/questions/{q_short_id} [patch]
func (ctrl *FeedbackFormController) UpdateQuestion(c *gin.Context) {
	shortID := c.Param("short_id")
	qShortID := c.Param("q_short_id")

	var input models.UpdateFeedbackFormQuestionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	question, err := ctrl.repo.UpdateQuestion(c.Request.Context(), shortID, qShortID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "form or question not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update question"})
		return
	}
	c.JSON(http.StatusOK, question)
}

// DeleteQuestion godoc
//
//	@Summary		Delete a question
//	@Description	Permanently remove a question from a feedback form. Restricted to super_admin, mentor, and team_lead.
//	@Tags			feedback-forms
//	@Produce		json
//	@Param			short_id	path	string	true	"Form short ID"
//	@Param			q_short_id	path	string	true	"Question short ID"
//	@Success		204			"No Content"
//	@Failure		403			{object}	map[string]string	"Forbidden"
//	@Failure		404			{object}	map[string]string	"Form or question not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/feedback-forms/{short_id}/questions/{q_short_id} [delete]
func (ctrl *FeedbackFormController) DeleteQuestion(c *gin.Context) {
	shortID := c.Param("short_id")
	qShortID := c.Param("q_short_id")

	if err := ctrl.repo.DeleteQuestion(c.Request.Context(), shortID, qShortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "form or question not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete question"})
		return
	}
	c.Status(http.StatusNoContent)
}
