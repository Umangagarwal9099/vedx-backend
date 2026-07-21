package models

import "time"

// CommunityPost is one post in a community's feed. LikeCount/CommentCount are
// computed at read time; LikedByMe reflects only the requesting user.
type CommunityPost struct {
	ID               string     `json:"id"`
	ShortID          string     `json:"short_id"`
	CommunityID      string     `json:"community_id"`
	CommunityShortID string     `json:"community_short_id"`
	AuthorID         string     `json:"author_id"`
	AuthorName       string     `json:"author_name"`
	AuthorRole       string     `json:"author_role"`
	Content          string     `json:"content"`
	IsPinned         bool       `json:"is_pinned"`
	LikeCount        int        `json:"like_count"`
	CommentCount     int        `json:"comment_count"`
	LikedByMe        bool       `json:"liked_by_me"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
}

// CreateCommunityPostInput carries the fields required to create a post.
type CreateCommunityPostInput struct {
	Content string `json:"content" binding:"required" example:"Excited for tomorrow's session!"`
}

// CommunityComment is one comment on a post.
type CommunityComment struct {
	ID         string    `json:"id"`
	ShortID    string    `json:"short_id"`
	PostID     string    `json:"post_id"`
	AuthorID   string    `json:"author_id"`
	AuthorName string    `json:"author_name"`
	AuthorRole string    `json:"author_role"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
}

// CreateCommunityCommentInput carries the fields required to add a comment.
type CreateCommunityCommentInput struct {
	Content string `json:"content" binding:"required" example:"Same here!"`
}
