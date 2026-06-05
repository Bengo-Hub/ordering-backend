package googlebusiness

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Google Business Profile API hosts.
//   - Account Management:  https://developers.google.com/my-business/reference/accountmanagement/rest
//   - Business Information: https://developers.google.com/my-business/reference/businessinformation/rest
//   - Reviews (v4 legacy):  https://developers.google.com/my-business/reference/rest/v4/accounts.locations.reviews
const (
	accountMgmtBase  = "https://mybusinessaccountmanagement.googleapis.com/v1"
	businessInfoBase = "https://mybusinessbusinessinformation.googleapis.com/v1"
	// reviews + reply still live under the v4 My Business host.
	myBusinessV4Base = "https://mybusiness.googleapis.com/v4"
)

// Client is a thin GBP API client bound to a single, already-valid access token.
// Token refresh/persistence is handled by Service before constructing the client.
type Client struct {
	accessToken string
	http        *http.Client
}

// NewClient builds a GBP client for the given (decrypted, valid) access token.
func NewClient(accessToken string) *Client {
	return &Client{
		accessToken: accessToken,
		http:        &http.Client{Timeout: 30 * time.Second},
	}
}

// Account is a Google Business Profile account (accounts/{id}).
type Account struct {
	Name        string `json:"name"`
	AccountName string `json:"accountName"`
	Type        string `json:"type"`
}

// Location is a Google Business Profile location with its Maps placeId metadata.
type Location struct {
	Name      string `json:"name"`  // resource name: locations/{id} (relative to account)
	Title     string `json:"title"` // business/location display name
	StoreCode string `json:"storeCode"`
	Metadata  struct {
		PlaceID      string `json:"placeId"`
		MapsURI      string `json:"mapsUri"`
		NewReviewURI string `json:"newReviewUri"`
	} `json:"metadata"`
}

// Review is a single Google review on a location.
type Review struct {
	Name       string `json:"name"` // accounts/*/locations/*/reviews/{reviewId}
	ReviewID   string `json:"reviewId"`
	Comment    string `json:"comment"`
	StarRating string `json:"starRating"`
	CreateTime string `json:"createTime"`
	UpdateTime string `json:"updateTime"`
	Reviewer   struct {
		DisplayName  string `json:"displayName"`
		ProfilePhoto string `json:"profilePhotoUrl"`
	} `json:"reviewer"`
	ReviewReply *struct {
		Comment    string `json:"comment"`
		UpdateTime string `json:"updateTime"`
	} `json:"reviewReply,omitempty"`
}

// doJSON issues an authenticated request and decodes a JSON body into out.
func (c *Client) doJSON(ctx context.Context, method, urlStr string, body any, out any) error {
	var reader *strings.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		reader = strings.NewReader(string(b))
	} else {
		reader = strings.NewReader("")
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		if errBody.Error.Message != "" {
			return fmt.Errorf("google api %d: %s", resp.StatusCode, errBody.Error.Message)
		}
		return fmt.Errorf("google api returned status %d", resp.StatusCode)
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// ListAccounts lists the GBP accounts the authenticated user can manage.
// GET https://mybusinessaccountmanagement.googleapis.com/v1/accounts
func (c *Client) ListAccounts(ctx context.Context) ([]Account, error) {
	var out struct {
		Accounts []Account `json:"accounts"`
	}
	if err := c.doJSON(ctx, http.MethodGet, accountMgmtBase+"/accounts", nil, &out); err != nil {
		return nil, err
	}
	return out.Accounts, nil
}

// ListLocations lists locations under an account, including placeId metadata.
// GET https://mybusinessbusinessinformation.googleapis.com/v1/{parent=accounts/*}/locations
func (c *Client) ListLocations(ctx context.Context, accountName string) ([]Location, error) {
	q := url.Values{}
	// readMask is required by the Business Information API.
	q.Set("readMask", "name,title,storeCode,metadata")
	q.Set("pageSize", "100")
	endpoint := fmt.Sprintf("%s/%s/locations?%s", businessInfoBase, strings.Trim(accountName, "/"), q.Encode())

	var out struct {
		Locations []Location `json:"locations"`
	}
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return nil, err
	}
	return out.Locations, nil
}

// ListAccountsLocations discovers the first account and its locations in one call,
// used during HandleCallback to auto-populate location + placeId where possible.
func (c *Client) ListAccountsLocations(ctx context.Context) (string, []Location, error) {
	accounts, err := c.ListAccounts(ctx)
	if err != nil {
		return "", nil, err
	}
	if len(accounts) == 0 {
		return "", nil, nil
	}
	account := accounts[0].Name
	locations, err := c.ListLocations(ctx, account)
	if err != nil {
		return account, nil, err
	}
	return account, locations, nil
}

// ListReviews lists reviews for a location.
// GET https://mybusiness.googleapis.com/v4/{location=accounts/*/locations/*}/reviews
// location must be the fully-qualified "accounts/{aid}/locations/{lid}" resource name.
func (c *Client) ListReviews(ctx context.Context, location string) ([]Review, error) {
	endpoint := fmt.Sprintf("%s/%s/reviews", myBusinessV4Base, strings.Trim(location, "/"))
	var out struct {
		Reviews []Review `json:"reviews"`
	}
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return nil, err
	}
	return out.Reviews, nil
}

// ReplyToReview posts (or updates) the owner reply on a review.
// PUT https://mybusiness.googleapis.com/v4/{name=accounts/*/locations/*/reviews/*}/reply
// reviewName must be the full review resource name.
func (c *Client) ReplyToReview(ctx context.Context, reviewName, comment string) error {
	endpoint := fmt.Sprintf("%s/%s/reply", myBusinessV4Base, strings.Trim(reviewName, "/"))
	body := map[string]string{"comment": comment}
	return c.doJSON(ctx, http.MethodPut, endpoint, body, nil)
}
