package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/ponchik327/ImageProcessor/internal/domain"
)

const (
	defaultLimit = 50
	maxLimit     = 200
)

// imageListItem — одна запись в ответе GET /images.
type imageListItem struct {
	ID        uuid.UUID          `json:"id"`
	Name      string             `json:"original_name"`
	MIMEType  string             `json:"mime_type"`
	Status    domain.ImageStatus `json:"status"`
	CreatedAt time.Time          `json:"created_at"`
	Variants  []variantInfo      `json:"variants,omitempty"`
}

// listImages обрабатывает GET /images?limit=N&offset=M.
func (h *Handler) listImages(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", defaultLimit)
	offset := queryInt(r, "offset", 0)

	if limit > maxLimit {
		limit = maxLimit
	}

	imgs, variants, err := h.svc.ListImages(r.Context(), limit, offset)
	if err != nil {
		h.log.Error("list_images: service error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")

		return
	}

	// Индексируем варианты по ID изображения для поиска за O(1).
	variantsByImage := make(map[uuid.UUID][]variantInfo, len(variants))
	for _, v := range variants {
		variantsByImage[v.ImageID] = append(variantsByImage[v.ImageID], variantInfo{
			Type:   v.VariantType,
			URL:    fmt.Sprintf("/image/%s/file?variant=%s", v.ImageID, v.VariantType),
			Width:  v.Width,
			Height: v.Height,
		})
	}

	items := make([]imageListItem, 0, len(imgs))

	for _, img := range imgs {
		items = append(items, imageListItem{
			ID:        img.ID,
			Name:      img.OriginalName,
			MIMEType:  img.MIMEType,
			Status:    img.Status,
			CreatedAt: img.CreatedAt,
			Variants:  variantsByImage[img.ID],
		})
	}

	writeJSON(w, http.StatusOK, items)
}

// queryInt читает целочисленный query-параметр, возвращая def при отсутствии или некорректном значении.
func queryInt(r *http.Request, key string, def int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}

	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return def
	}

	return v
}
