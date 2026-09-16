package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

// TeamController manages the team hierarchy — which Operations employee
// reports to which Team Lead. Team-building itself (add/remove member,
// browse every team) is Manager/Admin/Super Admin only; a Team Lead may only
// read their own team.
type TeamController struct {
	teamRepo *repository.TeamRepository
}

func NewTeamController(teamRepo *repository.TeamRepository) *TeamController {
	return &TeamController{teamRepo: teamRepo}
}

// AddMember godoc
//
//	@Summary		Add a team member
//	@Description	Assigns an employee to a Team Lead's team, moving them off any team they were previously on. Restricted to Manager/Admin/Super Admin.
//	@Tags			teams
//	@Accept			json
//	@Produce		json
//	@Param			team_lead_id	path		string					true	"Team Lead's user id"
//	@Param			body			body		models.AddTeamMemberInput	true	"Employee to add"
//	@Success		200				{object}	map[string]string
//	@Failure		400				{object}	map[string]string	"Validation error"
//	@Failure		500				{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/teams/{team_lead_id}/members [post]
func (ctrl *TeamController) AddMember(c *gin.Context) {
	teamLeadID := c.Param("team_lead_id")
	var input models.AddTeamMemberInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	performedBy := c.GetString("user_id")
	if err := ctrl.teamRepo.AddMember(c.Request.Context(), teamLeadID, input.MemberID, performedBy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not add team member: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "team member added"})
}

// RemoveMember godoc
//
//	@Summary		Remove a team member
//	@Description	Takes an employee off whatever team they're currently on. Restricted to Manager/Admin/Super Admin.
//	@Tags			teams
//	@Produce		json
//	@Param			member_id	path	string	true	"Employee's user id"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Not on a team"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/teams/members/{member_id} [delete]
func (ctrl *TeamController) RemoveMember(c *gin.Context) {
	memberID := c.Param("member_id")
	if err := ctrl.teamRepo.RemoveMember(c.Request.Context(), memberID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "employee is not currently on a team"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not remove team member"})
		return
	}
	c.Status(http.StatusNoContent)
}

// GetMembers godoc
//
//	@Summary		List a team's members
//	@Description	Returns everyone assigned to the given Team Lead. A Team Lead may only view their own team; Manager/Admin/Super Admin may view any.
//	@Tags			teams
//	@Produce		json
//	@Param			team_lead_id	path	string	true	"Team Lead's user id"
//	@Success		200				{array}	models.TeamMember
//	@Failure		403				{object}	map[string]string	"Not your team"
//	@Failure		500				{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/teams/{team_lead_id}/members [get]
func (ctrl *TeamController) GetMembers(c *gin.Context) {
	teamLeadID := c.Param("team_lead_id")

	role := c.GetString("role")
	department := c.GetString("department")
	isTeamBuilder := role == string(models.RoleSuperAdmin) || role == string(models.RoleAdmin) || department == "manager"
	if !isTeamBuilder && c.GetString("user_id") != teamLeadID {
		c.JSON(http.StatusForbidden, gin.H{"error": "you can only view your own team"})
		return
	}

	members, err := ctrl.teamRepo.GetMembers(c.Request.Context(), teamLeadID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch team members"})
		return
	}
	if members == nil {
		members = []models.TeamMember{}
	}
	c.JSON(http.StatusOK, members)
}

// GetTeamLeadOptions godoc
//
//	@Summary		List Team Lead options
//	@Description	Every active employee tagged department=team_lead — the picker list for "choose a Team Lead." Restricted to Manager/Admin/Super Admin.
//	@Tags			teams
//	@Produce		json
//	@Success		200	{array}	models.StaffOption
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/teams/leads [get]
func (ctrl *TeamController) GetTeamLeadOptions(c *gin.Context) {
	options, err := ctrl.teamRepo.GetStaffByDepartment(c.Request.Context(), "team_lead")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch team leads"})
		return
	}
	if options == nil {
		options = []models.StaffOption{}
	}
	c.JSON(http.StatusOK, options)
}

// GetEligibleMemberOptions godoc
//
//	@Summary		List Operations employees
//	@Description	Every active employee tagged department=operations — the picker list for "add a team member." Restricted to Manager/Admin/Super Admin.
//	@Tags			teams
//	@Produce		json
//	@Success		200	{array}	models.StaffOption
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/teams/eligible-members [get]
func (ctrl *TeamController) GetEligibleMemberOptions(c *gin.Context) {
	options, err := ctrl.teamRepo.GetStaffByDepartment(c.Request.Context(), "operations")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch eligible members"})
		return
	}
	if options == nil {
		options = []models.StaffOption{}
	}
	c.JSON(http.StatusOK, options)
}

// GetAll godoc
//
//	@Summary		List every team
//	@Description	Returns every Team Lead who currently has at least one member, each with their full member list. Restricted to Manager/Admin/Super Admin.
//	@Tags			teams
//	@Produce		json
//	@Success		200	{array}	models.Team
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/teams [get]
func (ctrl *TeamController) GetAll(c *gin.Context) {
	teams, err := ctrl.teamRepo.GetAllTeams(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch teams"})
		return
	}
	if teams == nil {
		teams = []models.Team{}
	}
	c.JSON(http.StatusOK, teams)
}
