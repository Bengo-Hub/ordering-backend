package handlers

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
)

type MediaHandler struct {
	log *zap.Logger
	cfg *config.Config
}

func NewMediaHandler(log *zap.Logger, cfg *config.Config) *MediaHandler {
	return &MediaHandler{
		log: log,
		cfg: cfg,
	}
}

// Upload handles file uploads for menu items and other media.
// It validates file size, extension, and saves the file to the media root.
func (h *MediaHandler) Upload(w http.ResponseWriter, r *http.Request) {
	// 1. Limit upload size (10MB as per security config)
	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)
	if err := r.ParseMultipartForm(10 * 1024 * 1024); err != nil {
		h.log.Error("failed to parse multipart form", zap.Error(err))
		RespondError(w, http.StatusBadRequest, "File too large (max 10MB)")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.log.Error("failed to get file from form", zap.Error(err))
		RespondError(w, http.StatusBadRequest, "Invalid file upload")
		return
	}
	defer file.Close()

	// 2. Detect Content Type
	buffer := make([]byte, 512)
	n, _ := file.Read(buffer)
	contentType := http.DetectContentType(buffer[:n])
	file.Seek(0, 0) // Reset to beginning after detection

	allowedTypes := map[string]bool{
		"image/jpeg":    true,
		"image/jpg":     true,
		"image/png":     true,
		"image/webp":    true,
		"image/svg+xml": true,
	}

	if !allowedTypes[contentType] {
		RespondError(w, http.StatusBadRequest, "Invalid file type. Supported: JPEG, JPG, PNG, WEBP, SVG")
		return
	}

	// 3. Generate unique filename
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		// Fallback extensions based on content type
		switch contentType {
		case "image/jpeg":
			ext = ".jpg"
		case "image/png":
			ext = ".png"
		case "image/webp":
			ext = ".webp"
		case "image/svg+xml":
			ext = ".svg"
		}
	}
	filename := fmt.Sprintf("%s_%d%s", uuid.New().String(), time.Now().Unix(), ext)

	// Create subdirectory for uploads
	dir := filepath.Join(h.cfg.Media.Root, "uploads", "menu")
	if err := os.MkdirAll(dir, 0755); err != nil {
		h.log.Error("failed to create upload directory", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	dstPath := filepath.Join(dir, filename)
	dst, err := os.Create(dstPath)
	if err != nil {
		h.log.Error("failed to create destination file", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	defer dst.Close()

	// 4. Sanitize and Compress (Re-encode)
	// This also strips metadata (EXIF) as a security measure
	if contentType == "image/jpeg" || contentType == "image/png" {
		img, _, err := image.Decode(file)
		if err == nil {
			if contentType == "image/jpeg" {
				err = jpeg.Encode(dst, img, &jpeg.Options{Quality: 85})
			} else {
				err = png.Encode(dst, img)
			}
			if err == nil {
				goto response
			}
			// If re-encoding fails, fallback to direct copy but log error
			h.log.Warn("re-encoding failed, falling back to direct copy", zap.Error(err))
			file.Seek(0, 0)
			dst.Seek(0, 0)
			dst.Truncate(0)
		}
	}

	if _, err := io.Copy(dst, file); err != nil {
		h.log.Error("failed to copy file", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

response:

	// 4. Return URL
	// Note: URLBase is defined in config and values.yaml
	url := fmt.Sprintf("%s/media/uploads/menu/%s", h.cfg.Media.URLBase, filename)

	RespondJSON(w, http.StatusCreated, map[string]string{
		"url":      url,
		"filename": filename,
	})
}
