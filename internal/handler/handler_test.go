package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/wb-go/wbf/logger"

	"github.com/ponchik327/ImageProcessor/internal/domain"
)

// ─── Мок сервиса ──────────────────────────────────────────────────────────────

type MockImageService struct{ mock.Mock }

func (m *MockImageService) Upload(ctx context.Context, filename, mimeType string, data []byte) (*domain.Image, error) {
	args := m.Called(ctx, filename, mimeType, data)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Image), args.Error(1)
}

func (m *MockImageService) GetImageInfo(ctx context.Context, id uuid.UUID) (*domain.Image, []*domain.ImageVariant, error) {
	args := m.Called(ctx, id)
	img, _ := args.Get(0).(*domain.Image)
	vars, _ := args.Get(1).([]*domain.ImageVariant)
	return img, vars, args.Error(2)
}

func (m *MockImageService) ListImages(ctx context.Context, limit, offset int) ([]*domain.Image, []*domain.ImageVariant, error) {
	args := m.Called(ctx, limit, offset)
	imgs, _ := args.Get(0).([]*domain.Image)
	vars, _ := args.Get(1).([]*domain.ImageVariant)
	return imgs, vars, args.Error(2)
}

func (m *MockImageService) DeleteImage(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}

// ─── Хелперы ──────────────────────────────────────────────────────────────────

func newTestHandler(svc *MockImageService) *Handler {
	log, _ := logger.InitLogger(logger.ZapEngine, "test", "test", logger.WithLevel(logger.ErrorLevel))
	return &Handler{svc: svc, log: log}
}

// do отправляет запрос через Routes() мукса, возвращает ResponseRecorder.
func do(h *Handler, r *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, r)
	return rr
}

// buildMultipartRequest формирует multipart/form-data запрос для POST /upload.
func buildMultipartRequest(t *testing.T, fieldName, filename, contentType string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	part, err := w.CreateFormFile(fieldName, filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func sampleImage(status domain.ImageStatus) *domain.Image {
	return &domain.Image{
		ID:           uuid.New(),
		OriginalName: "photo.jpg",
		MIMEType:     "image/jpeg",
		Status:       status,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

// ─── Upload ───────────────────────────────────────────────────────────────────

func TestUpload_HappyPath(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	img := sampleImage(domain.StatusPending)
	svc.On("Upload", mock.Anything, "photo.jpg", mock.Anything, mock.Anything).Return(img, nil)

	req := buildMultipartRequest(t, "file", "photo.jpg", "image/jpeg", []byte("jpeg-data"))
	rr := do(h, req)

	assert.Equal(t, http.StatusCreated, rr.Code)

	var resp uploadResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, img.ID, resp.ID)
	assert.Equal(t, img.OriginalName, resp.Name)
	assert.Equal(t, domain.StatusPending, resp.Status)
	svc.AssertExpectations(t)
}

func TestUpload_MissingFileField(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	require.NoError(t, w.Close())

	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rr := do(h, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	svc.AssertNotCalled(t, "Upload")
}

func TestUpload_ServiceError(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	svc.On("Upload", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("upload failed"))

	req := buildMultipartRequest(t, "file", "photo.jpg", "image/jpeg", []byte("data"))
	rr := do(h, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// ─── GetImage ─────────────────────────────────────────────────────────────────

func TestGetImage_HappyPath(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	img := sampleImage(domain.StatusDone)
	vars := []*domain.ImageVariant{
		{ID: uuid.New(), ImageID: img.ID, VariantType: "resize", Width: 800, Height: 600},
	}
	svc.On("GetImageInfo", mock.Anything, img.ID).Return(img, vars, nil)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/image/%s", img.ID), nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp imageDetailResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, img.ID, resp.ID)
	assert.Equal(t, domain.StatusDone, resp.Status)
	assert.Len(t, resp.Variants, 1)
	assert.Equal(t, "resize", resp.Variants[0].Type)
}

func TestGetImage_InvalidUUID(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/image/not-a-uuid", nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	svc.AssertNotCalled(t, "GetImageInfo")
}

func TestGetImage_NotFound(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	id := uuid.New()
	svc.On("GetImageInfo", mock.Anything, id).Return(nil, nil, fmt.Errorf("wrapped: %w", domain.ErrNotFound))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/image/%s", id), nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestGetImage_ServiceError(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	id := uuid.New()
	svc.On("GetImageInfo", mock.Anything, id).Return(nil, nil, errors.New("db error"))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/image/%s", id), nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// ─── GetFile ──────────────────────────────────────────────────────────────────

func TestGetFile_HappyPath(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	// Создаём реальный файл на диске — getFile читает через os.Open
	dir := t.TempDir()
	filePath := filepath.Join(dir, "resize.jpg")
	require.NoError(t, os.WriteFile(filePath, []byte("jpeg-content"), 0o640))

	img := sampleImage(domain.StatusDone)
	vars := []*domain.ImageVariant{
		{ID: uuid.New(), ImageID: img.ID, VariantType: "resize", FilePath: filePath},
	}
	svc.On("GetImageInfo", mock.Anything, img.ID).Return(img, vars, nil)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/image/%s/file?variant=resize", img.ID), nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Header().Get("Content-Disposition"), "resize.jpg")
	assert.Equal(t, "jpeg-content", rr.Body.String())
}

func TestGetFile_MissingVariantParam(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	id := uuid.New()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/image/%s/file", id), nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	svc.AssertNotCalled(t, "GetImageInfo")
}

func TestGetFile_InvalidUUID(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/image/bad-uuid/file?variant=resize", nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestGetFile_NotFound(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	id := uuid.New()
	svc.On("GetImageInfo", mock.Anything, id).Return(nil, nil, fmt.Errorf("w: %w", domain.ErrNotFound))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/image/%s/file?variant=resize", id), nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestGetFile_NotDoneStatus(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	img := sampleImage(domain.StatusProcessing)
	svc.On("GetImageInfo", mock.Anything, img.ID).Return(img, []*domain.ImageVariant{}, nil)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/image/%s/file?variant=resize", img.ID), nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusConflict, rr.Code)
}

func TestGetFile_VariantNotFound(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	img := sampleImage(domain.StatusDone)
	vars := []*domain.ImageVariant{
		{ID: uuid.New(), ImageID: img.ID, VariantType: "thumbnail"},
	}
	svc.On("GetImageInfo", mock.Anything, img.ID).Return(img, vars, nil)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/image/%s/file?variant=resize", img.ID), nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestGetFile_FileNotOnDisk(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	img := sampleImage(domain.StatusDone)
	vars := []*domain.ImageVariant{
		{ID: uuid.New(), ImageID: img.ID, VariantType: "resize", FilePath: "/nonexistent/path/resize.jpg"},
	}
	svc.On("GetImageInfo", mock.Anything, img.ID).Return(img, vars, nil)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/image/%s/file?variant=resize", img.ID), nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// ─── ListImages ───────────────────────────────────────────────────────────────

func TestListImages_HappyPath(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	imgs := []*domain.Image{sampleImage(domain.StatusDone)}
	vars := []*domain.ImageVariant{{ID: uuid.New(), ImageID: imgs[0].ID, VariantType: "resize"}}
	svc.On("ListImages", mock.Anything, 50, 0).Return(imgs, vars, nil)

	req := httptest.NewRequest(http.MethodGet, "/images", nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var items []imageListItem
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &items))
	assert.Len(t, items, 1)
}

func TestListImages_DefaultLimitOffset(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	svc.On("ListImages", mock.Anything, defaultLimit, 0).Return([]*domain.Image{}, []*domain.ImageVariant{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/images", nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	svc.AssertCalled(t, "ListImages", mock.Anything, defaultLimit, 0)
}

func TestListImages_LimitClamped(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	svc.On("ListImages", mock.Anything, maxLimit, 0).Return([]*domain.Image{}, []*domain.ImageVariant{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/images?limit=9999", nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	svc.AssertCalled(t, "ListImages", mock.Anything, maxLimit, 0)
}

func TestListImages_ServiceError(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	svc.On("ListImages", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil, errors.New("db error"))

	req := httptest.NewRequest(http.MethodGet, "/images", nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// ─── DeleteImage ──────────────────────────────────────────────────────────────

func TestDeleteImage_HappyPath(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	id := uuid.New()
	svc.On("DeleteImage", mock.Anything, id).Return(nil)

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/image/%s", id), nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
}

func TestDeleteImage_InvalidUUID(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodDelete, "/image/not-a-uuid", nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	svc.AssertNotCalled(t, "DeleteImage")
}

func TestDeleteImage_NotFound(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	id := uuid.New()
	svc.On("DeleteImage", mock.Anything, id).Return(fmt.Errorf("w: %w", domain.ErrNotFound))

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/image/%s", id), nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestDeleteImage_ServiceError(t *testing.T) {
	svc := &MockImageService{}
	h := newTestHandler(svc)

	id := uuid.New()
	svc.On("DeleteImage", mock.Anything, id).Return(errors.New("db error"))

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/image/%s", id), nil)
	rr := do(h, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}
