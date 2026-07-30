package controller

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/auth"
	"github.com/umangagarwal/vedx-backend/middleware"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
	"github.com/umangagarwal/vedx-backend/util"
	"golang.org/x/crypto/bcrypt"
)

// streamCookieMaxAge matches the 24h expiry auth.GenerateToken hardcodes for
// the JWT itself, so the cookie and the token it carries expire together.
const streamCookieMaxAge = 24 * 60 * 60

// setStreamCookie issues the httpOnly cookie that authenticates video
// recording streaming (see middleware.JWTAuthCookieOrHeader) — a second
// delivery of the same JWT, since <video src> can't send a custom
// Authorization header the way normal API calls do. SameSite=None+Secure is
// required for it to work when the frontend is on a different origin than
// this API, which also means it requires HTTPS — it simply won't be set over
// plain HTTP in local dev, leaving the rest of the app (which uses the
// Bearer token) unaffected.
func setStreamCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteNoneMode)
	c.SetCookie(middleware.StreamCookieName, token, streamCookieMaxAge, "/", "", true, true)
}

// otpExpiry is how long a forgot-password or login OTP stays valid after being issued.
const otpExpiry = 5 * time.Minute

type AuthController struct {
	userRepo          *repository.UserRepository
	otpRepo           *repository.PasswordResetRepository
	loginOtpRepo      *repository.LoginOTPRepository
	loginActivityRepo *repository.LoginActivityRepository
	collegeRepo       *repository.CollegeRepository
	emailSvc          *service.EmailService
	jwtSecret         string
}

func NewAuthController(userRepo *repository.UserRepository, otpRepo *repository.PasswordResetRepository, loginOtpRepo *repository.LoginOTPRepository, loginActivityRepo *repository.LoginActivityRepository, collegeRepo *repository.CollegeRepository, emailSvc *service.EmailService, jwtSecret string) *AuthController {
	return &AuthController{userRepo: userRepo, otpRepo: otpRepo, loginOtpRepo: loginOtpRepo, loginActivityRepo: loginActivityRepo, collegeRepo: collegeRepo, emailSvc: emailSvc, jwtSecret: jwtSecret}
}

// ── Login ─────────────────────────────────────────────────────────────────────

// LoginRequest holds the credentials for any role.
type LoginRequest struct {
	Email    string `json:"email"    binding:"required,email" example:"user@example.com"`
	Password string `json:"password" binding:"required,min=6" example:"secret123"`
}

// LoginResponse is returned on successful authentication (after OTP verification).
type LoginResponse struct {
	Token     string `json:"token"      example:"eyJhbGci..."`
	Role      string `json:"role"       example:"student"`
	UserID    string `json:"user_id"    example:"550e8400-e29b-41d4-a716-446655440000"`
	FirstName string `json:"first_name" example:"John"`
	LastName  string `json:"last_name"  example:"Doe"`
}

// LoginOTPRequiredResponse is returned when credentials are valid — a
// verification code has been emailed and must be submitted to
// /auth/login/verify-otp to receive the JWT.
type LoginOTPRequiredResponse struct {
	Message string `json:"message" example:"verification code sent to your email"`
	Email   string `json:"email"   example:"user@example.com"`
}

// Login godoc
//
//	@Summary		Login (step 1 of 2) — verify credentials, send OTP
//	@Description	Verifies email/password for any role. On success, emails a 6-digit verification code (valid 5 minutes) and returns without a token — call POST /auth/login/verify-otp with that code to receive the JWT.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		LoginRequest	true	"Email and password"
//	@Success		200		{object}	LoginOTPRequiredResponse
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

	otp := util.GenerateOTP()
	otpHash, err := bcrypt.GenerateFromPassword([]byte(otp), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if err := ctrl.loginOtpRepo.CreateOTP(c.Request.Context(), user.ID, string(otpHash), time.Now().Add(otpExpiry)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if ctrl.emailSvc.Configured() {
		subject, html := service.LoginOTPEmail(otp)
		ctrl.emailSvc.SendAsync(user.Email, subject, html)
	}

	c.JSON(http.StatusOK, LoginOTPRequiredResponse{
		Message: "verification code sent to your email",
		Email:   user.Email,
	})
}

// ── Verify login OTP (step 2 of 2, issues the JWT) ────────────────────────────

// VerifyLoginOTPRequest carries the email and OTP emailed by /auth/login.
type VerifyLoginOTPRequest struct {
	Email string `json:"email" binding:"required,email" example:"user@example.com"`
	OTP   string `json:"otp"   binding:"required,len=6"  example:"042913"`
}

// VerifyLoginOTP godoc
//
//	@Summary		Login (step 2 of 2) — verify OTP, get JWT
//	@Description	Verifies the 6-digit code emailed by POST /auth/login and, if valid and unexpired, returns a JWT — send it as `Authorization: Bearer <token>` on protected requests. The OTP is single-use.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		VerifyLoginOTPRequest	true	"Email and verification code"
//	@Success		200		{object}	LoginResponse
//	@Failure		400		{object}	map[string]string	"Invalid or expired OTP / validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Router			/auth/login/verify-otp [post]
func (ctrl *AuthController) VerifyLoginOTP(c *gin.Context) {
	var req VerifyLoginOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	const invalidOTPMsg = "invalid or expired verification code"

	user, err := ctrl.userRepo.FindByEmail(c.Request.Context(), req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if user == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": invalidOTPMsg})
		return
	}

	record, err := ctrl.loginOtpRepo.FindLatestOTP(c.Request.Context(), user.ID)
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
	if err := ctrl.loginOtpRepo.MarkOTPUsed(c.Request.Context(), record.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
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
	setStreamCookie(c, token)

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

// Logout godoc
//
//	@Summary		Logout
//	@Description	Clears the httpOnly video-streaming session cookie set at login. The Bearer token itself is stateless and just discarded client-side as before — this only covers the cookie, which client-side JS can't clear on its own.
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/auth/logout [post]
func (ctrl *AuthController) Logout(c *gin.Context) {
	c.SetSameSite(http.SameSiteNoneMode)
	c.SetCookie(middleware.StreamCookieName, "", -1, "/", "", true, true)
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
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
