package handler

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/ponchik327/ImageProcessor/internal/domain"
)

const maxUploadSize = 32 << 20 // 32 МБ

// uploadResponse — JSON-тело ответа для POST /upload.
type uploadResponse struct {
	ID        uuid.UUID          `json:"id"`
	Name      string             `json:"original_name"`
	MIMEType  string             `json:"mime_type"`
	Status    domain.ImageStatus `json:"status"`
	CreatedAt time.Time          `json:"created_at"`
}

// upload обрабатывает POST /upload.
// Читает поле "file" из multipart-формы, сохраняет изображение и публикует задачу обработки.
func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "request too large or not multipart")

		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")

		return
	}

	defer func() {
		if err := file.Close(); err != nil {
			h.log.Warn("upload: close multipart file", "error", err)
		}
	}()

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	data := make([]byte, header.Size)

	if _, err = file.Read(data); err != nil {
		h.log.Error("upload: read file", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to read file")

		return
	}

	img, err := h.svc.Upload(r.Context(), header.Filename, mimeType, data)
	if err != nil {
		h.log.Error("upload: service error", "error", err)
		writeError(w, http.StatusInternalServerError, "upload failed")

		return
	}

	writeJSON(w, http.StatusCreated, uploadResponse{
		ID:        img.ID,
		Name:      img.OriginalName,
		MIMEType:  img.MIMEType,
		Status:    img.Status,
		CreatedAt: img.CreatedAt,
	})
}
