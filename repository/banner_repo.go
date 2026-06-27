package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type BannerRepository struct {
	pool *pgxpool.Pool
}

func NewBannerRepository(pool *pgxpool.Pool) *BannerRepository {
	return &BannerRepository{pool: pool}
}

const bannerBaseSelect = `
	SELECT b.id, b.short_id, b.name, b.branches,
	       COALESCE(b.thumbnail, ''), COALESCE(b.cta_url, ''),
	       COALESCE(b.category, ''),
	       b.is_active, b.created_by::TEXT,
	       b.created_at, b.updated_at
	FROM banners b`

func scanBanner(row pgx.Row) (*models.Banner, error) {
	var b models.Banner
	err := row.Scan(
		&b.ID, &b.ShortID, &b.Name, &b.Branches,
		&b.Thumbnail, &b.CTAURL,
		&b.Category,
		&b.IsActive, &b.CreatedBy,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if b.Branches == nil {
		b.Branches = []string{}
	}
	return &b, nil
}

// Create inserts a new banner, retrying up to 3 times on short_id collision.
func (r *BannerRepository) Create(ctx context.Context, in models.CreateBannerInput, createdBy string) (*models.Banner, error) {
	branches := in.Branches
	if branches == nil {
		branches = []string{}
	}

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		row := r.pool.QueryRow(ctx, `
			WITH ins AS (
				INSERT INTO banners
				  (short_id, name, branches, thumbnail, cta_url, category, is_active, created_by)
				VALUES ($1, $2, $3, NULLIF($4,''), NULLIF($5,''), NULLIF($6,''), $7, $8::UUID)
				RETURNING *
			)
			SELECT ins.id, ins.short_id, ins.name, ins.branches,
			       COALESCE(ins.thumbnail, ''), COALESCE(ins.cta_url, ''),
			       COALESCE(ins.category, ''),
			       ins.is_active, ins.created_by::TEXT,
			       ins.created_at, ins.updated_at
			FROM ins`,
			shortID, in.Name, branches,
			in.Thumbnail, in.CTAURL, in.Category,
			in.IsActive, createdBy,
		)

		b, err := scanBanner(row)
		if err == nil {
			return b, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("insert banner: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAll returns all non-deleted banners ordered newest first.
func (r *BannerRepository) FindAll(ctx context.Context) ([]models.Banner, error) {
	q := bannerBaseSelect + ` WHERE b.deleted_at IS NULL ORDER BY b.created_at DESC`
	return r.scanBanners(ctx, q)
}

// FindByShortID returns a single non-deleted banner.
func (r *BannerRepository) FindByShortID(ctx context.Context, shortID string) (*models.Banner, error) {
	q := bannerBaseSelect + ` WHERE b.short_id = $1 AND b.deleted_at IS NULL LIMIT 1`
	b, err := scanBanner(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return b, err
}

// Search filters non-deleted banners by name (ILIKE) and/or exact category.
func (r *BannerRepository) Search(ctx context.Context, f models.BannerFilter) ([]models.Banner, error) {
	args := []interface{}{}
	where := []string{"b.deleted_at IS NULL"}
	i := 1

	if f.Name != "" {
		where = append(where, fmt.Sprintf("b.name ILIKE $%d", i))
		args = append(args, "%"+f.Name+"%")
		i++
	}
	if f.Category != "" {
		where = append(where, fmt.Sprintf("b.category = $%d", i))
		args = append(args, f.Category)
		i++
	}

	q := bannerBaseSelect + ` WHERE ` + strings.Join(where, " AND ") + ` ORDER BY b.created_at DESC`
	return r.scanBanners(ctx, q, args...)
}

// Update applies a partial update — only non-nil/non-empty fields are changed.
func (r *BannerRepository) Update(ctx context.Context, shortID string, in models.UpdateBannerInput) error {
	args := []interface{}{shortID}
	setClauses := []string{}
	i := 2

	add := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf(clause, i))
		args = append(args, val)
		i++
	}

	if in.Name != nil {
		add("name = $%d", *in.Name)
	}
	if in.Branches != nil {
		add("branches = $%d", in.Branches)
	}
	if in.Thumbnail != nil {
		add("thumbnail = NULLIF($%d,'')", *in.Thumbnail)
	}
	if in.CTAURL != nil {
		add("cta_url = NULLIF($%d,'')", *in.CTAURL)
	}
	if in.Category != nil {
		add("category = NULLIF($%d,'')", *in.Category)
	}
	if in.IsActive != nil {
		add("is_active = $%d", *in.IsActive)
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf(
		"UPDATE banners SET %s WHERE short_id = $1 AND deleted_at IS NULL",
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

// Delete soft-deletes a banner by its short_id.
func (r *BannerRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE banners SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *BannerRepository) scanBanners(ctx context.Context, q string, args ...interface{}) ([]models.Banner, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var banners []models.Banner
	for rows.Next() {
		var b models.Banner
		if err := rows.Scan(
			&b.ID, &b.ShortID, &b.Name, &b.Branches,
			&b.Thumbnail, &b.CTAURL,
			&b.Category,
			&b.IsActive, &b.CreatedBy,
			&b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if b.Branches == nil {
			b.Branches = []string{}
		}
		banners = append(banners, b)
	}
	return banners, rows.Err()
}
