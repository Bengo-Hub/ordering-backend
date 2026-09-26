package ordering

import (
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"

	"github.com/bengobox/ordering-backend/internal/ent/order"
)

// The M-Pesa code reuse check must stay a jsonb containment so the GIN index on metadata serves
// it, with the code bound as a parameter rather than spliced into the SQL.
func TestMetadataContainsSQL(t *testing.T) {
	s := sql.Dialect(dialect.Postgres).Select("*").From(sql.Table(order.Table))
	metadataContains(map[string]any{"mpesa_code": "QK12AB34CD"})(s)
	query, args := s.Query()
	if !strings.Contains(query, `"orders"."metadata" @> $1::jsonb`) {
		t.Fatalf("unexpected SQL: %s", query)
	}
	if len(args) != 1 || args[0] != `{"mpesa_code":"QK12AB34CD"}` {
		t.Fatalf("unexpected args: %#v", args)
	}
}
