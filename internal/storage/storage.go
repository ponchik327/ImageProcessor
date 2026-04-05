// Package storage определяет интерфейс FileStorage для сохранения файлов изображений.
package storage

import (
	"context"

	"github.com/google/uuid"
)

// FileStorage абстрагирует локальное (или удалённое) хранение файлов изображений.
type FileStorage interface {
	// Save записывает байты изображения в директорию изображения и возвращает путь к сохранённому файлу.
	Save(ctx context.Context, imageID uuid.UUID, filename string, data []byte) (string, error)
	// Delete удаляет всю директорию заданного изображения, включая все его варианты.
	Delete(ctx context.Context, imageID uuid.UUID) error
}
