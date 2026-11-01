package config

import "testing"

func TestDefaultShortNameUsesDisplayName(t *testing.T) {
	resp := PublicConfigResponse{Name: "Codevertex Demo"}
	defaultShortName(&resp)
	if resp.ShortName != "Codevertex Demo" {
		t.Fatalf("short_name = %q, want the display name", resp.ShortName)
	}

	resp = PublicConfigResponse{Name: "Codevertex Demo", ShortName: "Demo"}
	defaultShortName(&resp)
	if resp.ShortName != "Demo" {
		t.Fatalf("short_name = %q, a set short name must be kept", resp.ShortName)
	}
}
