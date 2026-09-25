package posdiscounts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// OutletPaymentDetails is an outlet's own M-Pesa collection numbers (the ones it prints on its
// receipts) and whether it takes manual M-Pesa for online orders.
type OutletPaymentDetails struct {
	MpesaTill         string `json:"mpesa_till,omitempty"`
	MpesaPaybill      string `json:"mpesa_paybill,omitempty"`
	MpesaAccountRef   string `json:"mpesa_account_reference,omitempty"`
	MpesaPochi        string `json:"mpesa_pochi,omitempty"`
	ManualMpesaOnline bool   `json:"manual_mpesa_online"`
}

// OutletPaymentDetails fetches the outlet's M-Pesa details from pos-api (the owner of outlet
// payment settings) over the same S2S key as the discounts calls. Best-effort: nil on any failure,
// so checkout simply does not offer manual M-Pesa.
func (c *Client) OutletPaymentDetails(ctx context.Context, tenantID, outletID uuid.UUID) *OutletPaymentDetails {
	if !c.Enabled() || outletID == uuid.Nil {
		return nil
	}
	u := fmt.Sprintf("%s/api/v1/s2s/%s/outlets/%s/payment-details", c.baseURL, tenantID.String(), outletID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("X-API-Key", c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Warn("pos outlet payment details: request failed", zap.Error(err))
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.log.Warn("pos outlet payment details: unexpected status", zap.Int("status", resp.StatusCode))
		return nil
	}
	var out OutletPaymentDetails
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil
	}
	return &out
}
