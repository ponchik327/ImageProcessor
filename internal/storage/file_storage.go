package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

const (
	dirPerm  = 0o750
	filePerm = 0o640
)

// LocalStorage реализует FileStorage на основе локальной файловой системы.
// Файлы хранятся по пути basePath/<imageID>/<filename>.
type LocalStorage struct {
	basePath string
}

// New создаёт LocalStorage с корневым каталогом basePath.
func New(basePath string) (*LocalStorage, error) {
	if err := os.MkdirAll(basePath, dirPerm); err != nil {
		return nil, fmt.Errorf("storage: create base dir %s: %w", basePath, err)
	}
	return &LocalStorage{basePath: basePath}, nil
}

// Save записывает данные изображения в basePath/<imageID>/<filename> и возвращает полный путь к файлу.
func (s *LocalStorage) Save(ctx context.Context, imageID uuid.UUID, filename string, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return s.write(imageID, filename, data)
}

// Delete удаляет всё дерево каталогов по пути basePath/<imageID>.
func (s *LocalStorage) Delete(ctx context.Context, imageID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	dir := filepath.Join(s.basePath, imageID.String())

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("storage: delete image dir %s: %w", dir, err)
	}

	return nil
}

func (s *LocalStorage) write(imageID uuid.UUID, filename string, data []byte) (path string, err error) {
	dir := filepath.Join(s.basePath, imageID.String())

	if err = os.Mkdir(dir, dirPerm); err != nil {
		return "", fmt.Errorf("storage: create image dir %s: %w", dir, err)
	}
	defer func() {
		if rmErr := os.Remove(dir); rmErr != nil {
			err = errors.Join(err, fmt.Errorf("storage: rollback dir %s: %w", dir, rmErr))
		}
	}()

	path = filepath.Join(dir, filename)

	if err = os.WriteFile(path, data, filePerm); err != nil {
		return "", fmt.Errorf("storage: write file %s: %w", path, err)
	}

	return path, nil
}
