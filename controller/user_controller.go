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
	userRepo       *repository.UserRepository
	collegeRepo    *repository.CollegeRepository
	enrollmentRepo *repository.EnrollmentRepository
	emailSvc       *service.EmailService
	publicURL      string
	auditLogRepo   *repository.AuditLogRepository
}

func NewUserController(userRepo *repository.UserRepository, collegeRepo *repository.CollegeRepository, enrollmentRepo *repository.EnrollmentRepository, emailSvc *service.EmailService, publicURL string, auditLogRepo *repository.AuditLogRepository) *UserController {
	return &UserController{userRepo: userRepo, collegeRepo: collegeRepo, enrollmentRepo: enrollmentRepo, emailSvc: emailSvc, publicURL: publicURL, auditLogRepo: auditLogRepo}
}

// stripStudentPIIForMentor removes a student's phone/email/date-of-birth from
// the response when the caller is a mentor. Mentors keep academic visibility
// (enrollments, attendance, scores) but should never see a student's contact
// or personal details — see the Mentor role lockdown requirements.
func stripStudentPIIForMentor(users []models.User, callerRole string) []models.User {
	if callerRole != string(models.RoleMentor) {
		return users
	}
	for i := range users {
		if users[i].Role != models.RoleStudent {
			continue
		}
		users[i].Email = ""
		users[i].Phone = ""
		users[i].DateOfBirth = ""
	}
	return users
}

// roleLabels maps a role to the human-readable label used in the welcome email.
var roleLabels = map[models.Role]string{
	models.RoleMentor:       "Mentor",
	models.RoleEmployee:     "Employee",
	models.RoleTeamLead:     "Team Lead",
	models.RoleStudent:      "Student",
	models.RoleCollegeAdmin: "College Admin",
	models.RoleCollegeStaff: "College Staff",
}

// CreateStaffUserRequest carries the fields for admin-provisioned staff accounts.
type CreateStaffUserRequest struct {
	FirstName string `json:"first_name" binding:"required" example:"Jane"`
	LastName  string `json:"last_name"  binding:"required" example:"Doe"`
	Email     string `json:"email"      binding:"required,email" example:"jane@example.com"`
	Phone     string `json:"phone"      example:"+919876543210"`
	Role      string `json:"role"       binding:"required,oneof=mentor employee team_lead college_admin college_staff" enums:"mentor,employee,team_lead,college_admin,college_staff" example:"employee"`
	// CollegeShortID is REQUIRED when role is college_admin/college_staff
	// (only super_admin may create those roles, and every College Admin must
	// be permanently linked to one specific college — there is no sensible
	// default). Ignored for mentor/employee/team_lead, which always land on
	// the Internal EdTech Platform.
	CollegeShortID string `json:"college_short_id" example:"use GET /colleges to pick a real short_id — required for college_admin/college_staff"`
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

	var collegeID string
	if role == models.RoleCollegeAdmin || role == models.RoleCollegeStaff {
		// Only super_admin may create these roles, and they must be
		// permanently linked to one specific college — no default.
		if c.GetString("role") != string(models.RoleSuperAdmin) {
			c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "only super_admin may create a college_admin/college_staff account"})
			return
		}
		if req.CollegeShortID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "college_short_id is required for college_admin/college_staff"})
			return
		}
		college, err := ctrl.collegeRepo.FindByShortID(c.Request.Context(), req.CollegeShortID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not resolve target college"})
			return
		}
		if college == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "college not found"})
			return
		}
		collegeID = college.ID
	} else {
		// mentor/employee/team_lead always land on the Internal EdTech
		// Platform — these are internal, platform-wide staff roles.
		collegeID, err = ctrl.collegeRepo.DefaultCollegeID(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not resolve default organization"})
			return
		}
	}
	userID, err := ctrl.userRepo.CreateStaffUser(c.Request.Context(), models.User{
		Email:        req.Email,
		PasswordHash: string(hash),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Phone:        req.Phone,
	}, role, collegeID)
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
	// CollegeShortID is only honored for super_admin callers (picks which
	// college this student belongs to; omit for the Internal EdTech
	// Platform). A College Admin/College Staff caller is always forced onto
	// their own college_id server-side, regardless of what's sent here — the
	// frontend must never show them a college picker.
	CollegeShortID string `json:"college_short_id" example:"use GET /colleges to pick a real short_id, or omit for the Internal EdTech Platform"`
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

	// super_admin may pick any college (or omit, defaulting to the Internal
	// EdTech Platform); a College Admin/College Staff caller is always
	// forced onto their own college_id, regardless of what they send.
	collegeID, err := resolveTargetCollege(c.Request.Context(), ctrl.collegeRepo, c.GetString("role"), c.GetString("college_id"), req.CollegeShortID)
	if err != nil {
		if errors.Is(err, repository.ErrCollegeScopeRequired) {
			c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope to create a student under"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not resolve target college: " + err.Error()})
		return
	}

	var registrationNo *int
	if n, ok := ctrl.collegeRepo.NextRegistrationNo(c.Request.Context(), collegeID); ok {
		registrationNo = &n
	}

	userID, err := ctrl.userRepo.Register(c.Request.Context(), models.User{
		Email:        req.Email,
		PasswordHash: string(hash),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Phone:        req.Phone,
		DateOfBirth:  req.DateOfBirth,
	}, collegeID, registrationNo)
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
	role := c.GetString("role")

	var users []models.User
	var err error
	if role == string(models.RoleMentor) {
		// A mentor's "Users" list is scoped to students in their own
		// batches only — never the full platform roster.
		users, err = ctrl.userRepo.FindStudentsForMentor(c.Request.Context(), c.GetString("user_id"))
	} else {
		collegeID, scopeErr := repository.CollegeFilter(role, c.GetString("college_id"))
		if scopeErr != nil {
			c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope"})
			return
		}
		users, err = ctrl.userRepo.FindAll(c.Request.Context(), collegeID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch users"})
		return
	}
	if users == nil {
		users = []models.User{}
	}
	users = stripStudentPIIForMentor(users, role)
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
	collegeID, err := repository.CollegeFilter(c.GetString("role"), c.GetString("college_id"))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope"})
		return
	}

	users, err := ctrl.userRepo.FindByRole(c.Request.Context(), models.RoleMentor, collegeID)
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

	role := c.GetString("role")

	var users []models.User
	var err error
	if role == string(models.RoleMentor) {
		// A mentor can only look up students in their own batches — never
		// any student on the platform just by knowing a name/email/ID.
		users, err = ctrl.userRepo.SearchStudentsForMentor(c.Request.Context(), c.GetString("user_id"), q)
	} else {
		collegeID, scopeErr := repository.CollegeFilter(role, c.GetString("college_id"))
		if scopeErr != nil {
			c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope"})
			return
		}
		users, err = ctrl.userRepo.SearchUsers(c.Request.Context(), q, collegeID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not search users"})
		return
	}
	if users == nil {
		users = []models.User{}
	}
	users = stripStudentPIIForMentor(users, role)
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

	before, err := ctrl.userRepo.FindByID(c.Request.Context(), id)
	if err != nil || before == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
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

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "update", EntityType: "user",
		EntityID: user.ID, EntityLabel: user.FirstName + " " + user.LastName,
		Metadata: userUpdateDiff(before, input),
	})

	c.JSON(http.StatusOK, user)
}

// userUpdateDiff builds a { field: {from, to} } metadata map for only the
// fields actually present in the request, so the audit log shows exactly
// what changed rather than a full before/after snapshot.
func userUpdateDiff(before *models.User, input models.UpdateUserInput) map[string]interface{} {
	diff := map[string]interface{}{}
	if input.FirstName != nil && *input.FirstName != before.FirstName {
		diff["first_name"] = map[string]string{"from": before.FirstName, "to": *input.FirstName}
	}
	if input.LastName != nil && *input.LastName != before.LastName {
		diff["last_name"] = map[string]string{"from": before.LastName, "to": *input.LastName}
	}
	if input.Phone != nil && *input.Phone != before.Phone {
		diff["phone"] = map[string]string{"from": before.Phone, "to": *input.Phone}
	}
	if input.DateOfBirth != nil && *input.DateOfBirth != before.DateOfBirth {
		diff["date_of_birth"] = map[string]string{"from": before.DateOfBirth, "to": *input.DateOfBirth}
	}
	if input.IsActive != nil && *input.IsActive != before.IsActive {
		diff["is_active"] = map[string]interface{}{"from": before.IsActive, "to": *input.IsActive}
	}
	return diff
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

	before, err := ctrl.userRepo.FindByID(c.Request.Context(), id)
	if err != nil || before == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
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

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "update", EntityType: "user",
		EntityID: user.ID, EntityLabel: user.FirstName + " " + user.LastName,
		Metadata: map[string]interface{}{"role": map[string]string{"from": string(before.Role), "to": string(user.Role)}},
	})

	c.JSON(http.StatusOK, user)
}

// TransferCollegeRequest carries the target college and an optional reason
// for a super_admin-initiated college transfer.
type TransferCollegeRequest struct {
	CollegeShortID string `json:"college_short_id" binding:"required" example:"ABCENG"`
	Reason         string `json:"reason"           example:"Student transferred to partner college"`
	Force          bool   `json:"force"            example:"false"`
}

// TransferCollege godoc
//
//	@Summary		Transfer a user to a different college
//	@Description	Moves a user (typically a student) to another college. Super_admin only. Rejects the transfer if the user has any active batch enrollment unless force=true is passed. Always writes a user_college_history audit row.
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"User ID (UUID)"
//	@Param			body	body		TransferCollegeRequest	true	"Target college"
//	@Success		200		{object}	models.User
//	@Failure		400		{object}	map[string]string	"Validation error, or active enrollments block the transfer without force"
//	@Failure		404		{object}	map[string]string	"User or college not found"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/college [patch]
func (ctrl *UserController) TransferCollege(c *gin.Context) {
	id := c.Param("id")

	var req TransferCollegeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := ctrl.userRepo.FindByID(c.Request.Context(), id)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	targetCollege, err := ctrl.collegeRepo.FindByShortID(c.Request.Context(), req.CollegeShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not resolve target college"})
		return
	}
	if targetCollege == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "college not found"})
		return
	}

	if user.Role == models.RoleStudent && !req.Force {
		hasActive, err := ctrl.enrollmentRepo.HasAnyActiveEnrollment(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not check active enrollments"})
			return
		}
		if hasActive {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "this student has active batch enrollments — pass force=true to transfer anyway",
			})
			return
		}
	}

	oldCollegeID := user.CollegeID
	if err := ctrl.userRepo.SetCollegeID(c.Request.Context(), id, targetCollege.ID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not transfer college"})
		return
	}
	ctrl.userRepo.RecordCollegeTransfer(c.Request.Context(), id, oldCollegeID, targetCollege.ID, c.GetString("user_id"), req.Reason)

	updated, err := ctrl.userRepo.FindByID(c.Request.Context(), id)
	if err != nil || updated == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated user"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "college_transfer", EntityType: "user",
		EntityID: updated.ID, EntityLabel: updated.FirstName + " " + updated.LastName,
		Metadata: map[string]interface{}{"old_college_id": oldCollegeID, "new_college_id": targetCollege.ID, "reason": req.Reason, "force": req.Force},
	})

	c.JSON(http.StatusOK, updated)
}

// ChangeEmailRequest holds the new login email.
type ChangeEmailRequest struct {
	Email string `json:"email" binding:"required,email" example:"new.address@example.com"`
}

// ChangeEmail godoc
//
//	@Summary		Change a user's login email
//	@Description	Changes a user's email — a sensitive change, restricted to super_admin/team_lead. Rejects the change if the new address is already in use.
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"User ID (UUID)"
//	@Param			body	body		ChangeEmailRequest	true	"New email"
//	@Success		200		{object}	models.User
//	@Failure		400		{object}	map[string]string	"Invalid email / already in use"
//	@Failure		404		{object}	map[string]string	"User not found"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/email [patch]
func (ctrl *UserController) ChangeEmail(c *gin.Context) {
	id := c.Param("id")

	var req ChangeEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	before, err := ctrl.userRepo.FindByID(c.Request.Context(), id)
	if err != nil || before == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if before.Email != req.Email {
		exists, err := ctrl.userRepo.EmailExists(c.Request.Context(), req.Email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify email"})
			return
		}
		if exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "this email is already in use"})
			return
		}
	}

	if err := ctrl.userRepo.UpdateEmail(c.Request.Context(), id, req.Email); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not change email"})
		return
	}

	user, err := ctrl.userRepo.FindByID(c.Request.Context(), id)
	if err != nil || user == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated user"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "update", EntityType: "user",
		EntityID: user.ID, EntityLabel: user.FirstName + " " + user.LastName,
		Metadata: map[string]interface{}{"email": map[string]string{"from": before.Email, "to": user.Email}},
	})

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

// AdminResetPasswordResponse returns the one-time-shown new temporary
// password and whether the notification email was sent successfully.
type AdminResetPasswordResponse struct {
	TemporaryPassword string `json:"temporary_password"`
	EmailSent         bool   `json:"email_sent"`
}

// ResetPassword godoc
//
//	@Summary		Admin-triggered password reset
//	@Description	Generates a new temporary password for a user and emails it to them — unlike /auth/forgot-password, this is triggered by staff, not the user themselves. Restricted to super_admin/team_lead.
//	@Tags			users
//	@Produce		json
//	@Param			id	path		string	true	"User ID (UUID)"
//	@Success		200	{object}	AdminResetPasswordResponse
//	@Failure		404	{object}	map[string]string	"User not found"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/reset-password [post]
func (ctrl *UserController) ResetPassword(c *gin.Context) {
	id := c.Param("id")

	user, err := ctrl.userRepo.FindByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch user"})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// college_admin/college_staff were only just widened onto this route —
	// unlike super_admin/team_lead (trusted platform-wide), they may only
	// reset a password for a user in their own college.
	role := c.GetString("role")
	if role == string(models.RoleCollegeAdmin) || role == string(models.RoleCollegeStaff) {
		callerCollegeID := c.GetString("college_id")
		if callerCollegeID == "" || user.CollegeID != callerCollegeID {
			c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "this user does not belong to your college"})
			return
		}
	}

	tempPassword := util.GenerateTemporaryPassword()
	hash, err := bcrypt.GenerateFromPassword([]byte(tempPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not reset password"})
		return
	}

	if err := ctrl.userRepo.UpdatePassword(c.Request.Context(), id, string(hash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not reset password"})
		return
	}

	emailSent := false
	if ctrl.emailSvc != nil && ctrl.emailSvc.Configured() {
		subject, html := service.AdminPasswordResetEmail(user.FirstName, user.Email, tempPassword, ctrl.publicURL)
		ctrl.emailSvc.SendAsync(user.Email, subject, html)
		emailSent = true
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "update", EntityType: "user",
		EntityID: user.ID, EntityLabel: user.FirstName + " " + user.LastName,
		Metadata: map[string]interface{}{"action": "admin_reset_password", "email_sent": emailSent},
	})

	c.JSON(http.StatusOK, AdminResetPasswordResponse{TemporaryPassword: tempPassword, EmailSent: emailSent})
}
