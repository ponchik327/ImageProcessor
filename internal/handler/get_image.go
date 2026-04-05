package handler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/ponchik327/ImageProcessor/internal/domain"
)

// variantInfo — секция варианта в ответе с детальной информацией об изображении.
type variantInfo struct {
	Type   string `json:"type"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// imageDetailResponse — JSON-тело ответа для GET /image/{id}.
type imageDetailResponse struct {
	ID           uuid.UUID          `json:"id"`
	OriginalName string             `json:"original_name"`
	MIMEType     string             `json:"mime_type"`
	Status       domain.ImageStatus `json:"status"`
	ErrorMessage *string            `json:"error_message,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
	Variants     []variantInfo      `json:"variants,omitempty"`
}

// getImage обрабатывает GET /image/{id}.
func (h *Handler) getImage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid image id")

		return
	}

	img, variants, err := h.svc.GetImageInfo(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "image not found")

			return
		}

		h.log.Error("get_image: service error", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")

		return
	}

	resp := imageDetailResponse{
		ID:           img.ID,
		OriginalName: img.OriginalName,
		MIMEType:     img.MIMEType,
		Status:       img.Status,
		ErrorMessage: img.ErrorMessage,
		CreatedAt:    img.CreatedAt,
		UpdatedAt:    img.UpdatedAt,
	}

	for _, v := range variants {
		resp.Variants = append(resp.Variants, variantInfo{
			Type:   v.VariantType,
			URL:    fmt.Sprintf("/image/%s/file?variant=%s", img.ID, v.VariantType),
			Width:  v.Width,
			Height: v.Height,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// parseID извлекает и парсит значение {id} из пути запроса.
func parseID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(r.PathValue("id"))
}
