package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type CommunityPostController struct {
	postRepo      *repository.CommunityPostRepository
	communityRepo *repository.CommunityRepository
}

func NewCommunityPostController(postRepo *repository.CommunityPostRepository, communityRepo *repository.CommunityRepository) *CommunityPostController {
	return &CommunityPostController{postRepo: postRepo, communityRepo: communityRepo}
}

// isStaff reports whether the caller can moderate any community without
// being a member — students must be a member of the specific community.
func isStaff(role string) bool {
	return role == string(models.RoleSuperAdmin) || role == string(models.RoleTeamLead) || role == string(models.RoleMentor)
}

// checkCommunityMembership enforces that a non-staff caller is a member of
// the community before posting/commenting/liking in it. Writes the error
// response itself on failure; the caller should return immediately on false.
func (ctrl *CommunityPostController) checkCommunityMembership(c *gin.Context, communityShortID string) bool {
	if isStaff(c.GetString("role")) {
		return true
	}
	member, err := ctrl.communityRepo.IsMember(c.Request.Context(), communityShortID, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify community membership"})
		return false
	}
	if !member {
		c.JSON(http.StatusForbidden, gin.H{"error": "you're not a member of this community"})
		return false
	}
	return true
}

// CreatePost godoc
//
//	@Summary		Create a community post
//	@Description	Creates a post in a community. The caller must be a member (staff bypass this check).
//	@Tags			community-posts
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"Community short ID"
//	@Param			body		body		models.CreateCommunityPostInput	true	"Post content"
//	@Success		201			{object}	models.CommunityPost
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		403			{object}	map[string]string	"Not a member of this community"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/communities/{short_id}/posts [post]
func (ctrl *CommunityPostController) CreatePost(c *gin.Context) {
	communityShortID := c.Param("short_id")
	if !ctrl.checkCommunityMembership(c, communityShortID) {
		return
	}

	var input models.CreateCommunityPostInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	post, err := ctrl.postRepo.Create(c.Request.Context(), communityShortID, c.GetString("user_id"), input.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create post: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, post)
}

// GetPosts godoc
//
//	@Summary		List a community's posts
//	@Description	Returns a community's feed, pinned posts first, then newest first.
//	@Tags			community-posts
//	@Produce		json
//	@Param			short_id	path	string	true	"Community short ID"
//	@Success		200	{array}		models.CommunityPost
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/communities/{short_id}/posts [get]
func (ctrl *CommunityPostController) GetPosts(c *gin.Context) {
	communityShortID := c.Param("short_id")

	posts, err := ctrl.postRepo.FindAllForCommunity(c.Request.Context(), communityShortID, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch posts"})
		return
	}
	if posts == nil {
		posts = []models.CommunityPost{}
	}
	c.JSON(http.StatusOK, posts)
}

// checkPostOwnerOrStaff enforces that only the post's author or staff can
// delete/pin it. Writes the error response itself on failure.
func (ctrl *CommunityPostController) checkPostOwnerOrStaff(c *gin.Context, postShortID string) bool {
	post, err := ctrl.postRepo.FindByShortID(c.Request.Context(), postShortID, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch post"})
		return false
	}
	if post == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
		return false
	}
	if post.AuthorID != c.GetString("user_id") && !isStaff(c.GetString("role")) {
		c.JSON(http.StatusForbidden, gin.H{"error": "you can only delete your own posts"})
		return false
	}
	return true
}

// DeletePost godoc
//
//	@Summary		Delete a community post
//	@Description	Deletes a post — the post's author, or staff (moderation).
//	@Tags			community-posts
//	@Produce		json
//	@Param			short_id	path	string	true	"Post short ID"
//	@Success		204	"No Content"
//	@Failure		403	{object}	map[string]string	"Not your post"
//	@Failure		404	{object}	map[string]string	"Post not found"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/posts/{short_id} [delete]
func (ctrl *CommunityPostController) DeletePost(c *gin.Context) {
	shortID := c.Param("short_id")
	if !ctrl.checkPostOwnerOrStaff(c, shortID) {
		return
	}

	if err := ctrl.postRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete post"})
		return
	}
	c.Status(http.StatusNoContent)
}

// SetPostPinned godoc
//
//	@Summary		Pin or unpin a community post
//	@Description	Pins/unpins a post as an announcement — staff only.
//	@Tags			community-posts
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string					true	"Post short ID"
//	@Param			body		body		map[string]bool	true	"{\"pinned\": true}"
//	@Success		200			{object}	map[string]string	"pinned updated"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/posts/{short_id}/pin [patch]
func (ctrl *CommunityPostController) SetPostPinned(c *gin.Context) {
	shortID := c.Param("short_id")

	var body struct {
		Pinned bool `json:"pinned"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.postRepo.SetPinned(c.Request.Context(), shortID, body.Pinned); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update post"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "pinned updated"})
}

// ToggleLike godoc
//
//	@Summary		Like/unlike a community post
//	@Description	Toggles the caller's like on a post. Returns the resulting liked state.
//	@Tags			community-posts
//	@Produce		json
//	@Param			short_id	path		string	true	"Post short ID"
//	@Success		200			{object}	map[string]bool	"liked"
//	@Failure		403			{object}	map[string]string	"Not a member of this community"
//	@Failure		404			{object}	map[string]string	"Post not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/posts/{short_id}/like [post]
func (ctrl *CommunityPostController) ToggleLike(c *gin.Context) {
	shortID := c.Param("short_id")

	post, err := ctrl.postRepo.FindByShortID(c.Request.Context(), shortID, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch post"})
		return
	}
	if post == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
		return
	}
	if !ctrl.checkCommunityMembership(c, post.CommunityShortID) {
		return
	}

	liked, err := ctrl.postRepo.ToggleLike(c.Request.Context(), post.ID, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not toggle like"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"liked": liked})
}

// ── Comments ─────────────────────────────────────────────────────────────

// AddComment godoc
//
//	@Summary		Comment on a community post
//	@Description	Adds a comment to a post. The caller must be a member of the post's community (staff bypass this check).
//	@Tags			community-posts
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string								true	"Post short ID"
//	@Param			body		body		models.CreateCommunityCommentInput	true	"Comment content"
//	@Success		201			{object}	models.CommunityComment
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		403			{object}	map[string]string	"Not a member of this community"
//	@Failure		404			{object}	map[string]string	"Post not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/posts/{short_id}/comments [post]
func (ctrl *CommunityPostController) AddComment(c *gin.Context) {
	postShortID := c.Param("short_id")

	post, err := ctrl.postRepo.FindByShortID(c.Request.Context(), postShortID, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch post"})
		return
	}
	if post == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
		return
	}
	if !ctrl.checkCommunityMembership(c, post.CommunityShortID) {
		return
	}

	var input models.CreateCommunityCommentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	comment, err := ctrl.postRepo.AddComment(c.Request.Context(), postShortID, c.GetString("user_id"), input.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not add comment: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, comment)
}

// GetComments godoc
//
//	@Summary		List a post's comments
//	@Description	Returns every comment on a post, oldest first.
//	@Tags			community-posts
//	@Produce		json
//	@Param			short_id	path	string	true	"Post short ID"
//	@Success		200	{array}		models.CommunityComment
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/posts/{short_id}/comments [get]
func (ctrl *CommunityPostController) GetComments(c *gin.Context) {
	postShortID := c.Param("short_id")

	comments, err := ctrl.postRepo.FindComments(c.Request.Context(), postShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch comments"})
		return
	}
	if comments == nil {
		comments = []models.CommunityComment{}
	}
	c.JSON(http.StatusOK, comments)
}

// DeleteComment godoc
//
//	@Summary		Delete a comment
//	@Description	Deletes a comment — the comment's author, or staff (moderation).
//	@Tags			community-posts
//	@Produce		json
//	@Param			short_id	path	string	true	"Comment short ID"
//	@Success		204	"No Content"
//	@Failure		403	{object}	map[string]string	"Not your comment"
//	@Failure		404	{object}	map[string]string	"Comment not found"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/comments/{short_id} [delete]
func (ctrl *CommunityPostController) DeleteComment(c *gin.Context) {
	shortID := c.Param("short_id")

	authorID, err := ctrl.postRepo.FindCommentAuthor(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch comment"})
		return
	}
	if authorID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		return
	}
	if authorID != c.GetString("user_id") && !isStaff(c.GetString("role")) {
		c.JSON(http.StatusForbidden, gin.H{"error": "you can only delete your own comments"})
		return
	}

	if err := ctrl.postRepo.DeleteComment(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete comment"})
		return
	}
	c.Status(http.StatusNoContent)
}
