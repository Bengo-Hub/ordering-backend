package config

import "testing"

func TestApplyServiceBranding(t *testing.T) {
	resp := PublicConfigResponse{Name: "Urban Loft", Tagline: "Cafe"}
	applyServiceBranding(&resp, map[string]any{
		"service_branding": map[string]any{
			"ordering": map[string]any{
				"name":       "Urban Eats",
				"short_name": "Urban Eats",
				"icon_url":   "data:image/svg+xml;base64,PHN2Zy8+",
				"tagline":    "Loft food, delivered",
			},
			"rider": map[string]any{"name": "Loft Riders"},
		},
	})
	if resp.Name != "Urban Loft" {
		t.Fatalf("business name must not change, got %q", resp.Name)
	}
	if resp.AppName != "Urban Eats" || resp.AppShortName != "Urban Eats" || resp.AppIconURL == "" {
		t.Fatalf("ordering branding not applied: %#v", resp)
	}
	if resp.Tagline != "Loft food, delivered" {
		t.Fatalf("app tagline should win, got %q", resp.Tagline)
	}
}

func TestApplyServiceBranding_NoEntry(t *testing.T) {
	resp := PublicConfigResponse{Name: "Alpha China Market"}
	applyServiceBranding(&resp, map[string]any{"service_branding": map[string]any{"pos": map[string]any{"name": "Till"}}})
	applyServiceBranding(&resp, nil)
	if resp.AppName != "" || resp.AppIconURL != "" {
		t.Fatalf("no ordering entry should leave app fields empty: %#v", resp)
	}
}
