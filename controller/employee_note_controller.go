package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type EmployeeNoteController struct {
	noteRepo *repository.EmployeeNoteRepository
	userRepo *repository.UserRepository
}

func NewEmployeeNoteController(noteRepo *repository.EmployeeNoteRepository, userRepo *repository.UserRepository) *EmployeeNoteController {
	return &EmployeeNoteController{noteRepo: noteRepo, userRepo: userRepo}
}

type AddEmployeeNoteRequest struct {
	NoteText string `json:"note_text" binding:"required"`
}

// AddNote godoc
//
//	@Summary		Add a note to an employee's profile
//	@Description	Manager-department only. Adds a dated note — a running record, not a one-time review.
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"Employee user ID (UUID)"
//	@Param			body	body		AddEmployeeNoteRequest	true	"Note text"
//	@Success		201		{object}	models.EmployeeNote
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		404		{object}	map[string]string	"Employee not found"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/employee-notes [post]
func (ctrl *EmployeeNoteController) AddNote(c *gin.Context) {
	id := c.Param("id")

	var req AddEmployeeNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	employee, err := ctrl.userRepo.FindByID(c.Request.Context(), id)
	if err != nil || employee == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "employee not found"})
		return
	}
	if employee.Role != models.RoleEmployee {
		c.JSON(http.StatusBadRequest, gin.H{"error": "notes are only supported for role=employee"})
		return
	}

	note, err := ctrl.noteRepo.Create(c.Request.Context(), id, c.GetString("user_id"), req.NoteText)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not add note"})
		return
	}
	c.JSON(http.StatusCreated, note)
}

// GetNotes godoc
//
//	@Summary		List notes on an employee's profile
//	@Description	Manager-department only. Newest first.
//	@Tags			users
//	@Produce		json
//	@Param			id	path		string	true	"Employee user ID (UUID)"
//	@Success		200	{array}		models.EmployeeNote
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/employee-notes [get]
func (ctrl *EmployeeNoteController) GetNotes(c *gin.Context) {
	notes, err := ctrl.noteRepo.ListForEmployee(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch notes"})
		return
	}
	c.JSON(http.StatusOK, notes)
}
