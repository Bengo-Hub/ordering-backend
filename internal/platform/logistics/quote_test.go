package logistics

import (
	"testing"
	"time"
)

func TestQuoteTTL(t *testing.T) {
	cases := map[int]time.Duration{
		0:    0,
		-5:   0,
		3:    10 * time.Second,
		300:  5 * time.Minute,
		9999: 30 * time.Minute,
	}
	for in, want := range cases {
		if got := quoteTTL(in); got != want {
			t.Errorf("quoteTTL(%d) = %v, want %v", in, got, want)
		}
	}
}
