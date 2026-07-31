package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type CodingQuestionDraftController struct {
	draftRepo    *repository.CodingQuestionDraftRepository
	questionRepo *repository.CodingQuestionRepository
}

func NewCodingQuestionDraftController(draftRepo *repository.CodingQuestionDraftRepository, questionRepo *repository.CodingQuestionRepository) *CodingQuestionDraftController {
	return &CodingQuestionDraftController{draftRepo: draftRepo, questionRepo: questionRepo}
}

// SaveDraft godoc
//
//	@Summary		Autosave in-progress code
//	@Description	Upserts the caller's latest code for this question+language. Meant to be called on a debounce timer while typing — cheap to call repeatedly, and safe to call with empty code (a cleared editor is a valid state).
//	@Tags			coding-questions
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path	string					true	"Question short ID"
//	@Param			body		body	models.SaveDraftInput	true	"Draft"
//	@Success		204			"No Content"
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Question not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/coding-questions/{short_id}/draft [put]
func (ctrl *CodingQuestionDraftController) SaveDraft(c *gin.Context) {
	shortID := c.Param("short_id")
	userID := c.GetString("user_id")

	var input models.SaveDraftInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	question, err := ctrl.questionRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify question"})
		return
	}
	if question == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "question not found"})
		return
	}

	if err := ctrl.draftRepo.Upsert(c.Request.Context(), userID, question.ID, input.Language, input.Code); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save draft"})
		return
	}
	c.Status(http.StatusNoContent)
}

// GetDrafts godoc
//
//	@Summary		Get my saved drafts for a question
//	@Description	Returns every language draft the caller has saved for this question, keyed by language — e.g. {"python": "...", "java": "..."}. Missing keys mean no draft was ever saved for that language.
//	@Tags			coding-questions
//	@Produce		json
//	@Param			short_id	path	string	true	"Question short ID"
//	@Success		200			{object}	map[string]string
//	@Failure		404			{object}	map[string]string	"Question not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/coding-questions/{short_id}/draft [get]
func (ctrl *CodingQuestionDraftController) GetDrafts(c *gin.Context) {
	shortID := c.Param("short_id")
	userID := c.GetString("user_id")

	question, err := ctrl.questionRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify question"})
		return
	}
	if question == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "question not found"})
		return
	}

	drafts, err := ctrl.draftRepo.GetAllForQuestion(c.Request.Context(), userID, question.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch drafts"})
		return
	}
	if drafts == nil {
		drafts = map[string]string{}
	}
	c.JSON(http.StatusOK, drafts)
}
