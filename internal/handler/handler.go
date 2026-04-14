// Package handler регистрирует все HTTP-маршруты и предоставляет вспомогательные функции для ответов.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/wb-go/wbf/logger"

	"github.com/ponchik327/ImageProcessor/internal/domain"
	"github.com/ponchik327/ImageProcessor/internal/storage"
)

// ImageService — минимальный контракт сервиса, необходимый HTTP-обработчикам.
type ImageService interface {
	Upload(ctx context.Context, filename, mimeType string, data []byte) (*domain.Image, error)
	GetImageInfo(ctx context.Context, id uuid.UUID) (*domain.Image, []*domain.ImageVariant, error)
	ListImages(ctx context.Context, limit, offset int) ([]*domain.Image, []*domain.ImageVariant, error)
	DeleteImage(ctx context.Context, id uuid.UUID) error
}

// Handler хранит общие зависимости для всех HTTP-обработчиков.
type Handler struct {
	svc   ImageService
	store storage.FileStorage
	log   logger.Logger
}

// New создаёт Handler.
func New(svc ImageService, store storage.FileStorage, log logger.Logger) *Handler {
	return &Handler{svc: svc, store: store, log: log}
}

// Routes собирает и возвращает HTTP-мультиплексор со всеми зарегистрированными маршрутами.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /upload", h.upload)
	mux.HandleFunc("GET /images", h.listImages)
	mux.HandleFunc("GET /image/{id}", h.getImage)
	mux.HandleFunc("GET /image/{id}/file", h.getFile)
	mux.HandleFunc("DELETE /image/{id}", h.deleteImage)

	// Раздаём веб-интерфейс и статические ресурсы из директории web/.
	mux.Handle("/", http.FileServer(http.Dir("web/")))

	return h.Middleware(mux)
}

// Middleware логирует каждый запрос: метод, путь, код статуса и длительность.
func (h *Handler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ctx := logger.SetRequestID(r.Context(), logger.GenerateRequestID())
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r.WithContext(ctx))

		h.log.LogRequest(ctx, r.Method, r.URL.Path, rw.statusCode, time.Since(start))
	})
}

// responseWriter оборачивает http.ResponseWriter для перехвата записанного кода статуса.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// writeJSON кодирует v как JSON и записывает его с указанным statusCode.
func writeJSON(w http.ResponseWriter, statusCode int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		// На этом этапе заголовок уже отправлен; ничего больше сделать нельзя.
		return
	}
}

// writeError записывает JSON-ответ с ошибкой.
func writeError(w http.ResponseWriter, statusCode int, msg string) {
	writeJSON(w, statusCode, map[string]string{"error": msg})
}
