package handler

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ponchik327/ImageProcessor/internal/domain"
)

// getFile обрабатывает GET /image/{id}/file?variant=<type>.
// Передаёт бинарный файл изображения клиенту для использования в <img src="...">.
func (h *Handler) getFile(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid image id")

		return
	}

	variantType := r.URL.Query().Get("variant")
	if variantType == "" {
		writeError(w, http.StatusBadRequest, "missing variant query param")

		return
	}

	img, variants, err := h.svc.GetImageInfo(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "image not found")

			return
		}

		h.log.Error("get_file: service error", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")

		return
	}

	if img.Status != domain.StatusDone {
		writeError(w, http.StatusConflict, "image processing not complete")

		return
	}

	for _, v := range variants {
		if v.VariantType == variantType {
			f, err := os.Open(v.FilePath)
			if err != nil {
				h.log.Error("get_file: open variant file", "path", v.FilePath, "error", err)
				writeError(w, http.StatusInternalServerError, "internal error")

				return
			}
			defer func() {
				if closeErr := f.Close(); closeErr != nil {
					h.log.Warn("get_file: close variant file", "path", v.FilePath, "error", closeErr)
				}
			}()

			stat, err := f.Stat()
			if err != nil {
				h.log.Error("get_file: stat variant file", "path", v.FilePath, "error", err)
				writeError(w, http.StatusInternalServerError, "internal error")

				return
			}

			name := filepath.Base(v.FilePath)
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
			http.ServeContent(w, r, name, stat.ModTime(), f)

			return
		}
	}

	writeError(w, http.StatusNotFound, "variant not found")
}
