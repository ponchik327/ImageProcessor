package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ImageStatus представляет состояние жизненного цикла обработки изображения.
type ImageStatus string

const (
	// StatusPending — изображение загружено и ожидает обработки.
	StatusPending ImageStatus = "pending"
	// StatusProcessing — воркер в данный момент обрабатывает изображение.
	StatusProcessing ImageStatus = "processing"
	// StatusDone — все операции обработки завершились успешно.
	StatusDone ImageStatus = "done"
	// StatusFailed — обработка завершилась с ошибкой; подробности в ErrorMessage.
	StatusFailed ImageStatus = "failed"
	// StatusCancelled — изображение было удалено до завершения обработки.
	StatusCancelled ImageStatus = "cancelled"
)

// Sentinel-ошибки доменного слоя.
var (
	// ErrNotFound возвращается, когда запись об изображении не существует.
	ErrNotFound = errors.New("image not found")
	// ErrConflict возвращается, когда операция конфликтует с текущим состоянием изображения.
	ErrConflict = errors.New("image status conflict")
)

// Image представляет запись о загруженном изображении.
type Image struct {
	ID           uuid.UUID
	OriginalPath string
	OriginalName string
	MIMEType     string
	Status       ImageStatus
	ErrorMessage *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ImageVariant представляет один обработанный вариант изображения (resize, thumbnail или watermark).
type ImageVariant struct {
	ID          uuid.UUID
	ImageID     uuid.UUID
	VariantType string
	FilePath    string
	Width       int
	Height      int
	CreatedAt   time.Time
}

// Task — payload Kafka-сообщения для задачи обработки изображения.
type Task struct {
	ImageID    uuid.UUID       `json:"image_id"`
	SourcePath string          `json:"source_path"`
	MIMEType   string          `json:"mime_type"`
	Operations []TaskOperation `json:"operations"`
}

// TaskOperation описывает один шаг обработки внутри Task.
type TaskOperation struct {
	Type   string         `json:"type"`
	Params map[string]any `json:"params"`
}
