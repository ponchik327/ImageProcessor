package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/wb-go/wbf/logger"

	"github.com/ponchik327/ImageProcessor/internal/config"
	"github.com/ponchik327/ImageProcessor/internal/domain"
	"github.com/ponchik327/ImageProcessor/internal/processor"
	"github.com/ponchik327/ImageProcessor/internal/storage"
)

// ─── Моки ─────────────────────────────────────────────────────────────────────

type MockImageRepository struct{ mock.Mock }

func (m *MockImageRepository) Create(ctx context.Context, img *domain.Image) error {
	return m.Called(ctx, img).Error(0)
}
func (m *MockImageRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Image, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Image), args.Error(1)
}
func (m *MockImageRepository) List(ctx context.Context, limit, offset int) ([]*domain.Image, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Image), args.Error(1)
}
func (m *MockImageRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.ImageStatus, errMsg *string) error {
	return m.Called(ctx, id, status, errMsg).Error(0)
}
func (m *MockImageRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}
func (m *MockImageRepository) ListByStatus(ctx context.Context, status domain.ImageStatus) ([]*domain.Image, error) {
	args := m.Called(ctx, status)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Image), args.Error(1)
}

type MockVariantRepository struct{ mock.Mock }

func (m *MockVariantRepository) Create(ctx context.Context, v *domain.ImageVariant) error {
	return m.Called(ctx, v).Error(0)
}
func (m *MockVariantRepository) ListByImageID(ctx context.Context, imageID uuid.UUID) ([]*domain.ImageVariant, error) {
	args := m.Called(ctx, imageID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.ImageVariant), args.Error(1)
}
func (m *MockVariantRepository) ListByImageIDs(ctx context.Context, imageIDs []uuid.UUID) ([]*domain.ImageVariant, error) {
	args := m.Called(ctx, imageIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.ImageVariant), args.Error(1)
}
func (m *MockVariantRepository) DeleteByImageID(ctx context.Context, imageID uuid.UUID) error {
	return m.Called(ctx, imageID).Error(0)
}

type MockTaskPublisher struct{ mock.Mock }

func (m *MockTaskPublisher) Publish(ctx context.Context, task *domain.Task) error {
	return m.Called(ctx, task).Error(0)
}

type MockFileStorage struct{ mock.Mock }

func (m *MockFileStorage) Save(ctx context.Context, imageID uuid.UUID, filename string, data []byte) (string, error) {
	args := m.Called(ctx, imageID, filename, data)
	return args.String(0), args.Error(1)
}
func (m *MockFileStorage) Delete(ctx context.Context, imageID uuid.UUID) error {
	return m.Called(ctx, imageID).Error(0)
}

type MockProcessor struct{ mock.Mock }

func (m *MockProcessor) Process(ctx context.Context, imageID uuid.UUID, sourcePath, mimeType string, params map[string]any, store storage.FileStorage) (*processor.Result, error) {
	args := m.Called(ctx, imageID, sourcePath, mimeType, params, store)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*processor.Result), args.Error(1)
}

// ─── Хелперы ──────────────────────────────────────────────────────────────────

func newTestService(images *MockImageRepository, variants *MockVariantRepository, pub *MockTaskPublisher, store *MockFileStorage) *Service {
	log, _ := logger.InitLogger(logger.ZapEngine, "test", "test", logger.WithLevel(logger.ErrorLevel))
	cfg := &config.ProcessingConfig{
		Resize:    config.ResizeCfg{Width: 800, Height: 600},
		Thumbnail: config.ThumbnailCfg{Width: 200, Height: 150},
	}
	return New(images, variants, pub, store, log, cfg)
}

func newImage(status domain.ImageStatus) *domain.Image {
	return &domain.Image{
		ID:           uuid.New(),
		OriginalPath: "/storage/original.jpg",
		OriginalName: "photo.jpg",
		MIMEType:     "image/jpeg",
		Status:       status,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

// ─── Upload ───────────────────────────────────────────────────────────────────

func TestUpload_HappyPath(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	savedPath := "/storage/uuid/original.jpg"
	store.On("Save", mock.Anything, mock.AnythingOfType("uuid.UUID"), "original.jpg", mock.Anything).Return(savedPath, nil)
	images.On("Create", mock.Anything, mock.AnythingOfType("*domain.Image")).Return(nil)
	pub.On("Publish", mock.Anything, mock.AnythingOfType("*domain.Task")).Return(nil)

	img, err := svc.Upload(context.Background(), "photo.jpg", "image/jpeg", []byte("data"))

	require.NoError(t, err)
	require.NotNil(t, img)
	assert.Equal(t, domain.StatusPending, img.Status)
	assert.Equal(t, "photo.jpg", img.OriginalName)
	assert.Equal(t, savedPath, img.OriginalPath)
	store.AssertExpectations(t)
	images.AssertExpectations(t)
	pub.AssertExpectations(t)
}

func TestUpload_SaveError(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	store.On("Save", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", errors.New("disk full"))

	img, err := svc.Upload(context.Background(), "photo.jpg", "image/jpeg", []byte("data"))

	require.Error(t, err)
	assert.Nil(t, img)
	images.AssertNotCalled(t, "Create")
	pub.AssertNotCalled(t, "Publish")
}

func TestUpload_CreateError(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	store.On("Save", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("/path", nil)
	images.On("Create", mock.Anything, mock.Anything).Return(errors.New("db error"))

	_, err := svc.Upload(context.Background(), "photo.jpg", "image/jpeg", []byte("data"))

	require.Error(t, err)
	pub.AssertNotCalled(t, "Publish")
}

func TestUpload_PublishError(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	store.On("Save", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("/path", nil)
	images.On("Create", mock.Anything, mock.Anything).Return(nil)
	pub.On("Publish", mock.Anything, mock.Anything).Return(errors.New("kafka unavailable"))

	_, err := svc.Upload(context.Background(), "photo.jpg", "image/jpeg", []byte("data"))

	require.Error(t, err)
}

func TestUpload_ExtensionPNG(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	store.On("Save", mock.Anything, mock.Anything, "original.png", mock.Anything).Return("/path/original.png", nil)
	images.On("Create", mock.Anything, mock.Anything).Return(nil)
	pub.On("Publish", mock.Anything, mock.Anything).Return(nil)

	_, err := svc.Upload(context.Background(), "image.png", "image/png", []byte("png"))

	require.NoError(t, err)
	store.AssertCalled(t, "Save", mock.Anything, mock.Anything, "original.png", mock.Anything)
}

func TestUpload_ExtensionJPEG(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	store.On("Save", mock.Anything, mock.Anything, "original.jpg", mock.Anything).Return("/path/original.jpg", nil)
	images.On("Create", mock.Anything, mock.Anything).Return(nil)
	pub.On("Publish", mock.Anything, mock.Anything).Return(nil)

	_, err := svc.Upload(context.Background(), "image.bmp", "image/bmp", []byte("bmp"))

	require.NoError(t, err)
	store.AssertCalled(t, "Save", mock.Anything, mock.Anything, "original.jpg", mock.Anything)
}

// ─── GetImageInfo ─────────────────────────────────────────────────────────────

func TestGetImageInfo_HappyPath(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	img := newImage(domain.StatusDone)
	vars := []*domain.ImageVariant{
		{ID: uuid.New(), ImageID: img.ID, VariantType: "resize"},
		{ID: uuid.New(), ImageID: img.ID, VariantType: "thumbnail"},
	}
	images.On("GetByID", mock.Anything, img.ID).Return(img, nil)
	variants.On("ListByImageID", mock.Anything, img.ID).Return(vars, nil)

	gotImg, gotVars, err := svc.GetImageInfo(context.Background(), img.ID)

	require.NoError(t, err)
	assert.Equal(t, img, gotImg)
	assert.Equal(t, vars, gotVars)
}

func TestGetImageInfo_NotFound(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	id := uuid.New()
	images.On("GetByID", mock.Anything, id).Return(nil, domain.ErrNotFound)

	_, _, err := svc.GetImageInfo(context.Background(), id)

	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
	variants.AssertNotCalled(t, "ListByImageID")
}

func TestGetImageInfo_VariantsError(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	img := newImage(domain.StatusDone)
	images.On("GetByID", mock.Anything, img.ID).Return(img, nil)
	variants.On("ListByImageID", mock.Anything, img.ID).Return(nil, errors.New("db error"))

	_, _, err := svc.GetImageInfo(context.Background(), img.ID)

	require.Error(t, err)
}

// ─── ListImages ───────────────────────────────────────────────────────────────

func TestListImages_HappyPath(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	imgs := []*domain.Image{newImage(domain.StatusDone), newImage(domain.StatusDone)}
	ids := []uuid.UUID{imgs[0].ID, imgs[1].ID}
	vars := []*domain.ImageVariant{{ID: uuid.New(), ImageID: imgs[0].ID, VariantType: "resize"}}

	images.On("List", mock.Anything, 10, 0).Return(imgs, nil)
	variants.On("ListByImageIDs", mock.Anything, ids).Return(vars, nil)

	gotImgs, gotVars, err := svc.ListImages(context.Background(), 10, 0)

	require.NoError(t, err)
	assert.Equal(t, imgs, gotImgs)
	assert.Equal(t, vars, gotVars)
}

func TestListImages_ListError(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	images.On("List", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("db error"))

	_, _, err := svc.ListImages(context.Background(), 10, 0)

	require.Error(t, err)
	variants.AssertNotCalled(t, "ListByImageIDs")
}

func TestListImages_EmptyResult(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	images.On("List", mock.Anything, mock.Anything, mock.Anything).Return([]*domain.Image{}, nil)
	variants.On("ListByImageIDs", mock.Anything, []uuid.UUID{}).Return([]*domain.ImageVariant{}, nil)

	gotImgs, gotVars, err := svc.ListImages(context.Background(), 10, 0)

	require.NoError(t, err)
	assert.Empty(t, gotImgs)
	assert.Empty(t, gotVars)
}

func TestListImages_VariantsError(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	imgs := []*domain.Image{newImage(domain.StatusDone)}
	images.On("List", mock.Anything, mock.Anything, mock.Anything).Return(imgs, nil)
	variants.On("ListByImageIDs", mock.Anything, mock.Anything).Return(nil, errors.New("db error"))

	_, _, err := svc.ListImages(context.Background(), 10, 0)

	require.Error(t, err)
}

// ─── DeleteImage ──────────────────────────────────────────────────────────────

func TestDeleteImage_Pending(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	img := newImage(domain.StatusPending)
	images.On("GetByID", mock.Anything, img.ID).Return(img, nil)
	images.On("UpdateStatus", mock.Anything, img.ID, domain.StatusCancelled, (*string)(nil)).Return(nil)

	err := svc.DeleteImage(context.Background(), img.ID)

	require.NoError(t, err)
	store.AssertNotCalled(t, "Delete")
	images.AssertNotCalled(t, "Delete")
}

func TestDeleteImage_Processing(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	img := newImage(domain.StatusProcessing)
	images.On("GetByID", mock.Anything, img.ID).Return(img, nil)
	images.On("UpdateStatus", mock.Anything, img.ID, domain.StatusCancelled, (*string)(nil)).Return(nil)

	err := svc.DeleteImage(context.Background(), img.ID)

	require.NoError(t, err)
	store.AssertNotCalled(t, "Delete")
}

func TestDeleteImage_Done(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	img := newImage(domain.StatusDone)
	images.On("GetByID", mock.Anything, img.ID).Return(img, nil)
	store.On("Delete", mock.Anything, img.ID).Return(nil)
	images.On("Delete", mock.Anything, img.ID).Return(nil)

	err := svc.DeleteImage(context.Background(), img.ID)

	require.NoError(t, err)
	store.AssertExpectations(t)
	images.AssertExpectations(t)
}

func TestDeleteImage_Failed(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	img := newImage(domain.StatusFailed)
	images.On("GetByID", mock.Anything, img.ID).Return(img, nil)
	store.On("Delete", mock.Anything, img.ID).Return(nil)
	images.On("Delete", mock.Anything, img.ID).Return(nil)

	err := svc.DeleteImage(context.Background(), img.ID)

	require.NoError(t, err)
}

func TestDeleteImage_Cancelled(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	img := newImage(domain.StatusCancelled)
	images.On("GetByID", mock.Anything, img.ID).Return(img, nil)
	store.On("Delete", mock.Anything, img.ID).Return(nil)
	images.On("Delete", mock.Anything, img.ID).Return(nil)

	err := svc.DeleteImage(context.Background(), img.ID)

	require.NoError(t, err)
}

func TestDeleteImage_NotFound(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	id := uuid.New()
	images.On("GetByID", mock.Anything, id).Return(nil, domain.ErrNotFound)

	err := svc.DeleteImage(context.Background(), id)

	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestDeleteImage_StoreDeleteError_StillDeletesRecord(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	img := newImage(domain.StatusDone)
	images.On("GetByID", mock.Anything, img.ID).Return(img, nil)
	store.On("Delete", mock.Anything, img.ID).Return(errors.New("fs error"))
	images.On("Delete", mock.Anything, img.ID).Return(nil)

	err := svc.DeleteImage(context.Background(), img.ID)

	require.NoError(t, err)
	images.AssertCalled(t, "Delete", mock.Anything, img.ID)
}

func TestDeleteImage_RecordDeleteError(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	img := newImage(domain.StatusDone)
	images.On("GetByID", mock.Anything, img.ID).Return(img, nil)
	store.On("Delete", mock.Anything, img.ID).Return(nil)
	images.On("Delete", mock.Anything, img.ID).Return(errors.New("db error"))

	err := svc.DeleteImage(context.Background(), img.ID)

	require.Error(t, err)
}

// ─── ProcessTask ──────────────────────────────────────────────────────────────

func buildTask(imageID uuid.UUID, ops ...string) *domain.Task {
	operations := make([]domain.TaskOperation, len(ops))
	for i, op := range ops {
		operations[i] = domain.TaskOperation{
			Type:   op,
			Params: map[string]any{"width": 800, "height": 600},
		}
	}
	return &domain.Task{
		ImageID:    imageID,
		SourcePath: "/storage/original.jpg",
		MIMEType:   "image/jpeg",
		Operations: operations,
	}
}

func TestProcessTask_HappyPath(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	imageID := uuid.New()
	img := &domain.Image{ID: imageID, Status: domain.StatusPending}
	task := buildTask(imageID, "resize", "thumbnail")

	proc := &MockProcessor{}
	registry := processor.Registry{"resize": proc, "thumbnail": proc}

	images.On("UpdateStatus", mock.Anything, imageID, domain.StatusProcessing, (*string)(nil)).Return(nil)
	// GetByID вызывается перед каждой операцией
	images.On("GetByID", mock.Anything, imageID).Return(img, nil)
	proc.On("Process", mock.Anything, imageID, task.SourcePath, task.MIMEType, mock.Anything, store).
		Return(&processor.Result{FilePath: "/out/resize.jpg", Width: 800, Height: 600}, nil)
	variants.On("Create", mock.Anything, mock.AnythingOfType("*domain.ImageVariant")).Return(nil)
	images.On("UpdateStatus", mock.Anything, imageID, domain.StatusDone, (*string)(nil)).Return(nil)

	err := svc.ProcessTask(context.Background(), task, registry)

	require.NoError(t, err)
	proc.AssertNumberOfCalls(t, "Process", 2)
	variants.AssertNumberOfCalls(t, "Create", 2)
}

func TestProcessTask_UnknownOperation(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	imageID := uuid.New()
	img := &domain.Image{ID: imageID, Status: domain.StatusPending}
	task := buildTask(imageID, "unknown_op")

	images.On("UpdateStatus", mock.Anything, imageID, domain.StatusProcessing, (*string)(nil)).Return(nil)
	images.On("GetByID", mock.Anything, imageID).Return(img, nil)
	images.On("UpdateStatus", mock.Anything, imageID, domain.StatusFailed, mock.AnythingOfType("*string")).Return(nil)

	err := svc.ProcessTask(context.Background(), task, processor.Registry{})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnknownOperation))
}

func TestProcessTask_Cancelled(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	imageID := uuid.New()
	cancelledImg := &domain.Image{ID: imageID, Status: domain.StatusCancelled}
	task := buildTask(imageID, "resize")

	images.On("UpdateStatus", mock.Anything, imageID, domain.StatusProcessing, (*string)(nil)).Return(nil)
	images.On("GetByID", mock.Anything, imageID).Return(cancelledImg, nil)
	store.On("Delete", mock.Anything, imageID).Return(nil)

	err := svc.ProcessTask(context.Background(), task, processor.Registry{"resize": &MockProcessor{}})

	require.NoError(t, err)
	store.AssertCalled(t, "Delete", mock.Anything, imageID)
	variants.AssertNotCalled(t, "Create")
}

func TestProcessTask_ProcessorError(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	imageID := uuid.New()
	img := &domain.Image{ID: imageID, Status: domain.StatusPending}
	task := buildTask(imageID, "resize")

	proc := &MockProcessor{}
	images.On("UpdateStatus", mock.Anything, imageID, domain.StatusProcessing, (*string)(nil)).Return(nil)
	images.On("GetByID", mock.Anything, imageID).Return(img, nil)
	proc.On("Process", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("processing failed"))
	images.On("UpdateStatus", mock.Anything, imageID, domain.StatusFailed, mock.AnythingOfType("*string")).Return(nil)

	err := svc.ProcessTask(context.Background(), task, processor.Registry{"resize": proc})

	require.Error(t, err)
}

func TestProcessTask_VariantCreateError(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	imageID := uuid.New()
	img := &domain.Image{ID: imageID, Status: domain.StatusPending}
	task := buildTask(imageID, "resize")

	proc := &MockProcessor{}
	images.On("UpdateStatus", mock.Anything, imageID, domain.StatusProcessing, (*string)(nil)).Return(nil)
	images.On("GetByID", mock.Anything, imageID).Return(img, nil)
	proc.On("Process", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&processor.Result{FilePath: "/out.jpg", Width: 800, Height: 600}, nil)
	variants.On("Create", mock.Anything, mock.Anything).Return(errors.New("db error"))
	images.On("UpdateStatus", mock.Anything, imageID, domain.StatusFailed, mock.AnythingOfType("*string")).Return(nil)

	err := svc.ProcessTask(context.Background(), task, processor.Registry{"resize": proc})

	require.Error(t, err)
}

func TestProcessTask_SetProcessingError(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	imageID := uuid.New()
	task := buildTask(imageID, "resize")

	images.On("UpdateStatus", mock.Anything, imageID, domain.StatusProcessing, (*string)(nil)).Return(errors.New("db error"))

	err := svc.ProcessTask(context.Background(), task, processor.Registry{})

	require.Error(t, err)
}

// ─── RecoverProcessing ────────────────────────────────────────────────────────

func TestRecoverProcessing_HappyPath(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	img1 := newImage(domain.StatusProcessing)
	img2 := newImage(domain.StatusProcessing)
	imgs := []*domain.Image{img1, img2}

	images.On("ListByStatus", mock.Anything, domain.StatusProcessing).Return(imgs, nil)
	images.On("UpdateStatus", mock.Anything, img1.ID, domain.StatusPending, (*string)(nil)).Return(nil)
	images.On("UpdateStatus", mock.Anything, img2.ID, domain.StatusPending, (*string)(nil)).Return(nil)
	pub.On("Publish", mock.Anything, mock.AnythingOfType("*domain.Task")).Return(nil)

	svc.RecoverProcessing(context.Background())

	images.AssertNumberOfCalls(t, "UpdateStatus", 2)
	pub.AssertNumberOfCalls(t, "Publish", 2)
}

func TestRecoverProcessing_ListError(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	images.On("ListByStatus", mock.Anything, domain.StatusProcessing).Return(nil, errors.New("db error"))

	svc.RecoverProcessing(context.Background())

	images.AssertNotCalled(t, "UpdateStatus")
	pub.AssertNotCalled(t, "Publish")
}

func TestRecoverProcessing_UpdateStatusError_ContinuesForOthers(t *testing.T) {
	images := &MockImageRepository{}
	variants := &MockVariantRepository{}
	pub := &MockTaskPublisher{}
	store := &MockFileStorage{}
	svc := newTestService(images, variants, pub, store)

	img1 := newImage(domain.StatusProcessing)
	img2 := newImage(domain.StatusProcessing)
	imgs := []*domain.Image{img1, img2}

	images.On("ListByStatus", mock.Anything, domain.StatusProcessing).Return(imgs, nil)
	images.On("UpdateStatus", mock.Anything, img1.ID, domain.StatusPending, (*string)(nil)).Return(errors.New("db error"))
	images.On("UpdateStatus", mock.Anything, img2.ID, domain.StatusPending, (*string)(nil)).Return(nil)
	pub.On("Publish", mock.Anything, mock.AnythingOfType("*domain.Task")).Return(nil)

	svc.RecoverProcessing(context.Background())

	// img1 UpdateStatus упал — Publish для него не вызывается
	pub.AssertNumberOfCalls(t, "Publish", 1)
}

// ─── extensionForMIME ─────────────────────────────────────────────────────────

func TestExtensionForMIME(t *testing.T) {
	assert.Equal(t, ".png", extensionForMIME("image/png"))
	assert.Equal(t, ".jpg", extensionForMIME("image/jpeg"))
	assert.Equal(t, ".jpg", extensionForMIME("image/webp"))
	assert.Equal(t, ".jpg", extensionForMIME(""))
}
