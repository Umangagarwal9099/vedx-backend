package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type HelpSupportController struct {
	repo *repository.HelpSupportRepository
}

func NewHelpSupportController(repo *repository.HelpSupportRepository) *HelpSupportController {
	return &HelpSupportController{repo: repo}
}

// ── FAQs ─────────────────────────────────────────────────────────────────────

// GetFAQs godoc
//
//	@Summary		List published FAQs
//	@Description	Returns every published FAQ, in display order — the Help & Support page for any authenticated user.
//	@Tags			help-support
//	@Produce		json
//	@Success		200	{array}		models.FAQ
//	@Router			/faqs [get]
func (ctrl *HelpSupportController) GetFAQs(c *gin.Context) {
	faqs, err := ctrl.repo.FindAllPublished(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch FAQs"})
		return
	}
	if faqs == nil {
		faqs = []models.FAQ{}
	}
	c.JSON(http.StatusOK, faqs)
}

// GetFAQsAdmin godoc
//
//	@Summary		List all FAQs (admin)
//	@Description	Returns every FAQ including unpublished drafts — for the admin management page.
//	@Tags			help-support
//	@Produce		json
//	@Success		200	{array}		models.FAQ
//	@Security		BearerAuth
//	@Router			/faqs/admin [get]
func (ctrl *HelpSupportController) GetFAQsAdmin(c *gin.Context) {
	faqs, err := ctrl.repo.FindAllAdmin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch FAQs"})
		return
	}
	if faqs == nil {
		faqs = []models.FAQ{}
	}
	c.JSON(http.StatusOK, faqs)
}

// CreateFAQ godoc
//
//	@Summary		Create FAQ
//	@Description	Add a new Help & Support FAQ entry. Restricted to staff.
//	@Tags			help-support
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateFAQInput	true	"FAQ"
//	@Success		201		{object}	models.FAQ
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Security		BearerAuth
//	@Router			/faqs [post]
func (ctrl *HelpSupportController) CreateFAQ(c *gin.Context) {
	var input models.CreateFAQInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	faq, err := ctrl.repo.CreateFAQ(c.Request.Context(), input, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create FAQ"})
		return
	}
	c.JSON(http.StatusCreated, faq)
}

// UpdateFAQ godoc
//
//	@Summary		Update FAQ
//	@Description	Partially update an FAQ. All fields optional. Restricted to staff.
//	@Tags			help-support
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path	string					true	"FAQ short ID"
//	@Param			body		body	models.UpdateFAQInput	true	"Fields to update"
//	@Success		204			"No Content"
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"FAQ not found"
//	@Security		BearerAuth
//	@Router			/faqs/{short_id} [patch]
func (ctrl *HelpSupportController) UpdateFAQ(c *gin.Context) {
	shortID := c.Param("short_id")
	var input models.UpdateFAQInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ctrl.repo.UpdateFAQ(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "FAQ not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update FAQ"})
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteFAQ godoc
//
//	@Summary		Delete FAQ
//	@Description	Permanently deletes an FAQ. Restricted to staff.
//	@Tags			help-support
//	@Produce		json
//	@Param			short_id	path	string	true	"FAQ short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"FAQ not found"
//	@Security		BearerAuth
//	@Router			/faqs/{short_id} [delete]
func (ctrl *HelpSupportController) DeleteFAQ(c *gin.Context) {
	shortID := c.Param("short_id")
	if err := ctrl.repo.DeleteFAQ(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "FAQ not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete FAQ"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ── Support tickets ──────────────────────────────────────────────────────────

// CreateTicket godoc
//
//	@Summary		Submit a support request
//	@Description	Creates a Help & Support ticket for the logged-in user. Visible to the admin panel afterward.
//	@Tags			help-support
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateSupportTicketInput	true	"Support request"
//	@Success		201		{object}	models.SupportTicket
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Security		BearerAuth
//	@Router			/support-tickets [post]
func (ctrl *HelpSupportController) CreateTicket(c *gin.Context) {
	var input models.CreateSupportTicketInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ticket, err := ctrl.repo.CreateTicket(c.Request.Context(), c.GetString("user_id"), input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not submit your request"})
		return
	}
	c.JSON(http.StatusCreated, ticket)
}

// GetMyTickets godoc
//
//	@Summary		My support requests
//	@Description	Returns the logged-in user's own support tickets, newest first.
//	@Tags			help-support
//	@Produce		json
//	@Success		200	{array}		models.SupportTicket
//	@Security		BearerAuth
//	@Router			/support-tickets/me [get]
func (ctrl *HelpSupportController) GetMyTickets(c *gin.Context) {
	tickets, err := ctrl.repo.FindMyTickets(c.Request.Context(), c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch your requests"})
		return
	}
	if tickets == nil {
		tickets = []models.SupportTicket{}
	}
	c.JSON(http.StatusOK, tickets)
}

// GetAllTicketsAdmin godoc
//
//	@Summary		List support requests (admin)
//	@Description	Returns every support ticket, newest first — college_admin/college_staff see only their own college's; super_admin/team_lead/mentor see everything.
//	@Tags			help-support
//	@Produce		json
//	@Success		200	{array}		models.SupportTicket
//	@Failure		403	{object}	map[string]string	"Forbidden"
//	@Security		BearerAuth
//	@Router			/support-tickets [get]
func (ctrl *HelpSupportController) GetAllTicketsAdmin(c *gin.Context) {
	collegeID, err := repository.CollegeFilter(c.GetString("role"), c.GetString("college_id"))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope"})
		return
	}
	tickets, err := ctrl.repo.FindAllForAdmin(c.Request.Context(), collegeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch support requests"})
		return
	}
	if tickets == nil {
		tickets = []models.SupportTicket{}
	}
	c.JSON(http.StatusOK, tickets)
}

// UpdateTicketStatus godoc
//
//	@Summary		Update support request status
//	@Description	Moves a ticket between open / in_progress / resolved. Restricted to staff.
//	@Tags			help-support
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path	string									true	"Ticket short ID"
//	@Param			body		body	models.UpdateSupportTicketStatusInput	true	"New status"
//	@Success		204			"No Content"
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Ticket not found"
//	@Security		BearerAuth
//	@Router			/support-tickets/{short_id}/status [patch]
func (ctrl *HelpSupportController) UpdateTicketStatus(c *gin.Context) {
	shortID := c.Param("short_id")
	var input models.UpdateSupportTicketStatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	collegeID, err := repository.CollegeFilter(c.GetString("role"), c.GetString("college_id"))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope"})
		return
	}
	if err := ctrl.repo.UpdateStatus(c.Request.Context(), shortID, input.Status, collegeID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ticket not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update ticket"})
		return
	}
	c.Status(http.StatusNoContent)
}
