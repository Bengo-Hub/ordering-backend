package ordering

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPlanTransition_DeliveryDropOffFinalizesAndSettlesCOD(t *testing.T) {
	o := &Order{FulfillmentType: FulfillmentTypeDelivery, Status: OrderStatusOutForDelivery,
		PaymentMethod: PaymentMethodCOD, PaymentStatus: "cod_pending", GrandTotal: 1100}
	fx := planTransition(o, OrderStatusDelivered)
	if !fx.finalize || !fx.markPaid || !fx.settleCOD {
		t.Fatalf("COD drop-off must finalize, mark paid and settle, got %+v", fx)
	}
	if fx.refund || fx.release {
		t.Fatalf("a delivery is not a cancellation, got %+v", fx)
	}
}

func TestPlanTransition_PickupPaidAtTerminalIsNotSettledAgain(t *testing.T) {
	o := &Order{FulfillmentType: FulfillmentTypePickup, Status: OrderStatusReady,
		PaymentMethod: PaymentMethodCOD, PaymentStatus: "cod_pending",
		Metadata: map[string]interface{}{metaPaidAtPOS: true}}
	fx := planTransition(o, OrderStatusCompleted)
	if !fx.markPaid || fx.settleCOD {
		t.Fatalf("terminal-paid pickup is marked paid but treasury already has it, got %+v", fx)
	}
	if !fx.finalize {
		t.Fatal("pickup collection must consume stock")
	}
}

func TestPlanTransition_PrepaidOrderNeverSettlesCOD(t *testing.T) {
	o := &Order{Status: OrderStatusOutForDelivery, PaymentMethod: PaymentMethodMpesa, PaymentStatus: PaymentStatusPaid}
	fx := planTransition(o, OrderStatusDelivered)
	if fx.markPaid || fx.settleCOD {
		t.Fatalf("online-paid order has nothing to collect, got %+v", fx)
	}
}

func TestPlanTransition_DeliveredThenCompletedFinalizesOnce(t *testing.T) {
	delivered := time.Now()
	o := &Order{Status: OrderStatusDelivered, DeliveredAt: &delivered, PaymentStatus: PaymentStatusPaid}
	if fx := planTransition(o, OrderStatusCompleted); fx.finalize {
		t.Fatal("an order finalized at delivered must not be finalized again at completed")
	}
}

func TestPlanTransition_CancelRefundsOnlyPrepaidAndReleasesHold(t *testing.T) {
	res := uuid.New()
	prepaid := &Order{Status: OrderStatusConfirmed, PaymentMethod: PaymentMethodMpesa, PaymentStatus: PaymentStatusPaid, GrandTotal: 500, ReservationID: &res}
	if fx := planTransition(prepaid, OrderStatusCancelled); !fx.refund || !fx.release {
		t.Fatalf("prepaid cancel must refund and release, got %+v", fx)
	}
	cod := &Order{Status: OrderStatusConfirmed, PaymentMethod: PaymentMethodCOD, PaymentStatus: "cod_pending", GrandTotal: 500}
	if fx := planTransition(cod, OrderStatusCancelled); fx.refund || fx.release {
		t.Fatalf("COD without a hold: nothing to refund or release, got %+v", fx)
	}
	unpaid := &Order{Status: OrderStatusPending, PaymentMethod: PaymentMethodMpesa, PaymentStatus: PaymentStatusPending, GrandTotal: 500}
	if fx := planTransition(unpaid, OrderStatusCancelled); fx.refund {
		t.Fatal("an unpaid order is never refunded")
	}
	free := &Order{Status: OrderStatusConfirmed, PaymentMethod: PaymentMethodMpesa, PaymentStatus: PaymentStatusPaid, GrandTotal: 0}
	if fx := planTransition(free, OrderStatusCancelled); fx.refund {
		t.Fatal("a zero-total order has nothing to refund")
	}
}

func TestPlanTransition_IntermediateStatusesHaveNoEffects(t *testing.T) {
	o := &Order{Status: OrderStatusConfirmed, PaymentMethod: PaymentMethodCOD, PaymentStatus: "cod_pending"}
	for _, s := range []OrderStatus{OrderStatusPreparing, OrderStatusReady, OrderStatusOutForDelivery} {
		if fx := planTransition(o, s); fx != (transitionEffects{}) {
			t.Fatalf("%s should owe nothing, got %+v", s, fx)
		}
	}
	if fx := planTransition(nil, OrderStatusDelivered); fx != (transitionEffects{}) {
		t.Fatal("nil order owes nothing")
	}
}

func TestApplyTransition_StampsTimeAndPaidFlag(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	o := &Order{Status: OrderStatusReady, PaymentMethod: PaymentMethodCOD, PaymentStatus: "cod_pending"}
	applyTransition(o, OrderStatusCompleted, transitionEffects{markPaid: true}, now)
	if o.Status != OrderStatusCompleted || o.CompletedAt == nil || !o.CompletedAt.Equal(now) {
		t.Fatalf("completed not stamped: %+v", o)
	}
	if o.PaymentStatus != PaymentStatusPaid {
		t.Fatal("pay-on-collection must be paid once collected")
	}
	c := &Order{Status: OrderStatusPending}
	applyTransition(c, OrderStatusCancelled, transitionEffects{}, now)
	if c.CancelledAt == nil {
		t.Fatal("cancelled_at not stamped")
	}
}

func TestCustomerMayCancel_OnlyBeforeTheKitchenStarts(t *testing.T) {
	allowed := map[OrderStatus]bool{
		OrderStatusPending: true, OrderStatusConfirmed: true,
		OrderStatusPreparing: false, OrderStatusReady: false, OrderStatusOutForDelivery: false,
		OrderStatusDelivered: false, OrderStatusCompleted: false, OrderStatusCancelled: false,
	}
	for s, want := range allowed {
		if got := customerMayCancel(s); got != want {
			t.Errorf("customerMayCancel(%s) = %v, want %v", s, got, want)
		}
	}
}

func TestOrderDeletable_LiveOrdersMustBeRejectedFirst(t *testing.T) {
	cases := []struct {
		name  string
		order *Order
		want  bool
	}{
		{"cancelled", &Order{Status: OrderStatusCancelled}, true},
		{"delivered", &Order{Status: OrderStatusDelivered}, true},
		{"timed out", &Order{Status: OrderStatusPaymentTimeout}, true},
		{"unpaid never offered", &Order{Status: OrderStatusPending}, true},
		{"offered to the outlet", &Order{Status: OrderStatusPending, Metadata: map[string]interface{}{metaOutletOfferedAt: "2026-10-06T10:00:00Z"}}, false},
		{"handed to the kitchen", &Order{Status: OrderStatusPending, Metadata: map[string]interface{}{metaOutletHandoffAt: "2026-10-06T10:00:00Z"}}, false},
		{"preparing", &Order{Status: OrderStatusPreparing}, false},
		{"out for delivery", &Order{Status: OrderStatusOutForDelivery}, false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		if got := orderDeletable(c.order); got != c.want {
			t.Errorf("%s: orderDeletable = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBuildAnalyticsSummary_RevenueExcludesCancelledAndFillsEveryDay(t *testing.T) {
	from := time.Date(2026, 10, 1, 15, 30, 0, 0, time.UTC)
	to := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC) // earlier time of day than from
	byStatus := []statusCurrencyRow{
		{Status: "completed", Currency: "KES", Orders: 3, Revenue: 900},
		{Status: "cancelled", Currency: "KES", Orders: 2, Revenue: 0},
		{Status: "pending", Currency: "KES", Orders: 1, Revenue: 0},
		{Status: "delivered", Currency: "USD", Orders: 1, Revenue: 10},
	}
	daily := []dailyRow{{Date: "2026-10-01", Orders: 4, Revenue: 600}, {Date: "2026-10-03", Orders: 3, Revenue: 310}}
	s := buildAnalyticsSummary(from, to, byStatus, daily, nil)

	if s.TotalOrders != 7 || s.CancelledOrders != 2 {
		t.Fatalf("counts wrong: total %d cancelled %d", s.TotalOrders, s.CancelledOrders)
	}
	if s.TotalRevenue != 910 || s.RevenueByCurrency["KES"] != 900 || s.RevenueByCurrency["USD"] != 10 {
		t.Fatalf("revenue wrong: %v %v", s.TotalRevenue, s.RevenueByCurrency)
	}
	if s.OrdersByStatus["pending"] != 1 {
		t.Fatal("status breakdown lost a status")
	}
	if len(s.Trend) != 3 || s.Trend[0].Date != "2026-10-01" || s.Trend[2].Date != "2026-10-03" {
		t.Fatalf("trend must cover every day of the range, got %+v", s.Trend)
	}
	if s.Trend[1].Orders != 0 || s.Trend[2].Revenue != 310 {
		t.Fatalf("trend values wrong: %+v", s.Trend)
	}
	if s.TopSellingItems == nil {
		t.Fatal("top items must be an empty list, not null")
	}
}
