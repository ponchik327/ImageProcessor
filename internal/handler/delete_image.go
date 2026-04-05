package handler

import (
	"errors"
	"net/http"

	"github.com/ponchik327/ImageProcessor/internal/domain"
)

// deleteImage обрабатывает DELETE /image/{id}.
func (h *Handler) deleteImage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid image id")

		return
	}

	if err = h.svc.DeleteImage(r.Context(), id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "image not found")

			return
		}

		h.log.Error("delete_image: service error", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")

		return
	}

	w.WriteHeader(http.StatusNoContent)
}
