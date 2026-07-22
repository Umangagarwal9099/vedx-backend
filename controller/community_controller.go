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
	"github.com/umangagarwal/vedx-backend/service"
)

type CommunityController struct {
	communityRepo    *repository.CommunityRepository
	notificationRepo *repository.NotificationRepository
	userRepo         *repository.UserRepository
	batchRepo        *repository.BatchRepository
	emailSvc         *service.EmailService
}

func NewCommunityController(communityRepo *repository.CommunityRepository, notificationRepo *repository.NotificationRepository, userRepo *repository.UserRepository, batchRepo *repository.BatchRepository, emailSvc *service.EmailService) *CommunityController {
	return &CommunityController{communityRepo: communityRepo, notificationRepo: notificationRepo, userRepo: userRepo, batchRepo: batchRepo, emailSvc: emailSvc}
}

// CreateCommunity godoc
//
//	@Summary		Create community
//	@Description	Create a new community scoped to a batch. Restricted to super_admin / team_lead / mentor. A short unique ID is generated automatically.
//	@Tags			communities
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateCommunityInput	true	"Community details"
//	@Success		201		{object}	models.Community
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		403		{object}	map[string]string	"Forbidden"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/communities [post]
func (ctrl *CommunityController) Create(c *gin.Context) {
	var input models.CreateCommunityInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, input.BatchShortID) {
		return
	}

	createdBy := c.GetString("user_id")

	community, err := ctrl.communityRepo.Create(c.Request.Context(), input, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create community: " + err.Error()})
		return
	}

	// Best-effort — students are usually already enrolled in the batch by
	// the time its community gets created, so without this they'd never
	// see it unless someone happened to re-add them to the batch later.
	if err := ctrl.communityRepo.BackfillMembersFromBatch(c.Request.Context(), community.ShortID, createdBy); err != nil {
		log.Printf("backfill community members from batch: %v", err)
	}

	if err := ctrl.notificationRepo.NotifyRoles(c.Request.Context(),
		"New community: "+community.Name,
		fmt.Sprintf("A new community %q has been created for batch %q.", community.Name, community.BatchNumber),
		"community", "community", community.ShortID, createdBy,
		[]string{"mentor", "team_lead"},
	); err != nil {
		log.Printf("notify community create: %v", err)
	}

	if ctrl.emailSvc.Configured() {
		subject, html := service.CommunityCreatedEmail(community.Name, community.BatchNumber)
		emailUsersByRoles(c.Request.Context(), ctrl.userRepo, ctrl.emailSvc, []models.Role{models.RoleMentor, models.RoleTeamLead}, subject, html)
	}

	c.JSON(http.StatusCreated, community)
}

// GetAllCommunities godoc
//
//	@Summary		List communities
//	@Description	Returns all non-deleted communities with batch details and a live member count.
//	@Tags			communities
//	@Produce		json
//	@Success		200	{array}		models.Community
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/communities [get]
func (ctrl *CommunityController) GetAll(c *gin.Context) {
	communities, err := ctrl.communityRepo.FindAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch communities"})
		return
	}
	if communities == nil {
		communities = []models.Community{}
	}
	c.JSON(http.StatusOK, communities)
}

// GetMyCommunities godoc
//
//	@Summary		List my communities
//	@Description	Returns the active communities the calling user is a member of — used by the student Community page, scoped to their own batch(es) instead of every community on the platform.
//	@Tags			communities
//	@Produce		json
//	@Success		200	{array}		models.Community
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/communities/me [get]
func (ctrl *CommunityController) GetMyCommunities(c *gin.Context) {
	communities, err := ctrl.communityRepo.FindAllForUser(c.Request.Context(), c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch communities"})
		return
	}
	if communities == nil {
		communities = []models.Community{}
	}
	c.JSON(http.StatusOK, communities)
}

// GetCommunity godoc
//
//	@Summary		Get community
//	@Description	Returns a single non-deleted community by its short_id, with a live member count.
//	@Tags			communities
//	@Produce		json
//	@Param			short_id	path		string	true	"Community short ID"
//	@Success		200			{object}	models.Community
//	@Failure		404			{object}	map[string]string	"Community not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/communities/{short_id} [get]
func (ctrl *CommunityController) GetByShortID(c *gin.Context) {
	shortID := c.Param("short_id")

	community, err := ctrl.communityRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch community"})
		return
	}
	if community == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "community not found"})
		return
	}
	c.JSON(http.StatusOK, community)
}

// UpdateCommunity godoc
//
//	@Summary		Update community
//	@Description	Partially update a community by its short_id. Send only the fields you want to change.
//	@Tags			communities
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"Community short ID"
//	@Param			body		body		models.UpdateCommunityInput	true	"Fields to update (all optional)"
//	@Success		200			{object}	models.Community
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Community not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/communities/{short_id} [patch]
func (ctrl *CommunityController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	existing, err := ctrl.communityRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch community"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "community not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, existing.BatchShortID) {
		return
	}

	var input models.UpdateCommunityInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.BatchShortID != nil && !checkBatchAccess(c, ctrl.batchRepo, *input.BatchShortID) {
		return
	}

	if err := ctrl.communityRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "community not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update community"})
		return
	}

	community, err := ctrl.communityRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || community == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated community"})
		return
	}

	c.JSON(http.StatusOK, community)
}

// DeleteCommunity godoc
//
//	@Summary		Delete community
//	@Description	Soft-delete a community by its short_id.
//	@Tags			communities
//	@Produce		json
//	@Param			short_id	path	string	true	"Community short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Community not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/communities/{short_id} [delete]
func (ctrl *CommunityController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")

	existing, err := ctrl.communityRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch community"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "community not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, existing.BatchShortID) {
		return
	}

	if err := ctrl.communityRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "community not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete community"})
		return
	}

	c.Status(http.StatusNoContent)
}

// AddCommunityMembers godoc
//
//	@Summary		Add community members
//	@Description	Add one or more users (students, mentors, admins, etc.) to a community by user ID. Users already in the community are left unchanged.
//	@Tags			communities
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"Community short ID"
//	@Param			body		body		models.AddCommunityMembersInput	true	"User IDs to add"
//	@Success		200			{object}	map[string]int	"Number of members added"
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/communities/{short_id}/members [post]
func (ctrl *CommunityController) AddMembers(c *gin.Context) {
	shortID := c.Param("short_id")

	existing, err := ctrl.communityRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch community"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "community not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, existing.BatchShortID) {
		return
	}

	var input models.AddCommunityMembersInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	addedBy := c.GetString("user_id")

	added, err := ctrl.communityRepo.AddMembers(c.Request.Context(), shortID, input.UserIDs, addedBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not add members: " + err.Error()})
		return
	}

	if len(added) > 0 {
		community, err := ctrl.communityRepo.FindByShortID(c.Request.Context(), shortID)
		if err == nil && community != nil {
			if err := ctrl.notificationRepo.NotifyUsers(c.Request.Context(),
				"Added to community: "+community.Name,
				fmt.Sprintf("You've been added to the community %q.", community.Name),
				"community", "community", shortID, addedBy, added,
			); err != nil {
				log.Printf("notify community add members: %v", err)
			}

			if ctrl.emailSvc.Configured() {
				subject, html := service.CommunityMemberAddedEmail(community.Name)
				for _, userID := range added {
					user, err := ctrl.userRepo.FindByID(c.Request.Context(), userID)
					if err != nil {
						log.Printf("fetch user for community member email: %v", err)
						continue
					}
					if user != nil && user.Email != "" {
						ctrl.emailSvc.SendAsync(user.Email, subject, html)
					}
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"added": len(added)})
}

// GetCommunityMembers godoc
//
//	@Summary		List community members
//	@Description	Returns every member of a community along with the total member count.
//	@Tags			communities
//	@Produce		json
//	@Param			short_id	path	string	true	"Community short ID"
//	@Success		200			{object}	map[string]interface{}	"total_members and members[]"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/communities/{short_id}/members [get]
func (ctrl *CommunityController) GetMembers(c *gin.Context) {
	shortID := c.Param("short_id")

	members, err := ctrl.communityRepo.GetMembers(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch members"})
		return
	}
	if members == nil {
		members = []models.CommunityMember{}
	}
	c.JSON(http.StatusOK, gin.H{"total_members": len(members), "members": members})
}

// RemoveCommunityMember godoc
//
//	@Summary		Remove community member
//	@Description	Removes a single user from a community by user ID.
//	@Tags			communities
//	@Produce		json
//	@Param			short_id	path	string	true	"Community short ID"
//	@Param			user_id		path	string	true	"User ID (UUID)"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Member not found in community"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/communities/{short_id}/members/{user_id} [delete]
func (ctrl *CommunityController) RemoveMember(c *gin.Context) {
	shortID := c.Param("short_id")
	userID := c.Param("user_id")

	existing, err := ctrl.communityRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch community"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "community not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, existing.BatchShortID) {
		return
	}

	if err := ctrl.communityRepo.RemoveMember(c.Request.Context(), shortID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "member not found in community"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not remove member"})
		return
	}

	c.Status(http.StatusNoContent)
}
