package controller

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type ProjectController struct {
	projectRepo      *repository.ProjectRepository
	batchRepo        *repository.BatchRepository
	notificationRepo *repository.NotificationRepository
}

func NewProjectController(projectRepo *repository.ProjectRepository, batchRepo *repository.BatchRepository, notificationRepo *repository.NotificationRepository) *ProjectController {
	return &ProjectController{projectRepo: projectRepo, batchRepo: batchRepo, notificationRepo: notificationRepo}
}

// ── Projects ─────────────────────────────────────────────────────────────────

// CreateProject godoc
//
//	@Summary		Create project
//	@Description	Create a new project scoped to a batch. Set is_team_project to true if students should submit as teams (create teams afterwards via POST /projects/{short_id}/teams). Restricted to super_admin / team_lead / mentor.
//	@Tags			projects
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateProjectInput	true	"Project details"
//	@Success		201		{object}	models.Project
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/projects [post]
func (ctrl *ProjectController) Create(c *gin.Context) {
	var input models.CreateProjectInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	createdBy := c.GetString("user_id")
	p, err := ctrl.projectRepo.Create(c.Request.Context(), input, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create project: " + err.Error()})
		return
	}

	if p.Status == "active" {
		ctrl.notifyPublished(c, p, createdBy)
	}

	c.JSON(http.StatusCreated, p)
}

func (ctrl *ProjectController) notifyPublished(c *gin.Context, p *models.Project, actorID string) {
	title := "New project: " + p.Title
	message := fmt.Sprintf("A new project %q has been published for batch %s. Final deadline: %s.", p.Title, p.BatchNumber, p.FinalDeadline.Format("Jan 2, 2006 3:04 PM"))

	students, err := ctrl.batchRepo.GetStudents(c.Request.Context(), p.BatchShortID)
	if err != nil {
		log.Printf("fetch batch students for project notify: %v", err)
	}
	recipients := make([]string, 0, len(students))
	for _, s := range students {
		recipients = append(recipients, s.UserID)
	}
	if err := ctrl.notificationRepo.NotifyUsers(c.Request.Context(),
		title, message, "project", "project", p.ShortID, actorID, recipients,
	); err != nil {
		log.Printf("notify project publish (students): %v", err)
	}
	if err := ctrl.notificationRepo.NotifyRoles(c.Request.Context(),
		title, message, "project", "project", p.ShortID, actorID,
		[]string{string(models.RoleTeamLead), string(models.RoleSuperAdmin)},
	); err != nil {
		log.Printf("notify project publish (team_lead/super_admin): %v", err)
	}
}

// GetAllProjects godoc
//
//	@Summary		List projects
//	@Description	Returns projects scoped to the caller's role — students see published projects for their enrolled batches; mentors see projects for batches they manage; team_lead/super_admin see everything.
//	@Tags			projects
//	@Produce		json
//	@Param			batch_short_id	query	string	false	"Filter by batch short ID"
//	@Param			status			query	string	false	"Filter by status: draft | active | closed"
//	@Success		200	{array}		models.Project
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/projects [get]
func (ctrl *ProjectController) GetAll(c *gin.Context) {
	var filter models.ProjectFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	role := c.GetString("role")
	userID := c.GetString("user_id")

	var projects []models.Project
	var err error

	switch role {
	case string(models.RoleStudent):
		projects, err = ctrl.projectRepo.FindAllForStudent(c.Request.Context(), userID)
	case string(models.RoleMentor), string(models.RoleEmployee):
		projects, err = ctrl.projectRepo.FindAllForMentor(c.Request.Context(), userID)
	default:
		projects, err = ctrl.projectRepo.FindAll(c.Request.Context(), filter)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch projects"})
		return
	}
	if projects == nil {
		projects = []models.Project{}
	}
	c.JSON(http.StatusOK, projects)
}

// GetProject godoc
//
//	@Summary		Get project
//	@Description	Returns a single project by its short_id, with its milestones and (if applicable) teams embedded.
//	@Tags			projects
//	@Produce		json
//	@Param			short_id	path		string	true	"Project short ID"
//	@Success		200			{object}	models.ProjectDetail
//	@Failure		404			{object}	map[string]string	"Project not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/projects/{short_id} [get]
func (ctrl *ProjectController) GetByShortID(c *gin.Context) {
	shortID := c.Param("short_id")

	p, err := ctrl.projectRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch project"})
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	milestones, err := ctrl.projectRepo.GetMilestones(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch milestones"})
		return
	}
	if milestones == nil {
		milestones = []models.ProjectMilestone{}
	}

	detail := models.ProjectDetail{Project: *p, Milestones: milestones}

	if p.IsTeamProject {
		teams, err := ctrl.projectRepo.GetTeams(c.Request.Context(), shortID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch teams"})
			return
		}
		if teams == nil {
			teams = []models.ProjectTeam{}
		}
		detail.Teams = teams
	}

	c.JSON(http.StatusOK, detail)
}

// UpdateProject godoc
//
//	@Summary		Update project
//	@Description	Partially update a project. All fields are optional. Restricted to super_admin / team_lead / mentor.
//	@Tags			projects
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string						true	"Project short ID"
//	@Param			body		body		models.UpdateProjectInput	true	"Fields to update"
//	@Success		200			{object}	models.Project
//	@Failure		400			{object}	map[string]string	"Validation error or no fields provided"
//	@Failure		404			{object}	map[string]string	"Project not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/projects/{short_id} [patch]
func (ctrl *ProjectController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateProjectInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.projectRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update project"})
		return
	}

	p, err := ctrl.projectRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || p == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated project"})
		return
	}
	c.JSON(http.StatusOK, p)
}

// DeleteProject godoc
//
//	@Summary		Delete project
//	@Description	Soft-deletes a project by its short ID. Restricted to super_admin / team_lead / mentor.
//	@Tags			projects
//	@Produce		json
//	@Param			short_id	path	string	true	"Project short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Project not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/projects/{short_id} [delete]
func (ctrl *ProjectController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")
	if err := ctrl.projectRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete project"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ── Milestones ───────────────────────────────────────────────────────────────

// AddMilestone godoc
//
//	@Summary		Add milestone
//	@Description	Add a milestone/checkpoint to a project (e.g. Topic Approval, Design Submission, Final Submission). Mark exactly one milestone is_final to designate the final deliverable. Restricted to super_admin / team_lead / mentor.
//	@Tags			projects
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"Project short ID"
//	@Param			body		body		models.CreateMilestoneInput	true	"Milestone details"
//	@Success		201			{object}	models.ProjectMilestone
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/projects/{short_id}/milestones [post]
func (ctrl *ProjectController) AddMilestone(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.CreateMilestoneInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	m, err := ctrl.projectRepo.AddMilestone(c.Request.Context(), shortID, input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not add milestone: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, m)
}

// UpdateMilestone godoc
//
//	@Summary		Update milestone
//	@Description	Partially update a milestone. Restricted to super_admin / team_lead / mentor.
//	@Tags			projects
//	@Accept			json
//	@Produce		json
//	@Param			short_id			path	string							true	"Project short ID"
//	@Param			milestone_short_id	path	string							true	"Milestone short ID"
//	@Param			body				body	models.UpdateMilestoneInput	true	"Fields to update"
//	@Success		204					"No Content"
//	@Failure		400					{object}	map[string]string	"Validation error or no fields provided"
//	@Failure		404					{object}	map[string]string	"Milestone not found"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/projects/{short_id}/milestones/{milestone_short_id} [patch]
func (ctrl *ProjectController) UpdateMilestone(c *gin.Context) {
	milestoneShortID := c.Param("milestone_short_id")

	var input models.UpdateMilestoneInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.projectRepo.UpdateMilestone(c.Request.Context(), milestoneShortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "milestone not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update milestone"})
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteMilestone godoc
//
//	@Summary		Delete milestone
//	@Description	Soft-deletes a milestone. Restricted to super_admin / team_lead / mentor.
//	@Tags			projects
//	@Produce		json
//	@Param			short_id			path	string	true	"Project short ID"
//	@Param			milestone_short_id	path	string	true	"Milestone short ID"
//	@Success		204					"No Content"
//	@Failure		404					{object}	map[string]string	"Milestone not found"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/projects/{short_id}/milestones/{milestone_short_id} [delete]
func (ctrl *ProjectController) DeleteMilestone(c *gin.Context) {
	milestoneShortID := c.Param("milestone_short_id")
	if err := ctrl.projectRepo.DeleteMilestone(c.Request.Context(), milestoneShortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "milestone not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete milestone"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ── Teams ────────────────────────────────────────────────────────────────────

// CreateTeam godoc
//
//	@Summary		Create project team
//	@Description	Create a team for a team-based project, optionally with initial student members. Restricted to super_admin / team_lead / mentor.
//	@Tags			projects
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string					true	"Project short ID"
//	@Param			body		body		models.CreateTeamInput	true	"Team details"
//	@Success		201			{object}	models.ProjectTeam
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/projects/{short_id}/teams [post]
func (ctrl *ProjectController) CreateTeam(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.CreateTeamInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	t, err := ctrl.projectRepo.CreateTeam(c.Request.Context(), shortID, input, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create team: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, t)
}

// DeleteTeam godoc
//
//	@Summary		Delete project team
//	@Description	Soft-deletes a team. Restricted to super_admin / team_lead / mentor.
//	@Tags			projects
//	@Produce		json
//	@Param			short_id		path	string	true	"Project short ID"
//	@Param			team_short_id	path	string	true	"Team short ID"
//	@Success		204				"No Content"
//	@Failure		404				{object}	map[string]string	"Team not found"
//	@Failure		500				{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/projects/{short_id}/teams/{team_short_id} [delete]
func (ctrl *ProjectController) DeleteTeam(c *gin.Context) {
	teamShortID := c.Param("team_short_id")
	if err := ctrl.projectRepo.DeleteTeam(c.Request.Context(), teamShortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "team not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete team"})
		return
	}
	c.Status(http.StatusNoContent)
}

// AddTeamMembers godoc
//
//	@Summary		Add team members
//	@Description	Add one or more students to a project team. Restricted to super_admin / team_lead / mentor.
//	@Tags			projects
//	@Accept			json
//	@Produce		json
//	@Param			short_id		path		string						true	"Project short ID"
//	@Param			team_short_id	path		string						true	"Team short ID"
//	@Param			body			body		models.AddTeamMembersInput	true	"Student IDs to add"
//	@Success		204				"No Content"
//	@Failure		400				{object}	map[string]string	"Validation error"
//	@Failure		500				{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/projects/{short_id}/teams/{team_short_id}/members [post]
func (ctrl *ProjectController) AddTeamMembers(c *gin.Context) {
	teamShortID := c.Param("team_short_id")

	var input models.AddTeamMembersInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.projectRepo.AddTeamMembers(c.Request.Context(), teamShortID, input.StudentIDs, c.GetString("user_id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not add team members: " + err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// RemoveTeamMember godoc
//
//	@Summary		Remove team member
//	@Description	Removes a student from a project team. Restricted to super_admin / team_lead / mentor.
//	@Tags			projects
//	@Produce		json
//	@Param			short_id		path	string	true	"Project short ID"
//	@Param			team_short_id	path	string	true	"Team short ID"
//	@Param			user_id			path	string	true	"Student user ID"
//	@Success		204				"No Content"
//	@Failure		404				{object}	map[string]string	"Member not found"
//	@Failure		500				{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/projects/{short_id}/teams/{team_short_id}/members/{user_id} [delete]
func (ctrl *ProjectController) RemoveTeamMember(c *gin.Context) {
	teamShortID := c.Param("team_short_id")
	userID := c.Param("user_id")
	if err := ctrl.projectRepo.RemoveTeamMember(c.Request.Context(), teamShortID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "member not found in team"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not remove team member"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ── Submissions ──────────────────────────────────────────────────────────────

// CreateSubmission godoc
//
//	@Summary		Submit milestone work
//	@Description	Submit (or resubmit, if the mentor has requested one) work for a milestone. If the project is team-based, the calling student must already belong to a team for this project. Upload files first via POST /upload/project-file and pass the URL as file_url.
//	@Tags			projects
//	@Accept			json
//	@Produce		json
//	@Param			short_id			path		string								true	"Project short ID"
//	@Param			milestone_short_id	path		string								true	"Milestone short ID"
//	@Param			body				body		models.CreateProjectSubmissionInput	true	"Submission details"
//	@Success		201					{object}	models.ProjectSubmission
//	@Failure		400					{object}	map[string]string	"Validation error, not on a team, or already submitted"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/projects/{short_id}/milestones/{milestone_short_id}/submissions [post]
func (ctrl *ProjectController) CreateSubmission(c *gin.Context) {
	projectShortID := c.Param("short_id")
	milestoneShortID := c.Param("milestone_short_id")
	studentID := c.GetString("user_id")

	var input models.CreateProjectSubmissionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	p, err := ctrl.projectRepo.FindByShortID(c.Request.Context(), projectShortID)
	if err != nil || p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	teamID := ""
	individualID := studentID
	if p.IsTeamProject {
		teamShortID, err := ctrl.projectRepo.FindStudentTeam(c.Request.Context(), projectShortID, studentID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not resolve team"})
			return
		}
		if teamShortID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "you're not assigned to a team for this project yet"})
			return
		}
		teamID = teamShortID
		individualID = ""
	}

	submission, err := ctrl.projectRepo.CreateOrResubmitSubmission(c.Request.Context(), milestoneShortID, individualID, teamID, input)
	if err != nil {
		if err.Error() == "already submitted" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "already submitted for this milestone; ask your mentor to request a resubmission"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not submit: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, submission)
}

// GetMySubmission godoc
//
//	@Summary		Get my milestone submission
//	@Description	Returns the calling student's (or their team's) submission for a milestone, if any.
//	@Tags			projects
//	@Produce		json
//	@Param			short_id			path		string	true	"Project short ID"
//	@Param			milestone_short_id	path		string	true	"Milestone short ID"
//	@Success		200					{object}	models.ProjectSubmission
//	@Failure		404					{object}	map[string]string	"No submission yet"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/projects/{short_id}/milestones/{milestone_short_id}/submissions/me [get]
func (ctrl *ProjectController) GetMySubmission(c *gin.Context) {
	projectShortID := c.Param("short_id")
	milestoneShortID := c.Param("milestone_short_id")
	studentID := c.GetString("user_id")

	p, err := ctrl.projectRepo.FindByShortID(c.Request.Context(), projectShortID)
	if err != nil || p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	teamID := ""
	individualID := studentID
	if p.IsTeamProject {
		teamShortID, err := ctrl.projectRepo.FindStudentTeam(c.Request.Context(), projectShortID, studentID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not resolve team"})
			return
		}
		teamID = teamShortID
		individualID = ""
	}

	s, err := ctrl.projectRepo.FindMySubmission(c.Request.Context(), milestoneShortID, individualID, teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch submission"})
		return
	}
	if s == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no submission yet"})
		return
	}
	c.JSON(http.StatusOK, s)
}

// GetAllSubmissions godoc
//
//	@Summary		List milestone submissions
//	@Description	Returns every submission for a milestone, newest first. Restricted to super_admin / team_lead / mentor.
//	@Tags			projects
//	@Produce		json
//	@Param			short_id			path		string	true	"Project short ID"
//	@Param			milestone_short_id	path		string	true	"Milestone short ID"
//	@Success		200					{array}		models.ProjectSubmission
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/projects/{short_id}/milestones/{milestone_short_id}/submissions [get]
func (ctrl *ProjectController) GetAllSubmissions(c *gin.Context) {
	milestoneShortID := c.Param("milestone_short_id")
	submissions, err := ctrl.projectRepo.FindAllSubmissions(c.Request.Context(), milestoneShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch submissions"})
		return
	}
	if submissions == nil {
		submissions = []models.ProjectSubmission{}
	}
	c.JSON(http.StatusOK, submissions)
}

// GradeSubmission godoc
//
//	@Summary		Grade milestone submission
//	@Description	Records marks and feedback for a milestone submission, or requests a resubmission. Restricted to super_admin / team_lead / mentor.
//	@Tags			projects
//	@Accept			json
//	@Produce		json
//	@Param			short_id			path	string								true	"Project short ID"
//	@Param			milestone_short_id	path	string								true	"Milestone short ID"
//	@Param			submission_short_id	path	string								true	"Submission short ID"
//	@Param			body				body	models.GradeProjectSubmissionInput	true	"Grade details"
//	@Success		204					"No Content"
//	@Failure		400					{object}	map[string]string	"Validation error"
//	@Failure		404					{object}	map[string]string	"Submission not found"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/projects/{short_id}/milestones/{milestone_short_id}/submissions/{submission_short_id} [patch]
func (ctrl *ProjectController) GradeSubmission(c *gin.Context) {
	milestoneShortID := c.Param("milestone_short_id")
	submissionShortID := c.Param("submission_short_id")

	var input models.GradeProjectSubmissionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.projectRepo.GradeSubmission(c.Request.Context(), milestoneShortID, submissionShortID, c.GetString("user_id"), input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not grade submission"})
		return
	}
	c.Status(http.StatusNoContent)
}
