// Package variant_repo предоставляет PostgreSQL-репозиторий для записей о вариантах изображений.
package variant_repo

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgxdriver "github.com/wb-go/wbf/dbpg/pgx-driver"

	"github.com/ponchik327/ImageProcessor/internal/domain"
)

const table = "image_variants"

// VariantRepo хранит и извлекает записи о вариантах изображений из PostgreSQL.
type VariantRepo struct {
	qe pgxdriver.QueryExecuter
	b  sq.StatementBuilderType
}

// New создаёт VariantRepo с заданным исполнителем запросов и построителем squirrel.
func New(qe pgxdriver.QueryExecuter, b sq.StatementBuilderType) *VariantRepo {
	return &VariantRepo{qe: qe, b: b}
}

// Create вставляет новую запись о варианте изображения.
func (r *VariantRepo) Create(ctx context.Context, v *domain.ImageVariant) error {
	sql, args, err := r.b.
		Insert(table).
		Columns("id", "image_id", "variant_type", "file_path", "width", "height", "created_at").
		Values(v.ID, v.ImageID, v.VariantType, v.FilePath, v.Width, v.Height, v.CreatedAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("variant_repo: build create query: %w", err)
	}

	if _, err = r.qe.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("variant_repo: create variant: %w", err)
	}

	return nil
}

// ListByImageID возвращает все варианты заданного изображения, упорядоченные по created_at.
func (r *VariantRepo) ListByImageID(ctx context.Context, imageID uuid.UUID) ([]*domain.ImageVariant, error) {
	sql, args, err := r.b.
		Select("id", "image_id", "variant_type", "file_path", "width", "height", "created_at").
		From(table).
		Where(sq.Eq{"image_id": imageID}).
		OrderBy("created_at ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("variant_repo: build list query: %w", err)
	}

	rows, err := r.qe.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("variant_repo: list_by_image_id: %w", err)
	}
	defer rows.Close()

	var variants []*domain.ImageVariant

	for rows.Next() {
		v, err := scanVariantFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("variant_repo: list scan: %w", err)
		}

		variants = append(variants, v)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("variant_repo: list rows: %w", err)
	}

	return variants, nil
}

// ListByImageIDs возвращает все варианты для заданного набора изображений, упорядоченные по created_at.
func (r *VariantRepo) ListByImageIDs(ctx context.Context, imageIDs []uuid.UUID) ([]*domain.ImageVariant, error) {
	if len(imageIDs) == 0 {
		return nil, nil
	}

	sql, args, err := r.b.
		Select("id", "image_id", "variant_type", "file_path", "width", "height", "created_at").
		From(table).
		Where(sq.Eq{"image_id": imageIDs}).
		OrderBy("created_at ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("variant_repo: build list_by_image_ids query: %w", err)
	}

	rows, err := r.qe.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("variant_repo: list_by_image_ids: %w", err)
	}
	defer rows.Close()

	var variants []*domain.ImageVariant

	for rows.Next() {
		v, err := scanVariantFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("variant_repo: list_by_image_ids scan: %w", err)
		}

		variants = append(variants, v)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("variant_repo: list_by_image_ids rows: %w", err)
	}

	return variants, nil
}

// DeleteByImageID удаляет все варианты заданного изображения.
// На практике это обрабатывается через ON DELETE CASCADE, но метод доступен для явной очистки.
func (r *VariantRepo) DeleteByImageID(ctx context.Context, imageID uuid.UUID) error {
	sql, args, err := r.b.
		Delete(table).
		Where(sq.Eq{"image_id": imageID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("variant_repo: build delete query: %w", err)
	}

	if _, err = r.qe.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("variant_repo: delete_by_image_id: %w", err)
	}

	return nil
}

func scanVariantFromRows(rows pgx.Rows) (*domain.ImageVariant, error) {
	var v domain.ImageVariant

	err := rows.Scan(&v.ID, &v.ImageID, &v.VariantType, &v.FilePath, &v.Width, &v.Height, &v.CreatedAt)
	if err != nil {
		return nil, err
	}

	return &v, nil
}
