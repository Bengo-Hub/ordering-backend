package handlers

import (
	"net/http"
	"path"

	"github.com/Bengo-Hub/httpware/objectstore"
	authclient "github.com/Bengo-Hub/shared-auth-client"
	"go.uber.org/zap"
)

// MenuImageMaxBytes is the menu picture limit the ordering UI promises (5 MB); registered over the
// platform default at startup by RegisterMediaTypes.
const MenuImageMaxBytes = 5 << 20

// RegisterMediaTypes sets ordering's media rules on the shared registry. Call once at startup.
func RegisterMediaTypes() {
	objectstore.Register(objectstore.MediaType{
		Name: objectstore.TypeMenuImages, Public: true, MaxBytes: MenuImageMaxBytes,
		Allowed: []string{"image/jpeg", "image/png"}, Image: true,
	})
}

// mediaUploadTypes are the types the upload accepts (query "type"): menu pictures (default) and
// other public pictures (outlets, banners).
var mediaUploadTypes = map[string]bool{
	objectstore.TypeMenuImages: true,
	objectstore.TypeImages:     true,
}

// MediaHandler stores images on the shared object storage.
type MediaHandler struct {
	log   *zap.Logger
	store *objectstore.Store
}

// NewMediaHandler creates the upload handler.
func NewMediaHandler(log *zap.Logger, store *objectstore.Store) *MediaHandler {
	return &MediaHandler{log: log.Named("media"), store: store}
}

// Upload stores one image for the caller's tenant: JPEG or PNG, checked from the bytes and
// re-encoded (EXIF and anything appended removed). It answers the image's link ("url", what a form
// saves) and its key; saving either stores the key. Requires a signed-in user of a tenant.
//
// @Summary Upload an image
// @Tags Media
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "JPEG or PNG, up to 5 MB"
// @Param type query string false "menu-images (default) or images"
// @Success 201 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /api/v1/media/upload [post]
func (h *MediaHandler) Upload(w http.ResponseWriter, r *http.Request) {
	claims, ok := authclient.ClaimsFromContext(r.Context())
	if !ok || claims.TenantID == "" {
		RespondError(w, http.StatusUnauthorized, "Sign in to an organisation to upload")
		return
	}
	if !h.store.Enabled() {
		RespondError(w, http.StatusServiceUnavailable, "File storage is not configured")
		return
	}
	// The type decides the limits, so it is read before the body.
	mediaType := r.URL.Query().Get("type")
	if mediaType == "" {
		mediaType = objectstore.TypeMenuImages
	}
	if !mediaUploadTypes[mediaType] {
		RespondError(w, http.StatusBadRequest, "type must be menu-images or images")
		return
	}
	up, err := objectstore.ReadUpload(w, r, "file", mediaType)
	if err != nil {
		if objectstore.IsUploadError(err) {
			RespondError(w, http.StatusBadRequest, err.Error())
			return
		}
		RespondError(w, http.StatusInternalServerError, "Failed to read the upload")
		return
	}
	key, err := h.store.NewKey(objectstore.Owner{TenantID: claims.TenantID}, mediaType, up.Ext)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid organisation")
		return
	}
	if err := h.store.Put(r.Context(), key, up.Data, up.ContentType); err != nil {
		h.log.Error("upload not stored", zap.Error(err))
		RespondError(w, http.StatusBadGateway, "File storage is unavailable")
		return
	}
	RespondJSON(w, http.StatusCreated, map[string]string{
		"url":      h.store.URL(key),
		"key":      key,
		"filename": path.Base(key),
	})
}
