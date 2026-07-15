package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
	"github.com/umangagarwal/vedx-backend/util"
	"golang.org/x/crypto/bcrypt"
)

type UserController struct {
	userRepo  *repository.UserRepository
	emailSvc  *service.EmailService
	publicURL string
}

func NewUserController(userRepo *repository.UserRepository, emailSvc *service.EmailService, publicURL string) *UserController {
	return &UserController{userRepo: userRepo, emailSvc: emailSvc, publicURL: publicURL}
}

// roleLabels maps a role to the human-readable label used in the welcome email.
var roleLabels = map[models.Role]string{
	models.RoleMentor:   "Mentor",
	models.RoleEmployee: "Employee",
	models.RoleTeamLead: "Team Lead",
	models.RoleStudent:  "Student",
}

// CreateStaffUserRequest carries the fields for admin-provisioned staff accounts.
type CreateStaffUserRequest struct {
	FirstName string `json:"first_name" binding:"required" example:"Jane"`
	LastName  string `json:"last_name"  binding:"required" example:"Doe"`
	Email     string `json:"email"      binding:"required,email" example:"jane@example.com"`
	Phone     string `json:"phone"      example:"+919876543210"`
	Role      string `json:"role"       binding:"required,oneof=mentor employee team_lead" enums:"mentor,employee,team_lead" example:"employee"`
}

// CreateStaffUserResponse returns the created user plus the one-time-shown
// temporary password and whether the welcome email was sent successfully.
type CreateStaffUserResponse struct {
	User              models.User `json:"user"`
	TemporaryPassword string      `json:"temporary_password"`
	EmailSent         bool        `json:"email_sent"`
}

// CreateStaffUser godoc
//
//	@Summary		Create a staff account
//	@Description	Admin-provisioned account creation for mentor/employee/team_lead roles. Generates a temporary password, emails it to the new user, and returns it once in the response.
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CreateStaffUserRequest	true	"New staff account details"
//	@Success		201		{object}	CreateStaffUserResponse
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		409		{object}	map[string]string	"Email already in use"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/staff [post]
func (ctrl *UserController) CreateStaffUser(c *gin.Context) {
	var req CreateStaffUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	exists, err := ctrl.userRepo.EmailExists(c.Request.Context(), req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify email"})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{"error": "email already in use"})
		return
	}

	tempPassword := util.GenerateTemporaryPassword()
	hash, err := bcrypt.GenerateFromPassword([]byte(tempPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create account"})
		return
	}

	role := models.Role(req.Role)
	userID, err := ctrl.userRepo.CreateStaffUser(c.Request.Context(), models.User{
		Email:        req.Email,
		PasswordHash: string(hash),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Phone:        req.Phone,
	}, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create account: " + err.Error()})
		return
	}

	user, err := ctrl.userRepo.FindByID(c.Request.Context(), userID)
	if err != nil || user == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch created user"})
		return
	}

	emailSent := false
	if ctrl.emailSvc != nil && ctrl.emailSvc.Configured() {
		subject, html := service.StaffWelcomeEmail(req.FirstName, roleLabels[role], req.Email, tempPassword, ctrl.publicURL)
		ctrl.emailSvc.SendAsync(req.Email, subject, html)
		emailSent = true
	}

	c.JSON(http.StatusCreated, CreateStaffUserResponse{
		User:              *user,
		TemporaryPassword: tempPassword,
		EmailSent:         emailSent,
	})
}

// CreateStudentRequest carries the fields for admin-provisioned student accounts.
type CreateStudentRequest struct {
	FirstName   string `json:"first_name"    binding:"required" example:"Jane"`
	LastName    string `json:"last_name"     binding:"required" example:"Doe"`
	Email       string `json:"email"         binding:"required,email" example:"jane@example.com"`
	Phone       string `json:"phone"         example:"+919876543210"`
	DateOfBirth string `json:"date_of_birth" example:"1998-05-20"`
}

// CreateStudent godoc
//
//	@Summary		Create a student account
//	@Description	Admin/staff-provisioned student account creation. Generates a temporary password, emails it to the new student, and returns it once in the response.
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CreateStudentRequest	true	"New student account details"
//	@Success		201		{object}	CreateStaffUserResponse
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		409		{object}	map[string]string	"Email already in use"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/student [post]
func (ctrl *UserController) CreateStudent(c *gin.Context) {
	var req CreateStudentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	exists, err := ctrl.userRepo.EmailExists(c.Request.Context(), req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify email"})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{"error": "email already in use"})
		return
	}

	tempPassword := util.GenerateTemporaryPassword()
	hash, err := bcrypt.GenerateFromPassword([]byte(tempPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create account"})
		return
	}

	userID, err := ctrl.userRepo.Register(c.Request.Context(), models.User{
		Email:        req.Email,
		PasswordHash: string(hash),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Phone:        req.Phone,
		DateOfBirth:  req.DateOfBirth,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create account: " + err.Error()})
		return
	}

	user, err := ctrl.userRepo.FindByID(c.Request.Context(), userID)
	if err != nil || user == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch created user"})
		return
	}

	emailSent := false
	if ctrl.emailSvc != nil && ctrl.emailSvc.Configured() {
		subject, html := service.StaffWelcomeEmail(req.FirstName, roleLabels[models.RoleStudent], req.Email, tempPassword, ctrl.publicURL)
		ctrl.emailSvc.SendAsync(req.Email, subject, html)
		emailSent = true
	}

	c.JSON(http.StatusCreated, CreateStaffUserResponse{
		User:              *user,
		TemporaryPassword: tempPassword,
		EmailSent:         emailSent,
	})
}

// GetAll godoc
//
//	@Summary		List users
//	@Description	Returns all active (non-deleted) users.
//	@Tags			users
//	@Produce		json
//	@Success		200	{array}		models.User
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users [get]
func (ctrl *UserController) GetAll(c *gin.Context) {
	users, err := ctrl.userRepo.FindAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch users"})
		return
	}
	if users == nil {
		users = []models.User{}
	}
	c.JSON(http.StatusOK, users)
}

// GetDeleted godoc
//
//	@Summary		List deleted users
//	@Description	Returns all soft-deleted users (where deleted_at is set).
//	@Tags			users
//	@Produce		json
//	@Success		200	{array}		models.User
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/deleted [get]
func (ctrl *UserController) GetDeleted(c *gin.Context) {
	users, err := ctrl.userRepo.FindDeleted(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch deleted users"})
		return
	}
	if users == nil {
		users = []models.User{}
	}
	c.JSON(http.StatusOK, users)
}

// GetMentors godoc
//
//	@Summary		List mentors
//	@Description	Returns all active mentors. Use this to populate the batch manager dropdown.
//	@Tags			users
//	@Produce		json
//	@Success		200	{array}		models.User
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/mentors [get]
func (ctrl *UserController) GetMentors(c *gin.Context) {
	users, err := ctrl.userRepo.FindByRole(c.Request.Context(), models.RoleMentor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch mentors"})
		return
	}
	if users == nil {
		users = []models.User{}
	}
	c.JSON(http.StatusOK, users)
}

// Search godoc
//
//	@Summary		Search users
//	@Description	Search active users by name (first, last, or full), email, phone, or user ID. Pass the term as query param `q`.
//	@Tags			users
//	@Produce		json
//	@Param			q	query		string	true	"Search term"
//	@Success		200	{array}		models.User
//	@Failure		400	{object}	map[string]string	"Missing query param"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/search [get]
func (ctrl *UserController) Search(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query param 'q' is required"})
		return
	}

	users, err := ctrl.userRepo.SearchUsers(c.Request.Context(), q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not search users"})
		return
	}
	if users == nil {
		users = []models.User{}
	}
	c.JSON(http.StatusOK, users)
}

// Update godoc
//
//	@Summary		Update user
//	@Description	Partially update a user — send only the fields you want to change (first_name, last_name, phone, date_of_birth).
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"User ID (UUID)"
//	@Param			body	body		models.UpdateUserInput	true	"Fields to update (all optional)"
//	@Success		200		{object}	models.User
//	@Failure		400		{object}	map[string]string	"No fields provided / validation error"
//	@Failure		404		{object}	map[string]string	"User not found"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id} [patch]
func (ctrl *UserController) Update(c *gin.Context) {
	id := c.Param("id")

	var input models.UpdateUserInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	role := c.GetString("role")
	isStaff := role == string(models.RoleSuperAdmin) || role == string(models.RoleTeamLead)
	if c.GetString("user_id") != id && !isStaff {
		c.JSON(http.StatusForbidden, gin.H{"error": "you can only update your own profile"})
		return
	}
	if input.IsActive != nil && !isStaff {
		c.JSON(http.StatusForbidden, gin.H{"error": "only staff can change account status"})
		return
	}

	if err := ctrl.userRepo.UpdateUser(c.Request.Context(), id, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update user"})
		return
	}

	user, err := ctrl.userRepo.FindByID(c.Request.Context(), id)
	if err != nil || user == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated user"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// ChangeRoleRequest holds the target role for a promotion.
type ChangeRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=student mentor employee team_lead" enums:"student,mentor,employee,team_lead" example:"mentor"`
}

// ChangeRole godoc
//
//	@Summary		Change user role
//	@Description	Promotes or changes a user's role. Only super_admin can call this. The old role-specific profile is deleted and a new one is created.
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"User ID (UUID)"
//	@Param			body	body		ChangeRoleRequest	true	"New role"
//	@Success		200		{object}	models.User
//	@Failure		400		{object}	map[string]string	"Invalid role"
//	@Failure		404		{object}	map[string]string	"User not found"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/role [patch]
func (ctrl *UserController) ChangeRole(c *gin.Context) {
	id := c.Param("id")

	var req ChangeRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.userRepo.ChangeUserRole(c.Request.Context(), id, models.Role(req.Role)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not change role"})
		return
	}

	user, err := ctrl.userRepo.FindByID(c.Request.Context(), id)
	if err != nil || user == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated user"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// Delete godoc
//
//	@Summary		Delete user
//	@Description	Soft-delete a user by ID (sets deleted_at; the row is retained in the database).
//	@Tags			users
//	@Produce		json
//	@Param			id	path	string	true	"User ID (UUID)"
//	@Success		204	"No Content"
//	@Failure		404	{object}	map[string]string	"User not found"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id} [delete]
func (ctrl *UserController) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := ctrl.userRepo.SoftDeleteUser(c.Request.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete user"})
		return
	}

	c.Status(http.StatusNoContent)
}
