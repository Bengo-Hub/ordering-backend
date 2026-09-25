package ordering

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// mergeOnlyRepo records MergeOrderMetadata calls; every other method is unused here.
type mergeOnlyRepo struct {
	Repository
	patches []map[string]interface{}
}

func (r *mergeOnlyRepo) MergeOrderMetadata(_ context.Context, _, _ uuid.UUID, patch map[string]interface{}) error {
	r.patches = append(r.patches, patch)
	return nil
}

// A stamp must survive a later full save of the same in-memory order: checkout saves the whole
// order again (CRM link), and a database-only stamp was overwritten, so outlet_offered_at vanished.
func TestStampOrderMetadata_UpdatesInMemoryOrder(t *testing.T) {
	repo := &mergeOnlyRepo{}
	svc := &OrderService{repo: repo}
	order := &Order{ID: uuid.New(), TenantID: uuid.New(), Metadata: map[string]interface{}{"guest": true}}

	if err := svc.stampOrderMetadata(context.Background(), order, map[string]interface{}{metaOutletOfferedAt: "2026-09-26T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if order.Metadata[metaOutletOfferedAt] != "2026-09-26T00:00:00Z" || order.Metadata["guest"] != true {
		t.Fatalf("in-memory metadata not updated: %#v", order.Metadata)
	}
	if len(repo.patches) != 1 {
		t.Fatalf("stored metadata not updated")
	}
}
