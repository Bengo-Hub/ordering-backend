package orderinghandler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	entorder "github.com/bengobox/ordering-backend/internal/ent/order"
	enttenant "github.com/bengobox/ordering-backend/internal/ent/tenant"
)

// CRMLinkStore is what the link check reads. LocalTenant finds the local tenant row by UUID or
// slug, and ContactReferenced answers whether any order, in any tenant, carries the CRM contact id.
type CRMLinkStore interface {
	// LocalTenant returns the id of the local tenant row matching a UUID or slug, found=false when
	// there is none. A UUID without a local row (a tenant held under another id after a sync
	// drift) is not found, so the check answers 404 instead of a false "not referenced".
	LocalTenant(ctx context.Context, uuidOrSlug string) (id uuid.UUID, found bool, err error)
	// ContactReferenced reports whether any row, in any tenant, still links the CRM contact. CRM
	// contact ids are random UUIDs minted by marketflow, so a match anywhere is this person, and a
	// tenant mismatch can only answer "referenced", which keeps the contact.
	ContactReferenced(ctx context.Context, contactID uuid.UUID) (bool, error)
}

// EntCRMLinkStore reads the links from the database. Orders are the only table here that
// carries crm_contact_id.
type EntCRMLinkStore struct{ db *ent.Client }

// NewEntCRMLinkStore builds the database-backed store.
func NewEntCRMLinkStore(db *ent.Client) *EntCRMLinkStore { return &EntCRMLinkStore{db: db} }

// LocalTenant implements CRMLinkStore.
func (s *EntCRMLinkStore) LocalTenant(ctx context.Context, uuidOrSlug string) (uuid.UUID, bool, error) {
	pred := enttenant.Slug(uuidOrSlug)
	if id, err := uuid.Parse(uuidOrSlug); err == nil {
		pred = enttenant.ID(id)
	}
	t, err := s.db.Tenant.Query().Where(pred).Only(ctx)
	if ent.IsNotFound(err) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return t.ID, true, nil
}

// ContactReferenced implements CRMLinkStore.
func (s *EntCRMLinkStore) ContactReferenced(ctx context.Context, contactID uuid.UUID) (bool, error) {
	return s.db.Order.Query().Where(entorder.CrmContactID(contactID)).Exist(ctx)
}

// CRMLinksHandler tells marketflow whether ordering still links a CRM contact, so marketflow can
// decide between deleting an erased person's contact and stripping it to a minimal record.
type CRMLinksHandler struct {
	store CRMLinkStore
	log   *zap.Logger
}

// NewCRMLinksHandler builds the handler.
func NewCRMLinksHandler(store CRMLinkStore, log *zap.Logger) *CRMLinksHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &CRMLinksHandler{store: store, log: log}
}

func writeCRMLinksJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// Referenced answers whether any order, in any tenant, carries the contact id. {tenant} is the
// tenant UUID or slug and must be known here. The answer is the flag only: no order numbers or ids.
// GET /api/v1/s2s/{tenant}/crm-links/{contactID} (X-API-Key) -> {"referenced": bool}
func (h *CRMLinksHandler) Referenced(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	contactID, err := uuid.Parse(chi.URLParam(r, "contactID"))
	if err != nil || contactID == uuid.Nil {
		writeCRMLinksJSON(w, http.StatusBadRequest, map[string]string{"error": "contact id must be a UUID", "code": "invalid_contact_id"})
		return
	}
	tenantID, known, err := h.store.LocalTenant(ctx, chi.URLParam(r, "tenant"))
	if err != nil {
		h.log.Error("crm links: tenant lookup failed", zap.Error(err))
		writeCRMLinksJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check links", "code": "internal_error"})
		return
	}
	if !known {
		writeCRMLinksJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found", "code": "tenant_not_found"})
		return
	}
	found, err := h.store.ContactReferenced(ctx, contactID)
	if err != nil {
		// Ids only in the log, never the person's details.
		h.log.Error("crm links check failed", zap.String("tenant_id", tenantID.String()),
			zap.String("contact_id", contactID.String()), zap.Error(err))
		writeCRMLinksJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check links", "code": "internal_error"})
		return
	}
	writeCRMLinksJSON(w, http.StatusOK, map[string]bool{"referenced": found})
}
