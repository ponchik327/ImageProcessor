package processor

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	_ "image/png" // регистрируем декодер PNG для файлов наложения
	"os"

	"github.com/google/uuid"

	"github.com/ponchik327/ImageProcessor/internal/storage"
)

// WatermarkProcessor накладывает PNG-водяной знак на исходное изображение.
type WatermarkProcessor struct {
	overlayPath string
}

// NewWatermarkProcessor создаёт WatermarkProcessor, использующий overlayPath как PNG-наложение.
func NewWatermarkProcessor(overlayPath string) *WatermarkProcessor {
	return &WatermarkProcessor{overlayPath: overlayPath}
}

// Process накладывает водяной знак на исходное изображение.
// Ключи params: "position" (string) — одно из top-left, top-right, bottom-left, bottom-right, center.
// По умолчанию "bottom-right", если position отсутствует, не является строкой или является пустой строкой.
// Неизвестные значения position также дают "bottom-right".
func (p *WatermarkProcessor) Process(
	ctx context.Context,
	imageID uuid.UUID,
	sourcePath string,
	mimeType string,
	params map[string]any,
	store storage.FileStorage,
) (*Result, error) {
	position := "bottom-right"

	if pos, ok := params["position"]; ok {
		if s, ok := pos.(string); ok && s != "" {
			position = s
		}
	}

	src, err := decodeImage(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("watermark: decode source: %w", err)
	}

	overlay, err := decodePNG(p.overlayPath)
	if err != nil {
		return nil, fmt.Errorf("watermark: decode overlay: %w", err)
	}

	dst := image.NewRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, image.Point{}, draw.Src)

	offset := calcOffset(src.Bounds(), overlay.Bounds(), position)
	draw.Draw(dst, overlay.Bounds().Add(offset), overlay, image.Point{}, draw.Over)

	data, ext, err := encodeImage(dst, mimeType)
	if err != nil {
		return nil, fmt.Errorf("watermark: encode: %w", err)
	}

	path, err := store.Save(ctx, imageID, "watermark"+ext, data)
	if err != nil {
		return nil, fmt.Errorf("watermark: save: %w", err)
	}

	w := src.Bounds().Dx()
	h := src.Bounds().Dy()

	return &Result{FilePath: path, Width: w, Height: h}, nil
}

// decodePNG открывает и декодирует PNG-файл в image.Image.
func decodePNG(path string) (image.Image, error) {
	//nolint:gosec // путь берётся из конфига, а не из пользовательского ввода
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open overlay %s: %w", path, err)
	}

	img, _, decodeErr := image.Decode(f)
	if closeErr := f.Close(); closeErr != nil && decodeErr == nil {
		return nil, fmt.Errorf("close overlay %s: %w", path, closeErr)
	}

	if decodeErr != nil {
		return nil, fmt.Errorf("decode overlay %s: %w", path, decodeErr)
	}

	return img, nil
}

const centerDivisor = 2

// calcOffset вычисляет точку верхнего левого угла для размещения наложения внутри базового изображения.
func calcOffset(base, overlay image.Rectangle, position string) image.Point {
	bw := base.Dx()
	bh := base.Dy()
	ow := overlay.Dx()
	oh := overlay.Dy()

	const margin = 10

	switch position {
	case "top-left":
		return image.Point{X: margin, Y: margin}
	case "top-right":
		return image.Point{X: bw - ow - margin, Y: margin}
	case "bottom-left":
		return image.Point{X: margin, Y: bh - oh - margin}
	case "center":
		return image.Point{X: (bw - ow) / centerDivisor, Y: (bh - oh) / centerDivisor}
	default: // bottom-right
		return image.Point{X: bw - ow - margin, Y: bh - oh - margin}
	}
}
