package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type CommunityPostRepository struct {
	pool *pgxpool.Pool
}

func NewCommunityPostRepository(pool *pgxpool.Pool) *CommunityPostRepository {
	return &CommunityPostRepository{pool: pool}
}

// postBaseSelect computes like_count/comment_count for every post, and
// liked_by_me relative to $1 (the viewer's user ID) — every caller of a
// query built on this passes the viewer ID as the first parameter.
const postBaseSelect = `
	SELECT p.id, p.short_id, p.community_id, c.short_id, p.author_id,
	       CONCAT(u.first_name, ' ', u.last_name), u.role::TEXT,
	       p.content, p.is_pinned,
	       (SELECT COUNT(*) FROM community_post_likes pl WHERE pl.post_id = p.id),
	       (SELECT COUNT(*) FROM community_comments cm WHERE cm.post_id = p.id AND cm.deleted_at IS NULL),
	       EXISTS (SELECT 1 FROM community_post_likes pl WHERE pl.post_id = p.id AND pl.user_id = $1),
	       p.created_at, p.updated_at
	FROM community_posts p
	JOIN communities c ON p.community_id = c.id
	JOIN users u ON p.author_id = u.id AND u.deleted_at IS NULL`

func scanPost(row pgx.Row) (models.CommunityPost, error) {
	var p models.CommunityPost
	err := row.Scan(
		&p.ID, &p.ShortID, &p.CommunityID, &p.CommunityShortID, &p.AuthorID,
		&p.AuthorName, &p.AuthorRole,
		&p.Content, &p.IsPinned,
		&p.LikeCount, &p.CommentCount, &p.LikedByMe,
		&p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}

// Create inserts a post into the community identified by communityShortID.
// Retries on short_id collision.
func (r *CommunityPostRepository) Create(ctx context.Context, communityShortID, authorID, content string) (*models.CommunityPost, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		var id string
		err := r.pool.QueryRow(ctx, `
			INSERT INTO community_posts (short_id, community_id, author_id, content)
			VALUES ($1, (SELECT id FROM communities WHERE short_id = $2 AND deleted_at IS NULL), $3, $4)
			RETURNING id`,
			shortID, communityShortID, authorID, content,
		).Scan(&id)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				continue
			}
			return nil, err
		}
		return r.FindByID(ctx, id, authorID)
	}
	return nil, errors.New("could not generate a unique short_id after 3 attempts")
}

func (r *CommunityPostRepository) FindByID(ctx context.Context, id, viewerID string) (*models.CommunityPost, error) {
	p, err := scanPost(r.pool.QueryRow(ctx, postBaseSelect+" WHERE p.id = $2 AND p.deleted_at IS NULL", viewerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *CommunityPostRepository) FindByShortID(ctx context.Context, shortID, viewerID string) (*models.CommunityPost, error) {
	p, err := scanPost(r.pool.QueryRow(ctx, postBaseSelect+" WHERE p.short_id = $2 AND p.deleted_at IS NULL", viewerID, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// FindAllForCommunity returns a community's feed, pinned posts first, then
// newest first.
func (r *CommunityPostRepository) FindAllForCommunity(ctx context.Context, communityShortID, viewerID string) ([]models.CommunityPost, error) {
	rows, err := r.pool.Query(ctx, postBaseSelect+`
		WHERE c.short_id = $2 AND p.deleted_at IS NULL
		ORDER BY p.is_pinned DESC, p.created_at DESC`,
		viewerID, communityShortID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.CommunityPost
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Delete soft-deletes a post.
func (r *CommunityPostRepository) Delete(ctx context.Context, shortID string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE community_posts SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// SetPinned pins/unpins a post — used for staff announcements.
func (r *CommunityPostRepository) SetPinned(ctx context.Context, shortID string, pinned bool) error {
	tag, err := r.pool.Exec(ctx, `UPDATE community_posts SET is_pinned = $1, updated_at = NOW() WHERE short_id = $2 AND deleted_at IS NULL`, pinned, shortID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ToggleLike likes the post if the user hasn't already, otherwise unlikes it.
// Returns the resulting liked state.
func (r *CommunityPostRepository) ToggleLike(ctx context.Context, postID, userID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM community_post_likes WHERE post_id = $1 AND user_id = $2`, postID, userID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() > 0 {
		return false, nil
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO community_post_likes (post_id, user_id) VALUES ($1, $2)`, postID, userID)
	if err != nil {
		return false, err
	}
	return true, nil
}

// ── Comments ─────────────────────────────────────────────────────────────

const commentBaseSelect = `
	SELECT cm.id, cm.short_id, cm.post_id, cm.author_id,
	       CONCAT(u.first_name, ' ', u.last_name), u.role::TEXT,
	       cm.content, cm.created_at
	FROM community_comments cm
	JOIN users u ON cm.author_id = u.id AND u.deleted_at IS NULL`

func scanComment(row pgx.Row) (models.CommunityComment, error) {
	var cm models.CommunityComment
	err := row.Scan(&cm.ID, &cm.ShortID, &cm.PostID, &cm.AuthorID, &cm.AuthorName, &cm.AuthorRole, &cm.Content, &cm.CreatedAt)
	return cm, err
}

// AddComment adds a comment to the post identified by postShortID. Retries
// on short_id collision.
func (r *CommunityPostRepository) AddComment(ctx context.Context, postShortID, authorID, content string) (*models.CommunityComment, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		var id string
		err := r.pool.QueryRow(ctx, `
			INSERT INTO community_comments (short_id, post_id, author_id, content)
			VALUES ($1, (SELECT id FROM community_posts WHERE short_id = $2 AND deleted_at IS NULL), $3, $4)
			RETURNING id`,
			shortID, postShortID, authorID, content,
		).Scan(&id)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				continue
			}
			return nil, err
		}
		cm, err := scanComment(r.pool.QueryRow(ctx, commentBaseSelect+" WHERE cm.id = $1", id))
		if err != nil {
			return nil, err
		}
		return &cm, nil
	}
	return nil, errors.New("could not generate a unique short_id after 3 attempts")
}

// FindComments returns every comment on a post, oldest first.
func (r *CommunityPostRepository) FindComments(ctx context.Context, postShortID string) ([]models.CommunityComment, error) {
	rows, err := r.pool.Query(ctx, commentBaseSelect+`
		JOIN community_posts p ON cm.post_id = p.id
		WHERE p.short_id = $1 AND cm.deleted_at IS NULL
		ORDER BY cm.created_at ASC`,
		postShortID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.CommunityComment
	for rows.Next() {
		cm, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cm)
	}
	return out, rows.Err()
}

// FindCommentAuthor returns the author_id of a comment, for permission checks.
func (r *CommunityPostRepository) FindCommentAuthor(ctx context.Context, commentShortID string) (string, error) {
	var authorID string
	err := r.pool.QueryRow(ctx, `SELECT author_id FROM community_comments WHERE short_id = $1 AND deleted_at IS NULL`, commentShortID).Scan(&authorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return authorID, err
}

// DeleteComment soft-deletes a comment.
func (r *CommunityPostRepository) DeleteComment(ctx context.Context, shortID string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE community_comments SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
