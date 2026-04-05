// Package image_repo предоставляет PostgreSQL-репозиторий для записей об изображениях.
package image_repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgxdriver "github.com/wb-go/wbf/dbpg/pgx-driver"

	"github.com/ponchik327/ImageProcessor/internal/domain"
)

const table = "images"

// ImageRepo хранит и извлекает записи об изображениях из PostgreSQL.
type ImageRepo struct {
	qe pgxdriver.QueryExecuter
	b  sq.StatementBuilderType
}

// New создаёт ImageRepo с заданным исполнителем запросов и построителем squirrel.
func New(qe pgxdriver.QueryExecuter, b sq.StatementBuilderType) *ImageRepo {
	return &ImageRepo{qe: qe, b: b}
}

// Create вставляет новую запись об изображении в базу данных.
func (r *ImageRepo) Create(ctx context.Context, img *domain.Image) error {
	sql, args, err := r.b.
		Insert(table).
		Columns("id", "original_path", "original_name", "mime_type", "status", "created_at", "updated_at").
		Values(img.ID, img.OriginalPath, img.OriginalName, img.MIMEType, img.Status, img.CreatedAt, img.UpdatedAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("image_repo: build create query: %w", err)
	}

	if _, err = r.qe.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("image_repo: create image: %w", err)
	}

	return nil
}

// GetByID возвращает изображение по его UUID. Возвращает domain.ErrNotFound, если строка не найдена.
func (r *ImageRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Image, error) {
	sql, args, err := r.b.
		Select("id", "original_path", "original_name", "mime_type", "status", "error_message", "created_at", "updated_at").
		From(table).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("image_repo: build get_by_id query: %w", err)
	}

	row := r.qe.QueryRow(ctx, sql, args...)

	img, err := scanImage(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}

		return nil, fmt.Errorf("image_repo: get_by_id: %w", err)
	}

	return img, nil
}

// List возвращает страницу изображений, упорядоченных по created_at по убыванию.
func (r *ImageRepo) List(ctx context.Context, limit, offset int) ([]*domain.Image, error) {
	lim := uint64(limit)  //nolint:gosec // G115: limit всегда неотрицателен (проверяется на уровне пагинации)
	off := uint64(offset) //nolint:gosec // G115: offset всегда неотрицателен (проверяется на уровне пагинации)

	sql, args, err := r.b.
		Select("id", "original_path", "original_name", "mime_type", "status", "error_message", "created_at", "updated_at").
		From(table).
		OrderBy("created_at DESC").
		Limit(lim).
		Offset(off).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("image_repo: build list query: %w", err)
	}

	rows, err := r.qe.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("image_repo: list: %w", err)
	}
	defer rows.Close()

	var images []*domain.Image

	for rows.Next() {
		img, err := scanImageFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("image_repo: list scan: %w", err)
		}

		images = append(images, img)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("image_repo: list rows: %w", err)
	}

	return images, nil
}

// UpdateStatus изменяет статус (и опционально error_message) изображения.
func (r *ImageRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.ImageStatus, errMsg *string) error {
	sql, args, err := r.b.
		Update(table).
		Set("status", status).
		Set("error_message", errMsg).
		Set("updated_at", time.Now()).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("image_repo: build update_status query: %w", err)
	}

	if _, err = r.qe.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("image_repo: update_status: %w", err)
	}

	return nil
}

// Delete удаляет запись об изображении по ID. Варианты удаляются каскадно через ON DELETE CASCADE.
func (r *ImageRepo) Delete(ctx context.Context, id uuid.UUID) error {
	sql, args, err := r.b.
		Delete(table).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("image_repo: build delete query: %w", err)
	}

	if _, err = r.qe.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("image_repo: delete: %w", err)
	}

	return nil
}

// ListByStatus возвращает все изображения с заданным статусом.
func (r *ImageRepo) ListByStatus(ctx context.Context, status domain.ImageStatus) ([]*domain.Image, error) {
	sql, args, err := r.b.
		Select("id", "original_path", "original_name", "mime_type", "status", "error_message", "created_at", "updated_at").
		From(table).
		Where(sq.Eq{"status": status}).
		OrderBy("created_at ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("image_repo: build list_by_status query: %w", err)
	}

	rows, err := r.qe.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("image_repo: list_by_status: %w", err)
	}
	defer rows.Close()

	var images []*domain.Image

	for rows.Next() {
		img, err := scanImageFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("image_repo: list_by_status scan: %w", err)
		}

		images = append(images, img)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("image_repo: list_by_status rows: %w", err)
	}

	return images, nil
}

// scanImage сканирует одну строку из QueryRow в Image.
func scanImage(row pgx.Row) (*domain.Image, error) {
	var img domain.Image

	err := row.Scan(
		&img.ID,
		&img.OriginalPath,
		&img.OriginalName,
		&img.MIMEType,
		&img.Status,
		&img.ErrorMessage,
		&img.CreatedAt,
		&img.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &img, nil
}

// scanImageFromRows сканирует текущую строку курсора Rows в Image.
func scanImageFromRows(rows pgx.Rows) (*domain.Image, error) {
	var img domain.Image

	err := rows.Scan(
		&img.ID,
		&img.OriginalPath,
		&img.OriginalName,
		&img.MIMEType,
		&img.Status,
		&img.ErrorMessage,
		&img.CreatedAt,
		&img.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &img, nil
}
