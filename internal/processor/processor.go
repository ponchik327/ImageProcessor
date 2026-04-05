// Package processor определяет интерфейс обработки изображений и реестр обработчиков.
package processor

import (
	"context"

	"github.com/google/uuid"

	"github.com/ponchik327/ImageProcessor/internal/storage"
)

// Result содержит результат одной операции обработки изображения.
type Result struct {
	FilePath string
	Width    int
	Height   int
}

// Processor трансформирует изображение и сохраняет результат через FileStorage.
type Processor interface {
	// Process читает sourcePath, применяет трансформацию согласно params,
	// сохраняет результат через store и возвращает метаданные выходного файла.
	Process(
		ctx context.Context,
		imageID uuid.UUID,
		sourcePath string,
		mimeType string,
		params map[string]any,
		store storage.FileStorage,
	) (*Result, error)
}

// Registry отображает названия типов операций на реализации Processor.
type Registry map[string]Processor
