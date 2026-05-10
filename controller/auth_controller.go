package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/auth"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"golang.org/x/crypto/bcrypt"
)

type AuthController struct {
	userRepo  *repository.UserRepository
	jwtSecret string
}

func NewAuthController(userRepo *repository.UserRepository, jwtSecret string) *AuthController {
	return &AuthController{userRepo: userRepo, jwtSecret: jwtSecret}
}

// ── Login ─────────────────────────────────────────────────────────────────────

// LoginRequest holds the credentials for any role.
type LoginRequest struct {
	Email    string `json:"email"    binding:"required,email" example:"user@example.com"`
	Password string `json:"password" binding:"required,min=6" example:"secret123"`
}

// LoginResponse is returned on successful authentication.
type LoginResponse struct {
	Token     string `json:"token"      example:"eyJhbGci..."`
	Role      string `json:"role"       example:"student"`
	UserID    string `json:"user_id"    example:"550e8400-e29b-41d4-a716-446655440000"`
	FirstName string `json:"first_name" example:"John"`
	LastName  string `json:"last_name"  example:"Doe"`
}

// Login godoc
//
//	@Summary		Login
//	@Description	Single login endpoint for all roles. Returns a JWT — send it as `Authorization: Bearer <token>` on protected requests.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		LoginRequest	true	"Email and password"
//	@Success		200		{object}	LoginResponse
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		401		{object}	map[string]string	"Invalid credentials"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Router			/auth/login [post]
func (ctrl *AuthController) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := ctrl.userRepo.FindByEmail(c.Request.Context(), req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	// Same message for "not found" and "wrong password" — avoids leaking registered emails.
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		return
	}

	token, err := auth.GenerateToken(user.ID, user.Email, string(user.Role), ctrl.jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token"})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		Token:     token,
		Role:      string(user.Role),
		UserID:    user.ID,
		FirstName: user.FirstName,
		LastName:  user.LastName,
	})
}

// ── Register ──────────────────────────────────────────────────────────────────

// RegisterRequest accepts the basic details needed to create any user account.
type RegisterRequest struct {
	Email       string `json:"email"         binding:"required,email"                                                         example:"user@example.com"`
	Password    string `json:"password"      binding:"required,min=8"                                                         example:"Secret@123"`
	FirstName   string `json:"first_name"    binding:"required"                                                               example:"John"`
	LastName    string `json:"last_name"     binding:"required"                                                               example:"Doe"`
	Phone       string `json:"phone"                                                                                          example:"+919876543210"`
	DateOfBirth string `json:"date_of_birth"                                                                                  example:"1998-05-20"`
	Role        string `json:"role"          binding:"required,oneof=student mentor employee team_lead super_admin" enums:"student,mentor,employee,team_lead,super_admin" example:"student"`
}

// RegisterResponse is returned on successful registration.
type RegisterResponse struct {
	Message string `json:"message" example:"registration successful"`
	UserID  string `json:"user_id" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// Register godoc
//
//	@Summary		Register
//	@Description	Register a new user. A unique user ID is generated automatically and returned in the response.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		RegisterRequest		true	"Registration payload"
//	@Success		201		{object}	RegisterResponse
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		409		{object}	map[string]string	"Email already registered"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Router			/auth/register [post]
func (ctrl *AuthController) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	exists, err := ctrl.userRepo.EmailExists(c.Request.Context(), req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	user := models.User{
		Email:        req.Email,
		PasswordHash: string(hash),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Phone:        req.Phone,
		DateOfBirth:  req.DateOfBirth,
		Role:         models.Role(req.Role),
	}

	userID, err := ctrl.userRepo.Register(c.Request.Context(), user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create account"})
		return
	}

	c.JSON(http.StatusCreated, RegisterResponse{
		Message: "registration successful",
		UserID:  userID,
	})
}
