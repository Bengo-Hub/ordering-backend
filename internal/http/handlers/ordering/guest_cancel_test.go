package orderinghandler

import (
	"testing"

	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/modules/ordering"
)

func TestGuestSessionOwns(t *testing.T) {
	customer := uuid.New()
	guest := &ordering.Order{Metadata: map[string]interface{}{"sessionId": "s-1"}}
	cases := []struct {
		name    string
		order   *ordering.Order
		session string
		want    bool
	}{
		{"matching session", guest, "s-1", true},
		{"other session", guest, "s-2", false},
		{"no session given", guest, "", false},
		{"signed-in customer's order", &ordering.Order{CustomerID: &customer, Metadata: map[string]interface{}{"sessionId": "s-1"}}, "s-1", false},
		{"order without a session", &ordering.Order{Metadata: map[string]interface{}{}}, "s-1", false},
		{"nil order", nil, "s-1", false},
	}
	for _, c := range cases {
		if got := guestSessionOwns(c.order, c.session); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
