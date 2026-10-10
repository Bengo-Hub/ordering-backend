package sla

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSummarizeSLA(t *testing.T) {
	from, to := time.Now().AddDate(0, 0, -7), time.Now()
	rows := []slaStatRow{
		{MetricType: "order_to_delivery", Status: "met", Count: 3, SumSecs: 3000, WithSecs: 3},
		{MetricType: "order_to_delivery", Status: "breached", Count: 1, SumSecs: 2400, WithSecs: 1, SumBreach: 50, WithBreach: 1},
		// Rows without measured seconds must not drag the average down.
		{MetricType: "pickup_to_delivery", Status: "met", Count: 2, SumSecs: 600, WithSecs: 1},
	}
	s := summarizeSLA(uuid.New(), from, to, rows)
	if s.TotalMetrics != 6 || s.MetMetrics != 5 || s.BreachedMetrics != 1 {
		t.Fatalf("totals wrong: %+v", s)
	}
	if got := s.ComplianceRate; got < 83.3 || got > 83.4 {
		t.Fatalf("compliance = %v", got)
	}
	if s.AverageBreachPct != 50 {
		t.Fatalf("breach avg = %v", s.AverageBreachPct)
	}
	o := s.ByType["order_to_delivery"]
	if o.Total != 4 || o.Met != 3 || o.Breached != 1 || o.AverageSeconds != 1350 || o.ComplianceRate != 75 {
		t.Fatalf("order_to_delivery = %+v", o)
	}
	if p := s.ByType["pickup_to_delivery"]; p.AverageSeconds != 600 {
		t.Fatalf("average must ignore rows without seconds: %+v", p)
	}
	if e := summarizeSLA(uuid.New(), from, to, nil); e.TotalMetrics != 0 || e.ComplianceRate != 0 {
		t.Fatal("empty summary must be zero")
	}

	applyPercentiles(s, []slaPercentileRow{
		{MetricType: "order_to_delivery", P50: 1200.4, P90: 2380.9},
		{MetricType: "unknown_type", P50: 1, P90: 2}, // no summary row: ignored
	})
	if o := s.ByType["order_to_delivery"]; o.P50Seconds != 1200 || o.P90Seconds != 2380 || o.Total != 4 {
		t.Fatalf("percentiles = %+v", o)
	}
	if _, ok := s.ByType["unknown_type"]; ok {
		t.Fatal("percentiles must not invent metric types")
	}
}
