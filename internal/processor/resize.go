package processor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"

	"github.com/google/uuid"
	"golang.org/x/image/draw"

	"github.com/ponchik327/ImageProcessor/internal/storage"
)

// ErrMissingParam возвращается, когда обязательный параметр обработки отсутствует.
var ErrMissingParam = errors.New("missing param")

// ErrUnexpectedParamType возвращается, когда параметр имеет неподдерживаемый тип.
var ErrUnexpectedParamType = errors.New("unexpected param type")

const jpegQuality = 90

// ResizeProcessor вписывает изображение в заданные максимальные размеры, сохраняя соотношение сторон
// и никогда не увеличивая его. Использует интерполяцию CatmullRom.
type ResizeProcessor struct{}

// NewResizeProcessor создаёт ResizeProcessor.
func NewResizeProcessor() *ResizeProcessor {
	return &ResizeProcessor{}
}

// Process вписывает изображение в ограничивающий прямоугольник, заданный params,
// сохраняя исходное соотношение сторон. Изображение никогда не увеличивается.
// Ключи params: "width" (int), "height" (int) — максимальные размеры ограничивающего прямоугольника.
func (p *ResizeProcessor) Process(
	ctx context.Context,
	imageID uuid.UUID,
	sourcePath string,
	mimeType string,
	params map[string]any,
	store storage.FileStorage,
) (*Result, error) {
	maxW, maxH, err := extractDimensions(params)
	if err != nil {
		return nil, fmt.Errorf("resize: %w", err)
	}

	src, err := decodeImage(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("resize: decode source: %w", err)
	}

	finalW, finalH := fitIntoBox(src.Bounds().Dx(), src.Bounds().Dy(), maxW, maxH)

	dst := image.NewRGBA(image.Rect(0, 0, finalW, finalH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	data, filename, err := encodeImage(dst, mimeType)
	if err != nil {
		return nil, fmt.Errorf("resize: encode: %w", err)
	}

	outName := "resize" + filename

	path, err := store.Save(ctx, imageID, outName, data)
	if err != nil {
		return nil, fmt.Errorf("resize: save: %w", err)
	}

	return &Result{FilePath: path, Width: finalW, Height: finalH}, nil
}

// fitIntoBox вычисляет наибольшие размеры, вписывающие origW×origH в maxW×maxH,
// сохраняя соотношение сторон. Результат никогда не превышает исходный (без увеличения).
func fitIntoBox(origW, origH, maxW, maxH int) (int, int) {
	ratioW := float64(maxW) / float64(origW)
	ratioH := float64(maxH) / float64(origH)

	scale := min(ratioW, ratioH)
	if scale > 1 {
		scale = 1
	}

	return max(1, int(float64(origW)*scale)), max(1, int(float64(origH)*scale))
}

// decodeImage открывает и декодирует любой поддерживаемый файл изображения.
func decodeImage(path string) (image.Image, error) {
	//nolint:gosec // путь берётся из управляемого хранилища, а не напрямую из пользовательского ввода
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	img, _, decodeErr := image.Decode(f)
	if closeErr := f.Close(); closeErr != nil && decodeErr == nil {
		return nil, fmt.Errorf("close %s: %w", path, closeErr)
	}

	if decodeErr != nil {
		return nil, fmt.Errorf("decode %s: %w", path, decodeErr)
	}

	return img, nil
}

// encodeImage кодирует img в формат, соответствующий mimeType, и возвращает байты и расширение файла.
func encodeImage(img image.Image, mimeType string) ([]byte, string, error) {
	var buf bytes.Buffer

	if mimeType == "image/png" {
		if err := png.Encode(&buf, img); err != nil {
			return nil, "", fmt.Errorf("png encode: %w", err)
		}

		return buf.Bytes(), ".png", nil
	}

	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, "", fmt.Errorf("jpeg encode: %w", err)
	}

	return buf.Bytes(), ".jpg", nil
}

// extractDimensions читает "width" и "height" из params как целые числа.
func extractDimensions(params map[string]any) (int, int, error) {
	w, err := paramInt(params, "width")
	if err != nil {
		return 0, 0, err
	}

	h, err := paramInt(params, "height")
	if err != nil {
		return 0, 0, err
	}

	return w, h, nil
}

// paramInt читает целочисленное значение из params по ключу.
func paramInt(params map[string]any, key string) (int, error) {
	v, ok := params[key]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrMissingParam, key)
	}

	switch val := v.(type) {
	case int:
		return val, nil
	case float64:
		return int(val), nil
	case int64:
		return int(val), nil
	default:
		return 0, fmt.Errorf("%w: %q has type %T", ErrUnexpectedParamType, key, v)
	}
}
