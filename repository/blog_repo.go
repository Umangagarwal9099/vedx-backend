package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type BlogRepository struct {
	pool *pgxpool.Pool
}

func NewBlogRepository(pool *pgxpool.Pool) *BlogRepository {
	return &BlogRepository{pool: pool}
}

const blogBaseSelect = `
	SELECT b.id, b.short_id, b.title, b.content,
	       COALESCE(b.excerpt, ''), b.author,
	       b.status::TEXT, b.publish_at,
	       b.is_featured, b.show_in_recent_updates,
	       COALESCE(b.featured_image, ''),
	       b.is_active, b.created_by::TEXT,
	       b.created_at, b.updated_at
	FROM blogs b`

func scanBlog(row pgx.Row) (*models.Blog, error) {
	var b models.Blog
	err := row.Scan(
		&b.ID, &b.ShortID, &b.Title, &b.Content,
		&b.Excerpt, &b.Author,
		&b.Status, &b.PublishAt,
		&b.IsFeatured, &b.ShowInRecentUpdates,
		&b.FeaturedImage,
		&b.IsActive, &b.CreatedBy,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// Create inserts a new blog post, retrying up to 3 times on short_id collision.
func (r *BlogRepository) Create(ctx context.Context, in models.CreateBlogInput, createdBy string) (*models.Blog, error) {
	// Auto-set publish_at to now for immediately published blogs.
	if in.Status == "published" && in.PublishAt == nil {
		now := time.Now()
		in.PublishAt = &now
	}

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		row := r.pool.QueryRow(ctx, `
			WITH ins AS (
				INSERT INTO blogs
				  (short_id, title, content, excerpt, author, status, publish_at,
				   is_featured, show_in_recent_updates, featured_image, created_by)
				VALUES ($1,$2,$3,NULLIF($4,''),$5,$6::blog_status,$7,
				        $8,$9,NULLIF($10,''),$11::UUID)
				RETURNING *
			)
			SELECT ins.id, ins.short_id, ins.title, ins.content,
			       COALESCE(ins.excerpt, ''), ins.author,
			       ins.status::TEXT, ins.publish_at,
			       ins.is_featured, ins.show_in_recent_updates,
			       COALESCE(ins.featured_image, ''),
			       ins.is_active, ins.created_by::TEXT,
			       ins.created_at, ins.updated_at
			FROM ins`,
			shortID, in.Title, in.Content, in.Excerpt, in.Author,
			in.Status, in.PublishAt,
			in.IsFeatured, in.ShowInRecentUpdates, in.FeaturedImage,
			createdBy,
		)

		b, err := scanBlog(row)
		if err == nil {
			return b, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("insert blog: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAll returns all non-deleted blogs ordered newest first.
func (r *BlogRepository) FindAll(ctx context.Context) ([]models.Blog, error) {
	q := blogBaseSelect + ` WHERE b.deleted_at IS NULL ORDER BY b.created_at DESC`
	return r.scanBlogs(ctx, q)
}

// FindByShortID returns a single non-deleted blog.
func (r *BlogRepository) FindByShortID(ctx context.Context, shortID string) (*models.Blog, error) {
	q := blogBaseSelect + ` WHERE b.short_id = $1 AND b.deleted_at IS NULL LIMIT 1`
	b, err := scanBlog(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return b, err
}

// Search filters non-deleted blogs by title (ILIKE), exact status, and/or creation date.
func (r *BlogRepository) Search(ctx context.Context, f models.BlogFilter) ([]models.Blog, error) {
	args := []interface{}{}
	where := []string{"b.deleted_at IS NULL"}
	i := 1

	if f.Title != "" {
		where = append(where, fmt.Sprintf("b.title ILIKE $%d", i))
		args = append(args, "%"+f.Title+"%")
		i++
	}
	if f.Status != "" {
		where = append(where, fmt.Sprintf("b.status = $%d::blog_status", i))
		args = append(args, f.Status)
		i++
	}
	if f.Date != "" {
		where = append(where, fmt.Sprintf("DATE(b.created_at) = $%d::DATE", i))
		args = append(args, f.Date)
		i++
	}

	q := blogBaseSelect + ` WHERE ` + strings.Join(where, " AND ") + ` ORDER BY b.created_at DESC`
	return r.scanBlogs(ctx, q, args...)
}

// Update applies a partial update — only non-nil fields are changed.
func (r *BlogRepository) Update(ctx context.Context, shortID string, in models.UpdateBlogInput) error {
	args := []interface{}{shortID}
	setClauses := []string{}
	i := 2

	add := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf(clause, i))
		args = append(args, val)
		i++
	}

	if in.Title != nil {
		add("title = $%d", *in.Title)
	}
	if in.Content != nil {
		add("content = $%d", *in.Content)
	}
	if in.Excerpt != nil {
		add("excerpt = NULLIF($%d,'')", *in.Excerpt)
	}
	if in.Author != nil {
		add("author = $%d", *in.Author)
	}
	if in.Status != nil {
		add("status = $%d::blog_status", *in.Status)
	}
	if in.PublishAt != nil {
		add("publish_at = $%d", *in.PublishAt)
	}
	if in.IsFeatured != nil {
		add("is_featured = $%d", *in.IsFeatured)
	}
	if in.ShowInRecentUpdates != nil {
		add("show_in_recent_updates = $%d", *in.ShowInRecentUpdates)
	}
	if in.FeaturedImage != nil {
		add("featured_image = NULLIF($%d,'')", *in.FeaturedImage)
	}
	if in.IsActive != nil {
		add("is_active = $%d", *in.IsActive)
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf(
		"UPDATE blogs SET %s WHERE short_id = $1 AND deleted_at IS NULL",
		strings.Join(setClauses, ", "),
	)
	result, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// Delete soft-deletes a blog by its short_id.
func (r *BlogRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE blogs SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *BlogRepository) scanBlogs(ctx context.Context, q string, args ...interface{}) ([]models.Blog, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blogs []models.Blog
	for rows.Next() {
		var b models.Blog
		if err := rows.Scan(
			&b.ID, &b.ShortID, &b.Title, &b.Content,
			&b.Excerpt, &b.Author,
			&b.Status, &b.PublishAt,
			&b.IsFeatured, &b.ShowInRecentUpdates,
			&b.FeaturedImage,
			&b.IsActive, &b.CreatedBy,
			&b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, err
		}
		blogs = append(blogs, b)
	}
	return blogs, rows.Err()
}
