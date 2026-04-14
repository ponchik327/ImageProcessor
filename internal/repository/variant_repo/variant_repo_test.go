//go:build integration

package variant_repo

import (
	"context"
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

func cleanTables(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := testPG.Exec(ctx, "DELETE FROM image_variants")
	require.NoError(t, err)
	_, err = testPG.Exec(ctx, "DELETE FROM images")
	require.NoError(t, err)
}

func newRepo() *VariantRepo {
	return New(testPG, testPG.Builder)
}

// insertImage вставляет родительское изображение напрямую через SQL.
func insertImage(t *testing.T, ctx context.Context) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPG.Exec(ctx,
		`INSERT INTO images (id, original_path, original_name, mime_type, status, created_at, updated_at)
         VALUES ($1, $2, 'photo.jpg', 'image/jpeg', 'pending', NOW(), NOW())`,
		id, "/storage/"+id.String()+"/original.jpg",
	)
	require.NoError(t, err)
	return id
}

func newVariant(imageID uuid.UUID, variantType string) *domain.ImageVariant {
	return &domain.ImageVariant{
		ID:          uuid.New(),
		ImageID:     imageID,
		VariantType: variantType,
		FilePath:    "/storage/" + imageID.String() + "/" + variantType + ".jpg",
		Width:       800,
		Height:      600,
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
}

// ─── Create / ListByImageID ───────────────────────────────────────────────────

func TestCreate_And_ListByImageID(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	repo := newRepo()

	imageID := insertImage(t, ctx)
	v := newVariant(imageID, "resize")

	require.NoError(t, repo.Create(ctx, v))

	variants, err := repo.ListByImageID(ctx, imageID)
	require.NoError(t, err)
	require.Len(t, variants, 1)

	got := variants[0]
	assert.Equal(t, v.ID, got.ID)
	assert.Equal(t, v.ImageID, got.ImageID)
	assert.Equal(t, v.VariantType, got.VariantType)
	assert.Equal(t, v.FilePath, got.FilePath)
	assert.Equal(t, v.Width, got.Width)
	assert.Equal(t, v.Height, got.Height)
	assert.WithinDuration(t, v.CreatedAt, got.CreatedAt, time.Second)
}

func TestListByImageID_OrderASC(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	repo := newRepo()

	imageID := insertImage(t, ctx)

	types := []string{"resize", "thumbnail", "watermark"}
	for i, vt := range types {
		v := newVariant(imageID, vt)
		v.CreatedAt = time.Now().UTC().Add(time.Duration(i) * time.Second).Truncate(time.Millisecond)
		require.NoError(t, repo.Create(ctx, v))
	}

	variants, err := repo.ListByImageID(ctx, imageID)
	require.NoError(t, err)
	require.Len(t, variants, 3)

	// Порядок ASC по created_at
	for i := 1; i < len(variants); i++ {
		assert.True(t,
			!variants[i].CreatedAt.Before(variants[i-1].CreatedAt),
			"ожидается порядок ASC по created_at",
		)
	}
}

func TestListByImageID_Empty(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	repo := newRepo()

	variants, err := repo.ListByImageID(ctx, uuid.New())
	require.NoError(t, err)
	assert.Empty(t, variants)
}

// ─── ListByImageIDs ───────────────────────────────────────────────────────────

func TestListByImageIDs_MultipleImages(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	repo := newRepo()

	id1 := insertImage(t, ctx)
	id2 := insertImage(t, ctx)

	require.NoError(t, repo.Create(ctx, newVariant(id1, "resize")))
	require.NoError(t, repo.Create(ctx, newVariant(id1, "thumbnail")))
	require.NoError(t, repo.Create(ctx, newVariant(id2, "resize")))

	variants, err := repo.ListByImageIDs(ctx, []uuid.UUID{id1, id2})
	require.NoError(t, err)
	assert.Len(t, variants, 3)
}

func TestListByImageIDs_Empty(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	repo := newRepo()

	variants, err := repo.ListByImageIDs(ctx, []uuid.UUID{})
	require.NoError(t, err)
	assert.Nil(t, variants)
}

// ─── DeleteByImageID ─────────────────────────────────────────────────────────

func TestDeleteByImageID(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	repo := newRepo()

	imageID := insertImage(t, ctx)
	require.NoError(t, repo.Create(ctx, newVariant(imageID, "resize")))
	require.NoError(t, repo.Create(ctx, newVariant(imageID, "thumbnail")))

	require.NoError(t, repo.DeleteByImageID(ctx, imageID))

	variants, err := repo.ListByImageID(ctx, imageID)
	require.NoError(t, err)
	assert.Empty(t, variants)
}