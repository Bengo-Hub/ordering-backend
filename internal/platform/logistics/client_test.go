package logistics

import "testing"

func TestMatchTaskByRef_ExactOrPrefixedOnly(t *testing.T) {
	id := "6f1c2a7e-0000-4000-8000-000000000001"
	tasks := []TaskResponse{
		{ExternalReference: "order:6f1c2a7e-0000-4000-8000-000000000001-x"}, // substring hit, not this order
		{ExternalReference: "order:" + id},
	}
	got := MatchTaskByRef(tasks, id)
	if got == nil || got.ExternalReference != "order:"+id {
		t.Fatalf("want the order:<id> task, got %+v", got)
	}
	if MatchTaskByRef([]TaskResponse{{ExternalReference: id}}, id) == nil {
		t.Fatal("a bare id reference must match")
	}
	if MatchTaskByRef(tasks[:1], id) != nil {
		t.Fatal("a longer reference containing the id must not match")
	}
	if MatchTaskByRef(nil, id) != nil {
		t.Fatal("no tasks, no match")
	}
}
