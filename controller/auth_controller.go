package controller

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/auth"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
	"github.com/umangagarwal/vedx-backend/util"
	"golang.org/x/crypto/bcrypt"
)

// otpExpiry is how long a forgot-password OTP stays valid after being issued.
const otpExpiry = 5 * time.Minute

type AuthController struct {
	userRepo          *repository.UserRepository
	otpRepo           *repository.PasswordResetRepository
	loginActivityRepo *repository.LoginActivityRepository
	collegeRepo       *repository.CollegeRepository
	emailSvc          *service.EmailService
	jwtSecret         string
}

func NewAuthController(userRepo *repository.UserRepository, otpRepo *repository.PasswordResetRepository, loginActivityRepo *repository.LoginActivityRepository, collegeRepo *repository.CollegeRepository, emailSvc *service.EmailService, jwtSecret string) *AuthController {
	return &AuthController{userRepo: userRepo, otpRepo: otpRepo, loginActivityRepo: loginActivityRepo, collegeRepo: collegeRepo, emailSvc: emailSvc, jwtSecret: jwtSecret}
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
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

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

	// Best-effort — a query failure here (e.g. this column's migration not
	// applied yet) must never block login; it just means this session is
	// treated as unscoped/full-access, same as before multi-tenancy existed.
	collegeID, err := ctrl.userRepo.GetCollegeID(c.Request.Context(), user.ID)
	if err != nil {
		log.Printf("fetch college_id for login: %v", err)
		collegeID = ""
	}

	token, err := auth.GenerateToken(user.ID, user.Email, string(user.Role), collegeID, ctrl.jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token"})
		return
	}

	// Best-effort device/login tracking — never blocks or fails the login itself.
	if deviceID := c.GetHeader("X-Device-Id"); deviceID != "" && ctrl.loginActivityRepo != nil {
		ua := util.ParseUserAgent(c.GetHeader("User-Agent"))
		go func() {
			_ = ctrl.loginActivityRepo.Upsert(context.Background(), user.ID, deviceID, ua)
		}()
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

// RegisterRequest accepts the basic details needed to create a student account.
type RegisterRequest struct {
	Email       string `json:"email"         binding:"required,email" example:"user@example.com"`
	Password    string `json:"password"      binding:"required,min=8"  example:"Secret@123"`
	FirstName   string `json:"first_name"    binding:"required"         example:"John"`
	LastName    string `json:"last_name"     binding:"required"         example:"Doe"`
	Phone       string `json:"phone"         binding:"required"         example:"+919876543210"`
	DateOfBirth string `json:"date_of_birth"                            example:"1998-05-20"`
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
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

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
		Role:         models.RoleStudent,
	}

	// Public self-registration is only ever for direct EdTech students — the
	// form has no college picker, and every self-registered account lands on
	// the Internal EdTech Platform. External college students are never
	// created through this endpoint (see CollegeAdmin-provisioned creation).
	collegeID, err := ctrl.collegeRepo.DefaultCollegeID(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not resolve default organization"})
		return
	}

	var registrationNo *int
	if n, ok := ctrl.collegeRepo.NextRegistrationNo(c.Request.Context(), collegeID); ok {
		registrationNo = &n
	}

	userID, err := ctrl.userRepo.Register(c.Request.Context(), user, collegeID, registrationNo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create account"})
		return
	}

	c.JSON(http.StatusCreated, RegisterResponse{
		Message: "registration successful",
		UserID:  userID,
	})
}

// ── Change password (logged in, requires old password) ───────────────────────

// ChangePasswordRequest carries the old and new password for an authenticated user.
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"         example:"secret123"`
	NewPassword string `json:"new_password" binding:"required,min=8"   example:"NewSecret@123"`
}

// ChangePassword godoc
//
//	@Summary		Change password
//	@Description	Change the logged-in user's password. Requires the current password.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		ChangePasswordRequest	true	"Old and new password"
//	@Success		200		{object}	map[string]string	"Password changed"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		401		{object}	map[string]string	"Incorrect current password"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/auth/change-password [post]
func (ctrl *AuthController) ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.GetString("user_id")

	currentHash, err := ctrl.userRepo.FindPasswordHashByID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "current password is incorrect"})
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if err := ctrl.userRepo.UpdatePassword(c.Request.Context(), userID, string(newHash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password changed successfully"})
}

// ── Forgot password (public, sends an OTP by email) ───────────────────────────

// ForgotPasswordRequest carries the email to send a reset OTP to.
type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email" example:"user@example.com"`
}

// ForgotPassword godoc
//
//	@Summary		Forgot password — request OTP
//	@Description	Sends a 6-digit OTP to the given email if it belongs to a registered account. Always returns 200, whether or not the email is registered, to avoid leaking which emails have accounts. The OTP expires after 5 minutes.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		ForgotPasswordRequest	true	"Account email"
//	@Success		200		{object}	map[string]string	"OTP sent if the email is registered"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Router			/auth/forgot-password [post]
func (ctrl *AuthController) ForgotPassword(c *gin.Context) {
	var req ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	const genericResponse = "if that email is registered, an OTP has been sent"

	user, err := ctrl.userRepo.FindByEmail(c.Request.Context(), req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if user == nil {
		// Same response as the success path — don't reveal whether the email is registered.
		c.JSON(http.StatusOK, gin.H{"message": genericResponse})
		return
	}

	otp := util.GenerateOTP()
	otpHash, err := bcrypt.GenerateFromPassword([]byte(otp), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if err := ctrl.otpRepo.CreateOTP(c.Request.Context(), user.ID, string(otpHash), time.Now().Add(otpExpiry)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if ctrl.emailSvc.Configured() {
		subject, html := service.ForgotPasswordOTPEmail(otp)
		ctrl.emailSvc.SendAsync(user.Email, subject, html)
	}

	c.JSON(http.StatusOK, gin.H{"message": genericResponse})
}

// ── Reset password with OTP (public) ──────────────────────────────────────────

// ResetPasswordRequest carries the email, OTP, and new password for the forgot-password flow.
type ResetPasswordRequest struct {
	Email       string `json:"email"        binding:"required,email"     example:"user@example.com"`
	OTP         string `json:"otp"          binding:"required,len=6"     example:"042913"`
	NewPassword string `json:"new_password" binding:"required,min=8"     example:"NewSecret@123"`
}

// ResetPassword godoc
//
//	@Summary		Reset password with OTP
//	@Description	Verifies the OTP emailed via /auth/forgot-password and sets a new password. The OTP is single-use and expires 5 minutes after being issued.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		ResetPasswordRequest	true	"Email, OTP, and new password"
//	@Success		200		{object}	map[string]string	"Password reset"
//	@Failure		400		{object}	map[string]string	"Invalid or expired OTP / validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Router			/auth/reset-password [post]
func (ctrl *AuthController) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	const invalidOTPMsg = "invalid or expired OTP"

	user, err := ctrl.userRepo.FindByEmail(c.Request.Context(), req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if user == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": invalidOTPMsg})
		return
	}

	record, err := ctrl.otpRepo.FindLatestOTP(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if record == nil || record.UsedAt != nil || time.Now().After(record.ExpiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": invalidOTPMsg})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(record.OTPHash), []byte(req.OTP)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": invalidOTPMsg})
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if err := ctrl.userRepo.UpdatePassword(c.Request.Context(), user.ID, string(newHash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update password"})
		return
	}

	if err := ctrl.otpRepo.MarkOTPUsed(c.Request.Context(), record.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password reset successfully"})
}
