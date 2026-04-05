package processor

import (
	"context"
	"fmt"
	"image"

	"github.com/google/uuid"
	"golang.org/x/image/draw"

	"github.com/ponchik327/ImageProcessor/internal/storage"
)

// ThumbnailProcessor создаёт центрально обрезанную миниатюру фиксированного размера.
type ThumbnailProcessor struct{}

// NewThumbnailProcessor создаёт ThumbnailProcessor.
func NewThumbnailProcessor() *ThumbnailProcessor {
	return &ThumbnailProcessor{}
}

// Process обрезает исходное изображение по центру до целевого соотношения сторон, затем масштабирует до целевых размеров.
// Ключи params: "width" (int), "height" (int).
func (p *ThumbnailProcessor) Process(
	ctx context.Context,
	imageID uuid.UUID,
	sourcePath string,
	mimeType string,
	params map[string]any,
	store storage.FileStorage,
) (*Result, error) {
	targetW, targetH, err := extractDimensions(params)
	if err != nil {
		return nil, fmt.Errorf("thumbnail: %w", err)
	}

	src, err := decodeImage(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("thumbnail: decode source: %w", err)
	}

	cropped := centerCrop(src, targetW, targetH)

	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), cropped, cropped.Bounds(), draw.Over, nil)

	data, ext, err := encodeImage(dst, mimeType)
	if err != nil {
		return nil, fmt.Errorf("thumbnail: encode: %w", err)
	}

	path, err := store.Save(ctx, imageID, "thumbnail"+ext, data)
	if err != nil {
		return nil, fmt.Errorf("thumbnail: save: %w", err)
	}

	return &Result{FilePath: path, Width: targetW, Height: targetH}, nil
}

// centerCrop возвращает наибольший центральный фрагмент src с соотношением сторон targetW:targetH.
func centerCrop(src image.Image, targetW, targetH int) image.Image {
	b := src.Bounds()
	srcW := b.Dx()
	srcH := b.Dy()

	cropW := srcW
	cropH := srcW * targetH / targetW

	if cropH > srcH {
		cropH = srcH
		cropW = srcH * targetW / targetH
	}

	x0 := b.Min.X + (srcW-cropW)/centerDivisor
	y0 := b.Min.Y + (srcH-cropH)/centerDivisor

	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}

	if si, ok := src.(subImager); ok {
		return si.SubImage(image.Rect(x0, y0, x0+cropW, y0+cropH))
	}

	// Запасной вариант: копируем пиксели в новое RGBA-изображение.
	dst := image.NewRGBA(image.Rect(0, 0, cropW, cropH))

	for y := range cropH {
		for x := range cropW {
			dst.Set(x, y, src.At(x0+x, y0+y))
		}
	}

	return dst
}
