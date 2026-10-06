package payments

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDecidePaymentPoll(t *testing.T) {
	deadline := time.Date(2026, 10, 6, 12, 30, 0, 0, time.UTC)
	before := deadline.Add(-10 * time.Minute)
	after := deadline.Add(time.Minute)
	cur, old := uuid.New(), uuid.New()
	check := func(id uuid.UUID, status string) intentCheck { return intentCheck{ID: id, Status: status} }

	t.Run("failed attempt inside the window is recorded, order stays open", func(t *testing.T) {
		d := decidePaymentPoll(before, deadline, []intentCheck{{ID: cur, Status: "failed", Reason: "Request cancelled by user"}}, "")
		if d.Expire || d.Confirm != nil || d.RecordFailure == nil || d.RecordFailure.ID != cur {
			t.Fatalf("unexpected %+v", d)
		}
	})

	t.Run("cancelled and expired attempts count as failures too", func(t *testing.T) {
		for _, s := range []string{"cancelled", "expired"} {
			if d := decidePaymentPoll(before, deadline, []intentCheck{check(cur, s)}, ""); d.RecordFailure == nil || d.Expire {
				t.Fatalf("%s: %+v", s, d)
			}
		}
	})

	t.Run("window closed after a failure cancels with the reason", func(t *testing.T) {
		d := decidePaymentPoll(after, deadline, []intentCheck{{ID: cur, Status: "failed", Reason: "insufficient funds"}}, "")
		if !d.Expire || !strings.Contains(d.Reason, "insufficient funds") {
			t.Fatalf("unexpected %+v", d)
		}
	})

	t.Run("pending past the window cancels", func(t *testing.T) {
		if d := decidePaymentPoll(after, deadline, []intentCheck{check(cur, "pending")}, ""); !d.Expire {
			t.Fatalf("unexpected %+v", d)
		}
	})

	t.Run("pending inside the window waits", func(t *testing.T) {
		if d := decidePaymentPoll(before, deadline, []intentCheck{check(cur, "pending")}, ""); d.Expire || d.RecordFailure != nil {
			t.Fatalf("unexpected %+v", d)
		}
	})

	t.Run("no intent at all expires only after the window", func(t *testing.T) {
		if d := decidePaymentPoll(before, deadline, nil, ""); d.Expire {
			t.Fatal("expired inside the window")
		}
		d := decidePaymentPoll(after, deadline, nil, "timeout")
		if !d.Expire || !strings.Contains(d.Reason, "timeout") {
			t.Fatalf("unexpected %+v", d)
		}
	})

	t.Run("success on an earlier attempt confirms even after the window", func(t *testing.T) {
		d := decidePaymentPoll(after, deadline, []intentCheck{check(cur, "failed"), check(old, "succeeded")}, "")
		if d.Confirm == nil || d.Confirm.ID != old || d.Expire || d.RecordFailure != nil {
			t.Fatalf("unexpected %+v", d)
		}
	})

	t.Run("a status error never cancels", func(t *testing.T) {
		d := decidePaymentPoll(after, deadline, []intentCheck{{ID: cur, Err: errors.New("treasury down")}}, "")
		if d.Expire {
			t.Fatal("cancelled on a status check error")
		}
		d = decidePaymentPoll(after, deadline, []intentCheck{check(cur, "failed"), {ID: old, Err: errors.New("timeout")}}, "")
		if d.Expire {
			t.Fatal("cancelled while an earlier attempt could not be checked")
		}
		if d.RecordFailure == nil {
			t.Fatal("current failure should still be recorded")
		}
	})

	t.Run("a prompt still processing gets a grace period", func(t *testing.T) {
		if d := decidePaymentPoll(after, deadline, []intentCheck{check(cur, "processing")}, ""); d.Expire {
			t.Fatal("cancelled while the customer may be entering their PIN")
		}
		late := deadline.Add(processingGrace + time.Minute)
		if d := decidePaymentPoll(late, deadline, []intentCheck{check(cur, "processing")}, ""); !d.Expire {
			t.Fatal("stuck prompt kept the order open forever")
		}
	})
}

func TestPaidIntentID(t *testing.T) {
	id := uuid.New()
	if got := paidIntentID(sharedEventEnvelope{AggregateID: id.String()}); got != id {
		t.Fatalf("aggregate id: %v", got)
	}
	if got := paidIntentID(sharedEventEnvelope{Payload: map[string]interface{}{"intent_id": id.String()}}); got != id {
		t.Fatalf("payload id: %v", got)
	}
	if got := paidIntentID(sharedEventEnvelope{AggregateID: "x"}); got != uuid.Nil {
		t.Fatalf("junk should be nil: %v", got)
	}
}
