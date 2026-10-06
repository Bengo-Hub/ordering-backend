package ordering

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestParsePaymentRetryWindow(t *testing.T) {
	cases := []struct {
		raw  string
		ok   bool
		want time.Duration
	}{
		{"", false, DefaultPaymentRetryWindow},
		{"45", true, 45 * time.Minute},
		{`"60"`, true, time.Hour},
		{"abc", true, DefaultPaymentRetryWindow},
		{"0", true, DefaultPaymentRetryWindow},
		{"-5", true, DefaultPaymentRetryWindow},
		{"1", true, 5 * time.Minute},
		{"100000", true, 24 * time.Hour},
	}
	for _, c := range cases {
		if got := ParsePaymentRetryWindow(c.raw, c.ok); got != c.want {
			t.Errorf("%q ok=%v: got %v want %v", c.raw, c.ok, got, c.want)
		}
	}
}

func TestPaymentRetryDeadline(t *testing.T) {
	placed := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	created := placed.Add(-time.Minute)

	if got := PaymentRetryDeadline(&placed, created, nil); !got.Equal(placed.Add(DefaultPaymentRetryWindow)) {
		t.Fatalf("default from placed_at: %v", got)
	}
	if got := PaymentRetryDeadline(nil, created, nil); !got.Equal(created.Add(DefaultPaymentRetryWindow)) {
		t.Fatalf("default from created_at: %v", got)
	}
	stamped := map[string]interface{}{metaPaymentRetryUntil: "2026-10-06T12:45:00Z"}
	if got := PaymentRetryDeadline(&placed, created, stamped); !got.Equal(placed.Add(45 * time.Minute)) {
		t.Fatalf("stamped deadline ignored: %v", got)
	}
	bad := map[string]interface{}{metaPaymentRetryUntil: "soon"}
	if got := PaymentRetryDeadline(&placed, created, bad); !got.Equal(placed.Add(DefaultPaymentRetryWindow)) {
		t.Fatalf("unparsable stamp should fall back: %v", got)
	}
}

func TestPaymentRetryInfoFor(t *testing.T) {
	placed := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	online := func() *Order {
		return &Order{
			Status: OrderStatusPending, PaymentStatus: PaymentStatusPending, PaymentMethod: PaymentMethodMpesa,
			GrandTotal: 900, PlacedAt: &placed,
			Metadata: map[string]interface{}{
				metaPaymentAttempts: float64(2), metaPaymentLastFailure: "Request cancelled by user",
				metaPaymentLastAttemptAt: "2026-10-06T12:05:00Z", metaPaymentRetries: float64(1),
			},
		}
	}

	info := PaymentRetryInfoFor(online(), placed.Add(10*time.Minute))
	if info == nil || !info.Open || info.Attempts != 2 || info.Retries != 1 || info.LastFailureReason != "Request cancelled by user" {
		t.Fatalf("unexpected info %+v", info)
	}
	if info.LastAttemptAt == nil {
		t.Fatal("last attempt time missing")
	}
	if closed := PaymentRetryInfoFor(online(), placed.Add(31*time.Minute)); closed == nil || closed.Open {
		t.Fatalf("window should be closed: %+v", closed)
	}

	paid := online()
	paid.PaymentStatus = PaymentStatusPaid
	cod := online()
	cod.PaymentMethod = PaymentMethodCOD
	manual := online()
	manual.Metadata[metaPaymentChannel] = PaymentChannelManualMpesa
	accepted := online()
	accepted.Status = OrderStatusConfirmed
	free := online()
	free.GrandTotal = 0
	for name, o := range map[string]*Order{"paid": paid, "cod": cod, "manual mpesa": manual, "accepted": accepted, "free": free} {
		if PaymentRetryInfoFor(o, placed) != nil {
			t.Errorf("%s: should not offer a retry", name)
		}
	}
}

func TestPaymentFailurePatch(t *testing.T) {
	at := time.Date(2026, 10, 6, 12, 3, 0, 0, time.UTC)
	p := PaymentFailurePatch(nil, "i1", "DS timeout", at)
	if p[metaPaymentAttempts] != 1 || p[metaPaymentLastFailure] != "DS timeout" || p[metaPaymentLastFailedIntent] != "i1" {
		t.Fatalf("first failure: %v", p)
	}
	if p[metaPaymentLastAttemptAt] != "2026-10-06T12:03:00Z" {
		t.Fatalf("time: %v", p[metaPaymentLastAttemptAt])
	}

	meta := map[string]interface{}{metaPaymentAttempts: float64(1), metaPaymentLastFailedIntent: "i1"}
	if again := PaymentFailurePatch(meta, "i1", "DS timeout", at); again != nil {
		t.Fatalf("the same failed intent was counted twice: %v", again)
	}
	next := PaymentFailurePatch(meta, "i2", "", at)
	if next[metaPaymentAttempts] != 2 || next[metaPaymentLastFailure] != "payment not completed" {
		t.Fatalf("second failure: %v", next)
	}
	if noIntent := PaymentFailurePatch(meta, "", "timeout", at); noIntent[metaPaymentAttempts] != 2 {
		t.Fatalf("failure without an intent id should still count: %v", noIntent)
	}
}

func TestPaymentRetryClaim(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 10, 0, 0, time.UTC)
	deadline := now.Add(20 * time.Minute)
	open := OrderSnapshot{Status: OrderStatusPending, PaymentStatus: PaymentStatusPending}

	p, err := paymentRetryClaim(open, deadline, now)
	if err != nil || p[metaPaymentRetries] != 1 {
		t.Fatalf("first retry: %v %v", p, err)
	}

	cases := []struct {
		name string
		snap OrderSnapshot
		now  time.Time
		want error
	}{
		{"paid", OrderSnapshot{Status: OrderStatusPending, PaymentStatus: PaymentStatusPaid}, now, ErrPaymentAlreadyCompleted},
		{"cancelled", OrderSnapshot{Status: OrderStatusCancelled, PaymentStatus: PaymentStatusPending}, now, ErrPaymentNotRetryable},
		{"cod", OrderSnapshot{Status: OrderStatusPending, PaymentStatus: "cod_pending"}, now, ErrPaymentNotRetryable},
		{"window closed", open, deadline, ErrPaymentRetryClosed},
		{"limit reached", OrderSnapshot{Status: OrderStatusPending, PaymentStatus: PaymentStatusPending,
			Metadata: map[string]interface{}{metaPaymentRetries: float64(MaxPaymentRetries)}}, now, ErrPaymentRetryLimit},
		{"too soon", OrderSnapshot{Status: OrderStatusPending, PaymentStatus: PaymentStatusPending,
			Metadata: map[string]interface{}{metaPaymentRetryLastAt: now.Add(-5 * time.Second).Format(time.RFC3339Nano)}}, now, ErrPaymentRetryTooSoon},
	}
	for _, c := range cases {
		if _, err := paymentRetryClaim(c.snap, deadline, c.now); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v want %v", c.name, err, c.want)
		}
	}

	spaced := OrderSnapshot{Status: OrderStatusPending, PaymentStatus: PaymentStatusFailed,
		Metadata: map[string]interface{}{metaPaymentRetries: float64(2), metaPaymentRetryLastAt: now.Add(-time.Minute).Format(time.RFC3339Nano)}}
	if p, err := paymentRetryClaim(spaced, deadline, now); err != nil || p[metaPaymentRetries] != 3 {
		t.Fatalf("spaced retry: %v %v", p, err)
	}
}

func TestPaidIntentPatch(t *testing.T) {
	unpaid := OrderSnapshot{Status: OrderStatusPending, PaymentStatus: PaymentStatusPending}
	p := paidIntentPatch(unpaid, "i1")
	if p[metaPaymentPaidIntentID] != "i1" || p[metaPaymentIntentID] != "i1" {
		t.Fatalf("paying intent not recorded: %v", p)
	}
	if paidIntentPatch(unpaid, "") != nil {
		t.Fatal("empty intent should write nothing")
	}

	paid := OrderSnapshot{PaymentStatus: PaymentStatusPaid, Metadata: map[string]interface{}{metaPaymentPaidIntentID: "i1"}}
	if again := paidIntentPatch(paid, "i1"); again != nil {
		t.Fatalf("redelivered success wrote %v", again)
	}
	extra := paidIntentPatch(paid, "i2")
	list, _ := extra[metaPaymentExtraPaidIntents].([]string)
	if len(list) != 1 || list[0] != "i2" {
		t.Fatalf("second payment not listed: %v", extra)
	}
	if _, moved := extra[metaPaymentIntentID]; moved {
		t.Fatal("refund intent moved to the duplicate payment")
	}
	paid.Metadata[metaPaymentExtraPaidIntents] = []interface{}{"i2"}
	if dup := paidIntentPatch(paid, "i2"); dup != nil {
		t.Fatalf("duplicate listed twice: %v", dup)
	}
}

func TestPaymentIntentIDs(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	meta := map[string]interface{}{metaPaymentIntentIDs: []interface{}{a.String(), b.String(), "junk", c.String()}}
	got := PaymentIntentIDs(&c, meta)
	if len(got) != 3 || got[0] != c || got[1] != b || got[2] != a {
		t.Fatalf("want current first then newest earlier attempts, got %v", got)
	}
	if got := PaymentIntentIDs(nil, nil); len(got) != 0 {
		t.Fatalf("no intents: %v", got)
	}
}

func TestIntentRecordPatch(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	p := intentRecordPatch(map[string]interface{}{metaPaymentIntentIDs: []interface{}{a.String()}}, b)
	ids, _ := p[metaPaymentIntentIDs].([]string)
	if len(ids) != 2 || ids[1] != b.String() || p[metaPaymentIntentID] != b.String() {
		t.Fatalf("unexpected %v", p)
	}
	same := intentRecordPatch(map[string]interface{}{metaPaymentIntentIDs: []interface{}{a.String()}}, a)
	if _, grew := same[metaPaymentIntentIDs]; grew {
		t.Fatalf("known intent appended again: %v", same)
	}
}

func TestPayableAmountOf(t *testing.T) {
	if got := payableAmountOf(&Order{GrandTotal: 1000}); got != 1000 {
		t.Fatalf("full: %v", got)
	}
	if got := payableAmountOf(&Order{GrandTotal: 1000, Metadata: map[string]interface{}{"deposit_amount": 300.0}}); got != 300 {
		t.Fatalf("deposit: %v", got)
	}
}
