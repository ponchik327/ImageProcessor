// Package service содержит основную бизнес-логику загрузки и обработки изображений.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/wb-go/wbf/logger"

	"github.com/ponchik327/ImageProcessor/internal/config"
	"github.com/ponchik327/ImageProcessor/internal/domain"
	"github.com/ponchik327/ImageProcessor/internal/processor"
	"github.com/ponchik327/ImageProcessor/internal/storage"
)

// ImageRepository определяет операции сохранения записей об изображениях.
type ImageRepository interface {
	Create(ctx context.Context, img *domain.Image) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Image, error)
	List(ctx context.Context, limit, offset int) ([]*domain.Image, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.ImageStatus, errMsg *string) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByStatus(ctx context.Context, status domain.ImageStatus) ([]*domain.Image, error)
}

// VariantRepository определяет операции сохранения записей о вариантах изображений.
type VariantRepository interface {
	Create(ctx context.Context, v *domain.ImageVariant) error
	ListByImageID(ctx context.Context, imageID uuid.UUID) ([]*domain.ImageVariant, error)
	ListByImageIDs(ctx context.Context, imageIDs []uuid.UUID) ([]*domain.ImageVariant, error)
	DeleteByImageID(ctx context.Context, imageID uuid.UUID) error
}

// TaskPublisher отправляет задачи обработки в брокер сообщений.
type TaskPublisher interface {
	Publish(ctx context.Context, task *domain.Task) error
}

// ErrUnknownOperation возвращается, когда задача содержит незарегистрированный тип операции.
var ErrUnknownOperation = errors.New("unknown operation")

// Service оркестрирует загрузку, получение, удаление и обработку изображений.
type Service struct {
	images    ImageRepository
	variants  VariantRepository
	publisher TaskPublisher
	store     storage.FileStorage
	log       logger.Logger
	cfg       *config.ProcessingConfig
}

// New создаёт Service со всеми необходимыми зависимостями.
func New(
	images ImageRepository,
	variants VariantRepository,
	publisher TaskPublisher,
	store storage.FileStorage,
	log logger.Logger,
	cfg *config.ProcessingConfig,
) *Service {
	return &Service{
		images:    images,
		variants:  variants,
		publisher: publisher,
		store:     store,
		log:       log,
		cfg:       cfg,
	}
}

// Upload сохраняет байты изображения, создаёт запись в БД и публикует задачу обработки.
func (s *Service) Upload(ctx context.Context, filename, mimeType string, data []byte) (*domain.Image, error) {
	imageID := uuid.New()
	now := time.Now()

	ext := extensionForMIME(mimeType)
	originalFilename := "original" + ext

	path, err := s.store.Save(ctx, imageID, originalFilename, data)
	if err != nil {
		return nil, fmt.Errorf("service: save original: %w", err)
	}

	img := &domain.Image{
		ID:           imageID,
		OriginalPath: path,
		OriginalName: filename,
		MIMEType:     mimeType,
		Status:       domain.StatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err = s.images.Create(ctx, img); err != nil {
		return nil, fmt.Errorf("service: create image record: %w", err)
	}

	task := s.buildTask(imageID, path, mimeType)

	if err = s.publisher.Publish(ctx, task); err != nil {
		return nil, fmt.Errorf("service: publish task: %w", err)
	}

	return img, nil
}

// GetImageInfo возвращает изображение и его варианты. Возвращает domain.ErrNotFound, если запись отсутствует.
func (s *Service) GetImageInfo(ctx context.Context, id uuid.UUID) (*domain.Image, []*domain.ImageVariant, error) {
	img, err := s.images.GetByID(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("service: get image: %w", err)
	}

	variants, err := s.variants.ListByImageID(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("service: list variants: %w", err)
	}

	return img, variants, nil
}

// ListImages возвращает страницу изображений и их вариантов, упорядоченных по времени загрузки по убыванию.
func (s *Service) ListImages(ctx context.Context, limit, offset int) ([]*domain.Image, []*domain.ImageVariant, error) {
	imgs, err := s.images.List(ctx, limit, offset)
	if err != nil {
		return nil, nil, fmt.Errorf("service: list images: %w", err)
	}

	ids := make([]uuid.UUID, len(imgs))
	for i, img := range imgs {
		ids[i] = img.ID
	}

	variants, err := s.variants.ListByImageIDs(ctx, ids)
	if err != nil {
		return nil, nil, fmt.Errorf("service: list variants for images: %w", err)
	}

	return imgs, variants, nil
}

// DeleteImage отменяет или удаляет изображение в зависимости от его текущего статуса.
// Для изображений со статусом done/failed/cancelled удаляются все файлы и запись в БД.
// Для изображений со статусом pending/processing статус изменяется на cancelled.
func (s *Service) DeleteImage(ctx context.Context, id uuid.UUID) error {
	img, err := s.images.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("service: delete – get image: %w", err)
	}

	switch img.Status {
	case domain.StatusPending, domain.StatusProcessing:
		if err = s.images.UpdateStatus(ctx, id, domain.StatusCancelled, nil); err != nil {
			return fmt.Errorf("service: cancel image: %w", err)
		}
	case domain.StatusDone, domain.StatusFailed, domain.StatusCancelled:
		if err = s.store.Delete(ctx, id); err != nil {
			s.log.Warn("service: delete files failed", "image_id", id, "error", err)
		}

		if err = s.images.Delete(ctx, id); err != nil {
			return fmt.Errorf("service: delete image record: %w", err)
		}
	}

	return nil
}

// ProcessTask выполняет все операции из Task-сообщения.
// Реализует кооперативную отмену: перед каждой операцией проверяется текущий статус в БД;
// если изображение отменено, воркер останавливается и удаляет частично созданные файлы.
func (s *Service) ProcessTask(ctx context.Context, task *domain.Task, registry processor.Registry) error {
	if err := s.images.UpdateStatus(ctx, task.ImageID, domain.StatusProcessing, nil); err != nil {
		return fmt.Errorf("service: set processing status: %w", err)
	}

	for _, op := range task.Operations {
		img, err := s.images.GetByID(ctx, task.ImageID)
		if err != nil {
			return fmt.Errorf("service: check status before %s: %w", op.Type, err)
		}

		if img.Status == domain.StatusCancelled {
			s.log.Info("service: processing cancelled, cleaning up", "image_id", task.ImageID)

			if err = s.store.Delete(ctx, task.ImageID); err != nil {
				s.log.Warn("service: delete files on cancel", "image_id", task.ImageID, "error", err)
			}

			return nil
		}

		proc, ok := registry[op.Type]
		if !ok {
			errMsg := fmt.Sprintf("unknown operation %q", op.Type)

			if err = s.images.UpdateStatus(ctx, task.ImageID, domain.StatusFailed, &errMsg); err != nil {
				s.log.Error("service: update status to failed", "image_id", task.ImageID, "error", err)
			}

			return fmt.Errorf("service: %w: %q", ErrUnknownOperation, op.Type)
		}

		result, err := proc.Process(ctx, task.ImageID, task.SourcePath, task.MIMEType, op.Params, s.store)
		if err != nil {
			errMsg := err.Error()

			if statusErr := s.images.UpdateStatus(ctx, task.ImageID, domain.StatusFailed, &errMsg); statusErr != nil {
				s.log.Error("service: update status to failed", "image_id", task.ImageID, "error", statusErr)
			}

			return fmt.Errorf("service: process %s: %w", op.Type, err)
		}

		variant := &domain.ImageVariant{
			ID:          uuid.New(),
			ImageID:     task.ImageID,
			VariantType: op.Type,
			FilePath:    result.FilePath,
			Width:       result.Width,
			Height:      result.Height,
			CreatedAt:   time.Now(),
		}

		if err = s.variants.Create(ctx, variant); err != nil {
			errMsg := err.Error()

			if statusErr := s.images.UpdateStatus(ctx, task.ImageID, domain.StatusFailed, &errMsg); statusErr != nil {
				s.log.Error("service: update status to failed", "image_id", task.ImageID, "error", statusErr)
			}

			return fmt.Errorf("service: save variant %s: %w", op.Type, err)
		}
	}

	if err := s.images.UpdateStatus(ctx, task.ImageID, domain.StatusDone, nil); err != nil {
		return fmt.Errorf("service: set done status: %w", err)
	}

	return nil
}

// RecoverProcessing сбрасывает все изображения, зависшие в статусе "processing", обратно в "pending"
// и повторно публикует их задачи. Вызывается один раз при запуске для восстановления после сбоя.
func (s *Service) RecoverProcessing(ctx context.Context) {
	imgs, err := s.images.ListByStatus(ctx, domain.StatusProcessing)
	if err != nil {
		s.log.Error("service: recovery – list processing images", "error", err)

		return
	}

	for _, img := range imgs {
		if err = s.images.UpdateStatus(ctx, img.ID, domain.StatusPending, nil); err != nil {
			s.log.Error("service: recovery – reset to pending", "image_id", img.ID, "error", err)

			continue
		}

		task := s.buildTask(img.ID, img.OriginalPath, img.MIMEType)

		if err = s.publisher.Publish(ctx, task); err != nil {
			s.log.Error("service: recovery – re-publish task", "image_id", img.ID, "error", err)
		}
	}

	if len(imgs) > 0 {
		s.log.Info("service: recovery complete", "recovered", len(imgs))
	}
}

// buildTask создаёт domain.Task с тремя стандартными операциями.
func (s *Service) buildTask(imageID uuid.UUID, sourcePath, mimeType string) *domain.Task {
	return &domain.Task{
		ImageID:    imageID,
		SourcePath: sourcePath,
		MIMEType:   mimeType,
		Operations: []domain.TaskOperation{
			{
				Type: "resize",
				Params: map[string]any{
					"width":  s.cfg.Resize.Width,
					"height": s.cfg.Resize.Height,
				},
			},
			{
				Type: "thumbnail",
				Params: map[string]any{
					"width":  s.cfg.Thumbnail.Width,
					"height": s.cfg.Thumbnail.Height,
				},
			},
			{
				Type:   "watermark",
				Params: map[string]any{"position": "bottom-right"},
			},
		},
	}
}

// extensionForMIME возвращает расширение файла для заданного MIME-типа.
func extensionForMIME(mimeType string) string {
	if mimeType == "image/png" {
		return ".png"
	}

	return ".jpg"
}
