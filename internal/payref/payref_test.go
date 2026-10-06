package payref

import (
	"testing"

	"github.com/google/uuid"
)

func TestBuildAttempt(t *testing.T) {
	tenant, order := uuid.New(), uuid.New()
	base := Build("ORD", "", tenant, order)
	if got := BuildAttempt("ORD", "", tenant, order, 1); got != base {
		t.Fatalf("first attempt must keep the canonical reference: %s vs %s", got, base)
	}
	if got := BuildAttempt("ORD", "", tenant, order, 0); got != base {
		t.Fatalf("attempt 0: %s", got)
	}
	if got := BuildAttempt("ORD", "", tenant, order, 3); got != base+"-R3" {
		t.Fatalf("retry reference: %s", got)
	}
}
