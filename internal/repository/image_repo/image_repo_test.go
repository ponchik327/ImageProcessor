//go:build integration

package image_repo

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wb-go/wbf/logger"
	pgxdriver "github.com/wb-go/wbf/dbpg/pgx-driver"

	"github.com/ponchik327/ImageProcessor/internal/domain"
	"github.com/ponchik327/ImageProcessor/internal/migration"
	"github.com/ponchik327/ImageProcessor/internal/repository/testhelper"
)

var testPG *pgxdriver.Postgres

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, connStr, err := testhelper.StartPostgres(ctx)
	if err != nil {
		panic("start postgres container: " + err.Error())
	}
	defer func() { _ = pgContainer.Terminate(ctx) }()

	log, err := logger.InitLogger(logger.ZapEngine, "test", "test", logger.WithLevel(logger.ErrorLevel))
	if err != nil {
		panic("init logger: " + err.Error())
	}

	if err = migration.Run(connStr, log); err != nil {
		panic("run migrations: " + err.Error())
	}

	testPG, err = pgxdriver.New(connStr, log)
	if err != nil {
		panic("connect to postgres: " + err.Error())
	}
	defer testPG.Close()

	os.Exit(m.Run())
}

// cleanTables удаляет все строки перед каждым тестом.
func cleanTables(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := testPG.Exec(ctx, "DELETE FROM image_variants")
	require.NoError(t, err)
	_, err = testPG.Exec(ctx, "DELETE FROM images")
	require.NoError(t, err)
}

func newRepo() *ImageRepo {
	return New(testPG, testPG.Builder)
}

func newImage(status domain.ImageStatus) *domain.Image {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &domain.Image{
		ID:           uuid.New(),
		OriginalPath: "/storage/" + uuid.NewString() + "/original.jpg",
		OriginalName: "photo.jpg",
		MIMEType:     "image/jpeg",
		Status:       status,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// ─── Create / GetByID ─────────────────────────────────────────────────────────

func TestCreate_And_GetByID(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	repo := newRepo()

	img := newImage(domain.StatusPending)
	require.NoError(t, repo.Create(ctx, img))

	got, err := repo.GetByID(ctx, img.ID)
	require.NoError(t, err)

	assert.Equal(t, img.ID, got.ID)
	assert.Equal(t, img.OriginalPath, got.OriginalPath)
	assert.Equal(t, img.OriginalName, got.OriginalName)
	assert.Equal(t, img.MIMEType, got.MIMEType)
	assert.Equal(t, img.Status, got.Status)
	assert.Nil(t, got.ErrorMessage)
	assert.WithinDuration(t, img.CreatedAt, got.CreatedAt, time.Second)
	assert.WithinDuration(t, img.UpdatedAt, got.UpdatedAt, time.Second)
}

func TestGetByID_NotFound(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	repo := newRepo()

	_, err := repo.GetByID(ctx, uuid.New())

	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

// ─── List ─────────────────────────────────────────────────────────────────────

func TestList_OrderAndPagination(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	repo := newRepo()

	// Вставляем 3 изображения с разным временем создания
	for i := range 3 {
		img := newImage(domain.StatusPending)
		img.CreatedAt = time.Now().UTC().Add(time.Duration(i) * time.Second).Truncate(time.Millisecond)
		img.UpdatedAt = img.CreatedAt
		require.NoError(t, repo.Create(ctx, img))
	}

	// limit=2, offset=0 → 2 новейших (DESC)
	page1, err := repo.List(ctx, 2, 0)
	require.NoError(t, err)
	assert.Len(t, page1, 2)
	assert.True(t, page1[0].CreatedAt.After(page1[1].CreatedAt) || page1[0].CreatedAt.Equal(page1[1].CreatedAt))

	// limit=2, offset=2 → оставшийся 1
	page2, err := repo.List(ctx, 2, 2)
	require.NoError(t, err)
	assert.Len(t, page2, 1)
}

// ─── UpdateStatus ─────────────────────────────────────────────────────────────

func TestUpdateStatus_WithoutErrMsg(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	repo := newRepo()

	img := newImage(domain.StatusPending)
	require.NoError(t, repo.Create(ctx, img))
	require.NoError(t, repo.UpdateStatus(ctx, img.ID, domain.StatusProcessing, nil))

	got, err := repo.GetByID(ctx, img.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusProcessing, got.Status)
	assert.Nil(t, got.ErrorMessage)
}

func TestUpdateStatus_WithErrMsg(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	repo := newRepo()

	img := newImage(domain.StatusPending)
	require.NoError(t, repo.Create(ctx, img))

	errMsg := "processing failed: out of memory"
	require.NoError(t, repo.UpdateStatus(ctx, img.ID, domain.StatusFailed, &errMsg))

	got, err := repo.GetByID(ctx, img.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusFailed, got.Status)
	require.NotNil(t, got.ErrorMessage)
	assert.Equal(t, errMsg, *got.ErrorMessage)
}

// ─── Delete ───────────────────────────────────────────────────────────────────

func TestDelete(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	repo := newRepo()

	img := newImage(domain.StatusDone)
	require.NoError(t, repo.Create(ctx, img))

	// Добавляем вариант напрямую, чтобы проверить CASCADE
	_, err := testPG.Exec(ctx,
		`INSERT INTO image_variants (id, image_id, variant_type, file_path, width, height, created_at)
         VALUES ($1, $2, 'resize', '/path/resize.jpg', 800, 600, NOW())`,
		uuid.New(), img.ID,
	)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, img.ID))

	_, getErr := repo.GetByID(ctx, img.ID)
	assert.True(t, errors.Is(getErr, domain.ErrNotFound))

	// Проверяем CASCADE: вариант тоже удалён
	var count int
	row := testPG.QueryRow(ctx, "SELECT COUNT(*) FROM image_variants WHERE image_id = $1", img.ID)
	require.NoError(t, row.Scan(&count))
	assert.Equal(t, 0, count)
}

// ─── ListByStatus ─────────────────────────────────────────────────────────────

func TestListByStatus(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	repo := newRepo()

	pending1 := newImage(domain.StatusPending)
	pending1.CreatedAt = time.Now().UTC().Add(-2 * time.Second).Truncate(time.Millisecond)
	pending1.UpdatedAt = pending1.CreatedAt

	pending2 := newImage(domain.StatusPending)
	pending2.CreatedAt = time.Now().UTC().Add(-1 * time.Second).Truncate(time.Millisecond)
	pending2.UpdatedAt = pending2.CreatedAt

	done := newImage(domain.StatusDone)

	require.NoError(t, repo.Create(ctx, pending1))
	require.NoError(t, repo.Create(ctx, pending2))
	require.NoError(t, repo.Create(ctx, done))

	result, err := repo.ListByStatus(ctx, domain.StatusPending)
	require.NoError(t, err)
	require.Len(t, result, 2)

	// Порядок ASC по created_at
	assert.True(t, result[0].CreatedAt.Before(result[1].CreatedAt) || result[0].CreatedAt.Equal(result[1].CreatedAt))

	// done не попадает
	for _, img := range result {
		assert.Equal(t, domain.StatusPending, img.Status)
	}
}
