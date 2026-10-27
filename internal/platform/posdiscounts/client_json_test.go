package posdiscounts

import (
	"encoding/json"
	"strings"
	"testing"
)

const posPromotion = `{"id":"p1","name":"Flash","start_at":"2026-10-27T08:00:00Z","end_at":"2026-10-27T20:00:00Z",
"tenant_id":"t1","metadata":{"banner":{"show_on_storefront":true,"is_flash_sale":true,"banner_title":"Up to 30% off"}},
"rule":{"scope_type":"item","scope_ids":["i1"],"discount_type":"percentage","discount_value":30,"max_discount":0}}`

// The storefront reads is_flash_sale from ordering's JSON; it used to be dropped (json:"-").
func TestDiscountSerializesFlashSale(t *testing.T) {
	var d Discount
	if err := json.Unmarshal([]byte(posPromotion), &d); err != nil {
		t.Fatal(err)
	}
	if !d.IsFlashSale {
		t.Fatal("metadata.banner.is_flash_sale not read")
	}
	out, _ := json.Marshal(d)
	if !strings.Contains(string(out), `"is_flash_sale":true`) {
		t.Fatalf("is_flash_sale not serialized: %s", out)
	}
}

func TestStorefrontPromotionKeepsTenantAndBanner(t *testing.T) {
	var p StorefrontPromotion
	if err := json.Unmarshal([]byte(posPromotion), &p); err != nil {
		t.Fatal(err)
	}
	if p.TenantID != "t1" || !p.IsFlashSale || !p.Banner.ShowOnStorefront || p.Banner.Title != "Up to 30% off" {
		t.Fatalf("decoded %+v", p)
	}
	if p.Rule == nil || p.Rule.DiscountValue != 30 {
		t.Fatalf("rule lost: %+v", p.Rule)
	}
	out, _ := json.Marshal(p)
	for _, want := range []string{`"tenant_id":"t1"`, `"is_flash_sale":true`, `"banner_title":"Up to 30% off"`, `"discount_value":30`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("%s missing from %s", want, out)
		}
	}
}
