package ordering

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bengobox/ordering-backend/internal/modules/catalog"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// ---------------------------------------------------------------------------
// Mock catalog repository – implements catalog.Repository
// Only GetMenuItem is backed by real data; every other method returns a
// reasonable zero-value or an error so the binary compiles.
// ---------------------------------------------------------------------------

type mockCatalogRepo struct {
	mu    sync.RWMutex
	items map[uuid.UUID]*catalog.MenuItem // keyed by item ID
}

func newMockCatalogRepo() *mockCatalogRepo {
	return &mockCatalogRepo{items: make(map[uuid.UUID]*catalog.MenuItem)}
}

func (r *mockCatalogRepo) seedMenuItem(item *catalog.MenuItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[item.ID] = item
}

func (r *mockCatalogRepo) GetMenuItem(_ context.Context, _, itemID uuid.UUID) (*catalog.MenuItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[itemID]
	if !ok {
		return nil, catalog.ErrMenuItemNotFound
	}
	return item, nil
}

// --- stubs for the remaining catalog.Repository methods ---

func (r *mockCatalogRepo) CreateCategory(_ context.Context, _ *catalog.Category) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) GetCategory(_ context.Context, _, _ uuid.UUID) (*catalog.Category, error) {
	return nil, errors.New("not implemented")
}
func (r *mockCatalogRepo) GetCategoryByName(_ context.Context, _, _ uuid.UUID, _ string) (*catalog.Category, error) {
	return nil, errors.New("not implemented")
}
func (r *mockCatalogRepo) ListCategories(_ context.Context, _ catalog.CategoryFilter) ([]catalog.Category, int, error) {
	return nil, 0, errors.New("not implemented")
}
func (r *mockCatalogRepo) UpdateCategory(_ context.Context, _ *catalog.Category) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) DeleteCategory(_ context.Context, _, _ uuid.UUID) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) CountCategoryItems(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, errors.New("not implemented")
}
func (r *mockCatalogRepo) CountCategoryChildren(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, errors.New("not implemented")
}
func (r *mockCatalogRepo) CreateMenuItem(_ context.Context, _ *catalog.MenuItem) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) GetMenuItemBySKU(_ context.Context, _ uuid.UUID, _ string) (*catalog.MenuItem, error) {
	return nil, errors.New("not implemented")
}
func (r *mockCatalogRepo) ListMenuItems(_ context.Context, _ catalog.MenuItemFilter) ([]catalog.MenuItem, int, error) {
	return nil, 0, errors.New("not implemented")
}
func (r *mockCatalogRepo) UpdateMenuItem(_ context.Context, _ *catalog.MenuItem) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) DeleteMenuItem(_ context.Context, _, _ uuid.UUID) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) CreateVariant(_ context.Context, _ *catalog.Variant) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) GetVariant(_ context.Context, _ uuid.UUID) (*catalog.Variant, error) {
	return nil, errors.New("not implemented")
}
func (r *mockCatalogRepo) ListVariants(_ context.Context, _ uuid.UUID) ([]catalog.Variant, error) {
	return nil, errors.New("not implemented")
}
func (r *mockCatalogRepo) UpdateVariant(_ context.Context, _ *catalog.Variant) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) DeleteVariant(_ context.Context, _ uuid.UUID) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) CreateTranslation(_ context.Context, _ *catalog.Translation) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) GetTranslation(_ context.Context, _ uuid.UUID, _ string) (*catalog.Translation, error) {
	return nil, errors.New("not implemented")
}
func (r *mockCatalogRepo) ListTranslations(_ context.Context, _ uuid.UUID) ([]catalog.Translation, error) {
	return nil, errors.New("not implemented")
}
func (r *mockCatalogRepo) UpdateTranslation(_ context.Context, _ *catalog.Translation) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) DeleteTranslation(_ context.Context, _ uuid.UUID, _ string) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) CreateDietaryTag(_ context.Context, _ *catalog.DietaryTag) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) GetDietaryTag(_ context.Context, _ string) (*catalog.DietaryTag, error) {
	return nil, errors.New("not implemented")
}
func (r *mockCatalogRepo) ListDietaryTags(_ context.Context) ([]catalog.DietaryTag, error) {
	return nil, errors.New("not implemented")
}
func (r *mockCatalogRepo) DeleteDietaryTag(_ context.Context, _ string) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) AddDietaryTagToItem(_ context.Context, _ uuid.UUID, _ string) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) RemoveDietaryTagFromItem(_ context.Context, _ uuid.UUID, _ string) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) ListItemDietaryTags(_ context.Context, _ uuid.UUID) ([]catalog.DietaryTag, error) {
	return nil, errors.New("not implemented")
}
func (r *mockCatalogRepo) CreateAsset(_ context.Context, _ *catalog.Asset) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) GetAsset(_ context.Context, _ uuid.UUID) (*catalog.Asset, error) {
	return nil, errors.New("not implemented")
}
func (r *mockCatalogRepo) ListAssets(_ context.Context, _ uuid.UUID) ([]catalog.Asset, error) {
	return nil, errors.New("not implemented")
}
func (r *mockCatalogRepo) DeleteAsset(_ context.Context, _ uuid.UUID) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) CreateSchedule(_ context.Context, _ *catalog.Schedule) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) GetSchedule(_ context.Context, _ uuid.UUID) (*catalog.Schedule, error) {
	return nil, errors.New("not implemented")
}
func (r *mockCatalogRepo) ListSchedules(_ context.Context, _ uuid.UUID) ([]catalog.Schedule, error) {
	return nil, errors.New("not implemented")
}
func (r *mockCatalogRepo) UpdateSchedule(_ context.Context, _ *catalog.Schedule) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) DeleteSchedule(_ context.Context, _ uuid.UUID) error {
	return errors.New("not implemented")
}
func (r *mockCatalogRepo) GetPublicMenu(_ context.Context, _ catalog.PublicMenuRequest) ([]catalog.PublicMenuItem, int, error) {
	return nil, 0, errors.New("not implemented")
}
func (r *mockCatalogRepo) GetPublicCategories(_ context.Context, _, _ uuid.UUID) ([]catalog.PublicCategory, error) {
	return nil, errors.New("not implemented")
}

// ---------------------------------------------------------------------------
// Mock ordering repository – in-memory implementation of ordering.Repository
// Covers Cart, CartItem, and stub implementations for Orders, Addresses,
// Promos, and Loyalty so the interface is fully satisfied.
// ---------------------------------------------------------------------------

type mockOrderingRepo struct {
	mu sync.RWMutex

	// Cart storage
	carts map[uuid.UUID]*Cart // cartID -> Cart

	// CartItem storage
	cartItems map[uuid.UUID][]CartItem // cartID -> items
}

func newMockOrderingRepo() *mockOrderingRepo {
	return &mockOrderingRepo{
		carts:     make(map[uuid.UUID]*Cart),
		cartItems: make(map[uuid.UUID][]CartItem),
	}
}

// --- Cart operations ---

func (r *mockOrderingRepo) CreateCart(_ context.Context, cart *Cart) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cart.ID == uuid.Nil {
		cart.ID = uuid.New()
	}
	now := time.Now()
	cart.CreatedAt = now
	cart.UpdatedAt = now
	r.carts[cart.ID] = cart
	return nil
}

func (r *mockOrderingRepo) GetCart(_ context.Context, _, cartID uuid.UUID) (*Cart, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cart, ok := r.carts[cartID]
	if !ok {
		return nil, ErrCartNotFound
	}
	// Return a copy so callers can mutate safely.
	c := *cart
	return &c, nil
}

func (r *mockOrderingRepo) GetActiveCartByUser(_ context.Context, tenantID, cafeID, userID uuid.UUID) (*Cart, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, cart := range r.carts {
		if cart.TenantID == tenantID && cart.CafeID == cafeID &&
			cart.UserID != nil && *cart.UserID == userID &&
			cart.Status == CartStatusActive {
			c := *cart
			return &c, nil
		}
	}
	return nil, ErrCartNotFound
}

func (r *mockOrderingRepo) GetActiveCartBySession(_ context.Context, tenantID, cafeID uuid.UUID, sessionID string) (*Cart, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, cart := range r.carts {
		if cart.TenantID == tenantID && cart.CafeID == cafeID &&
			cart.SessionID == sessionID &&
			cart.Status == CartStatusActive {
			c := *cart
			return &c, nil
		}
	}
	return nil, ErrCartNotFound
}

func (r *mockOrderingRepo) UpdateCart(_ context.Context, cart *Cart) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.carts[cart.ID]; !ok {
		return ErrCartNotFound
	}
	cart.UpdatedAt = time.Now()
	r.carts[cart.ID] = cart
	return nil
}

func (r *mockOrderingRepo) DeleteCart(_ context.Context, _, cartID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.carts, cartID)
	delete(r.cartItems, cartID)
	return nil
}

func (r *mockOrderingRepo) ListCarts(_ context.Context, _ CartFilter) ([]Cart, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Cart
	for _, c := range r.carts {
		out = append(out, *c)
	}
	return out, len(out), nil
}

func (r *mockOrderingRepo) ExpireOldCarts(_ context.Context, tenantID uuid.UUID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	now := time.Now()
	for _, cart := range r.carts {
		if cart.TenantID == tenantID && cart.Status == CartStatusActive &&
			cart.ExpiresAt != nil && cart.ExpiresAt.Before(now) {
			cart.Status = CartStatusExpired
			count++
		}
	}
	return count, nil
}

// --- CartItem operations ---

func (r *mockOrderingRepo) CreateCartItem(_ context.Context, item *CartItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	now := time.Now()
	item.CreatedAt = now
	item.UpdatedAt = now
	r.cartItems[item.CartID] = append(r.cartItems[item.CartID], *item)
	return nil
}

func (r *mockOrderingRepo) GetCartItem(_ context.Context, cartID, itemID uuid.UUID) (*CartItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := range r.cartItems[cartID] {
		if r.cartItems[cartID][i].ID == itemID {
			item := r.cartItems[cartID][i]
			return &item, nil
		}
	}
	return nil, ErrCartItemNotFound
}

func (r *mockOrderingRepo) GetCartItemByMenuItem(_ context.Context, cartID, menuItemID uuid.UUID, variantID *uuid.UUID) (*CartItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := range r.cartItems[cartID] {
		ci := &r.cartItems[cartID][i]
		if ci.MenuItemID == menuItemID {
			if variantID == nil && ci.VariantID == nil {
				item := *ci
				return &item, nil
			}
			if variantID != nil && ci.VariantID != nil && *ci.VariantID == *variantID {
				item := *ci
				return &item, nil
			}
		}
	}
	return nil, ErrCartItemNotFound
}

func (r *mockOrderingRepo) UpdateCartItem(_ context.Context, item *CartItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := r.cartItems[item.CartID]
	for i := range items {
		if items[i].ID == item.ID {
			item.UpdatedAt = time.Now()
			items[i] = *item
			r.cartItems[item.CartID] = items
			return nil
		}
	}
	return ErrCartItemNotFound
}

func (r *mockOrderingRepo) DeleteCartItem(_ context.Context, cartID, itemID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := r.cartItems[cartID]
	for i := range items {
		if items[i].ID == itemID {
			r.cartItems[cartID] = append(items[:i], items[i+1:]...)
			return nil
		}
	}
	return ErrCartItemNotFound
}

func (r *mockOrderingRepo) ListCartItems(_ context.Context, cartID uuid.UUID) ([]CartItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.cartItems[cartID]
	out := make([]CartItem, len(items))
	copy(out, items)
	return out, nil
}

func (r *mockOrderingRepo) ClearCartItems(_ context.Context, cartID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cartItems[cartID] = nil
	return nil
}

// --- Order stubs ---

func (r *mockOrderingRepo) CreateOrder(_ context.Context, _ *Order) error {
	return errors.New("not implemented")
}
func (r *mockOrderingRepo) GetOrder(_ context.Context, _, _ uuid.UUID) (*Order, error) {
	return nil, errors.New("not implemented")
}
func (r *mockOrderingRepo) GetOrderByNumber(_ context.Context, _ uuid.UUID, _ string) (*Order, error) {
	return nil, errors.New("not implemented")
}
func (r *mockOrderingRepo) GetOrderByIdempotencyKey(_ context.Context, _ uuid.UUID, _ string) (*Order, error) {
	return nil, errors.New("not implemented")
}
func (r *mockOrderingRepo) UpdateOrder(_ context.Context, _ *Order) error {
	return errors.New("not implemented")
}
func (r *mockOrderingRepo) ListOrders(_ context.Context, _ OrderFilter) ([]Order, int, error) {
	return nil, 0, errors.New("not implemented")
}
func (r *mockOrderingRepo) GenerateOrderNumber(_ context.Context, _, _ uuid.UUID) (string, error) {
	return "", errors.New("not implemented")
}

// --- OrderItem stubs ---

func (r *mockOrderingRepo) CreateOrderItem(_ context.Context, _ *OrderItem) error {
	return errors.New("not implemented")
}
func (r *mockOrderingRepo) GetOrderItem(_ context.Context, _, _ uuid.UUID) (*OrderItem, error) {
	return nil, errors.New("not implemented")
}
func (r *mockOrderingRepo) ListOrderItems(_ context.Context, _ uuid.UUID) ([]OrderItem, error) {
	return nil, errors.New("not implemented")
}

// --- OrderEvent stubs ---

func (r *mockOrderingRepo) CreateOrderEvent(_ context.Context, _ *OrderEvent) error {
	return errors.New("not implemented")
}
func (r *mockOrderingRepo) ListOrderEvents(_ context.Context, _ uuid.UUID) ([]OrderEvent, error) {
	return nil, errors.New("not implemented")
}

// --- CustomerAddress stubs ---

func (r *mockOrderingRepo) CreateAddress(_ context.Context, _ *CustomerAddress) error {
	return errors.New("not implemented")
}
func (r *mockOrderingRepo) GetAddress(_ context.Context, _, _ uuid.UUID) (*CustomerAddress, error) {
	return nil, errors.New("not implemented")
}
func (r *mockOrderingRepo) GetDefaultAddress(_ context.Context, _, _ uuid.UUID) (*CustomerAddress, error) {
	return nil, errors.New("not implemented")
}
func (r *mockOrderingRepo) UpdateAddress(_ context.Context, _ *CustomerAddress) error {
	return errors.New("not implemented")
}
func (r *mockOrderingRepo) DeleteAddress(_ context.Context, _, _ uuid.UUID) error {
	return errors.New("not implemented")
}
func (r *mockOrderingRepo) ListAddresses(_ context.Context, _, _ uuid.UUID) ([]CustomerAddress, error) {
	return nil, errors.New("not implemented")
}
func (r *mockOrderingRepo) SetDefaultAddress(_ context.Context, _, _, _ uuid.UUID) error {
	return errors.New("not implemented")
}
func (r *mockOrderingRepo) CountUserAddresses(_ context.Context, _, _ uuid.UUID) (int, error) {
	return 0, errors.New("not implemented")
}

// --- PromoCode stubs ---

func (r *mockOrderingRepo) CreatePromoCode(_ context.Context, _ *PromoCode) error {
	return errors.New("not implemented")
}
func (r *mockOrderingRepo) GetPromoCode(_ context.Context, _, _ uuid.UUID) (*PromoCode, error) {
	return nil, errors.New("not implemented")
}
func (r *mockOrderingRepo) GetPromoCodeByCode(_ context.Context, _ uuid.UUID, _ string) (*PromoCode, error) {
	return nil, errors.New("not implemented")
}
func (r *mockOrderingRepo) UpdatePromoCode(_ context.Context, _ *PromoCode) error {
	return errors.New("not implemented")
}
func (r *mockOrderingRepo) DeletePromoCode(_ context.Context, _, _ uuid.UUID) error {
	return errors.New("not implemented")
}
func (r *mockOrderingRepo) ListPromoCodes(_ context.Context, _ uuid.UUID, _ *bool) ([]PromoCode, error) {
	return nil, errors.New("not implemented")
}
func (r *mockOrderingRepo) IncrementPromoUsage(_ context.Context, _ uuid.UUID) error {
	return errors.New("not implemented")
}

// --- PromoRedemption stubs ---

func (r *mockOrderingRepo) CreatePromoRedemption(_ context.Context, _ *PromoRedemption) error {
	return errors.New("not implemented")
}
func (r *mockOrderingRepo) GetPromoRedemption(_ context.Context, _ uuid.UUID) (*PromoRedemption, error) {
	return nil, errors.New("not implemented")
}
func (r *mockOrderingRepo) CountUserPromoRedemptions(_ context.Context, _, _ uuid.UUID) (int, error) {
	return 0, errors.New("not implemented")
}
func (r *mockOrderingRepo) ListPromoRedemptions(_ context.Context, _ uuid.UUID) ([]PromoRedemption, error) {
	return nil, errors.New("not implemented")
}

// --- LoyaltyAccount stubs ---

func (r *mockOrderingRepo) CreateLoyaltyAccount(_ context.Context, _ *LoyaltyAccount) error {
	return errors.New("not implemented")
}
func (r *mockOrderingRepo) GetLoyaltyAccount(_ context.Context, _, _ uuid.UUID) (*LoyaltyAccount, error) {
	return nil, errors.New("not implemented")
}
func (r *mockOrderingRepo) GetLoyaltyAccountByUser(_ context.Context, _, _ uuid.UUID) (*LoyaltyAccount, error) {
	return nil, errors.New("not implemented")
}
func (r *mockOrderingRepo) UpdateLoyaltyAccount(_ context.Context, _ *LoyaltyAccount) error {
	return errors.New("not implemented")
}
func (r *mockOrderingRepo) AddLoyaltyPoints(_ context.Context, _ uuid.UUID, _ int) error {
	return errors.New("not implemented")
}
func (r *mockOrderingRepo) DeductLoyaltyPoints(_ context.Context, _ uuid.UUID, _ int) error {
	return errors.New("not implemented")
}

// --- LoyaltyTransaction stubs ---

func (r *mockOrderingRepo) CreateLoyaltyTransaction(_ context.Context, _ *LoyaltyTransaction) error {
	return errors.New("not implemented")
}
func (r *mockOrderingRepo) ListLoyaltyTransactions(_ context.Context, _ uuid.UUID, _, _ int) ([]LoyaltyTransaction, int, error) {
	return nil, 0, errors.New("not implemented")
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// testHarness bundles the dependencies needed for every test.
type testHarness struct {
	svc         *CartService
	repo        *mockOrderingRepo
	catalogRepo *mockCatalogRepo
	logger      *zap.Logger

	// Reusable IDs shared across tests.
	tenantID uuid.UUID
	cafeID   uuid.UUID
	userID   uuid.UUID
}

func newTestHarness(t *testing.T) *testHarness {
	t.Helper()
	logger := zaptest.NewLogger(t)
	orderingRepo := newMockOrderingRepo()
	catalogRepo := newMockCatalogRepo()
	catalogSvc := catalog.NewService(catalogRepo, logger)
	svc := NewCartService(orderingRepo, catalogSvc, logger)

	return &testHarness{
		svc:         svc,
		repo:        orderingRepo,
		catalogRepo: catalogRepo,
		logger:      logger,
		tenantID:    uuid.New(),
		cafeID:      uuid.New(),
		userID:      uuid.New(),
	}
}

// seedAvailableMenuItem inserts a menu item into the catalog mock and returns it.
func (h *testHarness) seedAvailableMenuItem(basePrice float64, variants ...catalog.Variant) *catalog.MenuItem {
	item := &catalog.MenuItem{
		ID:          uuid.New(),
		TenantID:    h.tenantID,
		CafeID:      h.cafeID,
		Name:        fmt.Sprintf("Test Item %s", uuid.New().String()[:8]),
		BasePrice:   basePrice,
		IsAvailable: true,
		Variants:    variants,
	}
	h.catalogRepo.seedMenuItem(item)
	return item
}

// seedUnavailableMenuItem inserts an unavailable menu item.
func (h *testHarness) seedUnavailableMenuItem(basePrice float64) *catalog.MenuItem {
	item := &catalog.MenuItem{
		ID:          uuid.New(),
		TenantID:    h.tenantID,
		CafeID:      h.cafeID,
		Name:        "Unavailable Item",
		BasePrice:   basePrice,
		IsAvailable: false,
	}
	h.catalogRepo.seedMenuItem(item)
	return item
}

func ptrUUID(id uuid.UUID) *uuid.UUID { return &id }
func ptrInt(n int) *int               { return &n }
func ptrString(s string) *string       { return &s }

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestGetOrCreateCart(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(h *testHarness)
		userID    func(h *testHarness) *uuid.UUID
		sessionID string
		wantErr   error
		check     func(t *testing.T, h *testHarness, cart *Cart)
	}{
		{
			name:  "creates new cart for user",
			setup: func(_ *testHarness) {},
			userID: func(h *testHarness) *uuid.UUID {
				return &h.userID
			},
			sessionID: "",
			wantErr:   nil,
			check: func(t *testing.T, h *testHarness, cart *Cart) {
				assert.NotEqual(t, uuid.Nil, cart.ID)
				assert.Equal(t, h.tenantID, cart.TenantID)
				assert.Equal(t, h.cafeID, cart.CafeID)
				assert.Equal(t, &h.userID, cart.UserID)
				assert.Equal(t, CartStatusActive, cart.Status)
				assert.Equal(t, DefaultCurrency, cart.Currency)
				assert.NotNil(t, cart.ExpiresAt)
			},
		},
		{
			name:  "creates new cart for session guest",
			setup: func(_ *testHarness) {},
			userID: func(_ *testHarness) *uuid.UUID {
				return nil
			},
			sessionID: "guest-session-123",
			wantErr:   nil,
			check: func(t *testing.T, _ *testHarness, cart *Cart) {
				assert.NotEqual(t, uuid.Nil, cart.ID)
				assert.Nil(t, cart.UserID)
				assert.Equal(t, "guest-session-123", cart.SessionID)
				assert.Equal(t, CartStatusActive, cart.Status)
			},
		},
		{
			name: "returns existing active cart for user",
			setup: func(h *testHarness) {
				// Pre-create a cart for this user.
				ctx := context.Background()
				_, _ = h.svc.GetOrCreateCart(ctx, h.tenantID, h.cafeID, &h.userID, "")
			},
			userID: func(h *testHarness) *uuid.UUID {
				return &h.userID
			},
			sessionID: "",
			wantErr:   nil,
			check: func(t *testing.T, h *testHarness, cart *Cart) {
				// Should not have created a second cart.
				h.repo.mu.RLock()
				defer h.repo.mu.RUnlock()
				userCarts := 0
				for _, c := range h.repo.carts {
					if c.TenantID == h.tenantID && c.UserID != nil && *c.UserID == h.userID && c.Status == CartStatusActive {
						userCarts++
					}
				}
				assert.Equal(t, 1, userCarts, "should have exactly one active cart for this user")
			},
		},
		{
			name: "expired cart creates new cart",
			setup: func(h *testHarness) {
				ctx := context.Background()
				expired := time.Now().Add(-1 * time.Hour)
				cart := &Cart{
					TenantID:  h.tenantID,
					CafeID:    h.cafeID,
					UserID:    &h.userID,
					Status:    CartStatusActive,
					Currency:  DefaultCurrency,
					ExpiresAt: &expired,
				}
				_ = h.repo.CreateCart(ctx, cart)
			},
			userID: func(h *testHarness) *uuid.UUID {
				return &h.userID
			},
			sessionID: "",
			wantErr:   nil,
			check: func(t *testing.T, h *testHarness, cart *Cart) {
				assert.Equal(t, CartStatusActive, cart.Status)
				assert.True(t, cart.ExpiresAt.After(time.Now()), "new cart should have future expiry")

				// Old cart should be marked expired.
				h.repo.mu.RLock()
				defer h.repo.mu.RUnlock()
				expiredCount := 0
				for _, c := range h.repo.carts {
					if c.TenantID == h.tenantID && c.Status == CartStatusExpired {
						expiredCount++
					}
				}
				assert.Equal(t, 1, expiredCount, "old cart should be expired")
			},
		},
		{
			name:  "returns ErrUnauthorized when no user or session",
			setup: func(_ *testHarness) {},
			userID: func(_ *testHarness) *uuid.UUID {
				return nil
			},
			sessionID: "",
			wantErr:   ErrUnauthorized,
			check:     nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHarness(t)
			tc.setup(h)

			ctx := context.Background()
			cart, err := h.svc.GetOrCreateCart(ctx, h.tenantID, h.cafeID, tc.userID(h), tc.sessionID)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cart)
			if tc.check != nil {
				tc.check(t, h, cart)
			}
		})
	}
}

func TestAddItem(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(h *testHarness) AddItemRequest
		wantErr error
		check   func(t *testing.T, h *testHarness, cart *Cart)
	}{
		{
			name: "adds new item to cart",
			setup: func(h *testHarness) AddItemRequest {
				mi := h.seedAvailableMenuItem(250.0)
				return AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   2,
					Notes:      "extra spicy",
				}
			},
			wantErr: nil,
			check: func(t *testing.T, _ *testHarness, cart *Cart) {
				require.Len(t, cart.Items, 1)
				assert.Equal(t, 2, cart.Items[0].Quantity)
				assert.Equal(t, 250.0, cart.Items[0].UnitPrice)
				assert.Equal(t, 500.0, cart.Items[0].TotalPrice)
				assert.Equal(t, "extra spicy", cart.Items[0].Notes)
				assert.Equal(t, 500.0, cart.Subtotal)
			},
		},
		{
			name: "rejects zero quantity",
			setup: func(h *testHarness) AddItemRequest {
				mi := h.seedAvailableMenuItem(100.0)
				return AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   0,
				}
			},
			wantErr: ErrInvalidQuantity,
			check:   nil,
		},
		{
			name: "rejects negative quantity",
			setup: func(h *testHarness) AddItemRequest {
				mi := h.seedAvailableMenuItem(100.0)
				return AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   -1,
				}
			},
			wantErr: ErrInvalidQuantity,
			check:   nil,
		},
		{
			name: "rejects unavailable menu item",
			setup: func(h *testHarness) AddItemRequest {
				mi := h.seedUnavailableMenuItem(100.0)
				return AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   1,
				}
			},
			wantErr: ErrMenuItemUnavailable,
			check:   nil,
		},
		{
			name: "rejects non-existent menu item",
			setup: func(h *testHarness) AddItemRequest {
				return AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: uuid.New(), // does not exist
					Quantity:   1,
				}
			},
			wantErr: ErrMenuItemUnavailable,
			check:   nil,
		},
		{
			name: "adds item with valid variant and price delta",
			setup: func(h *testHarness) AddItemRequest {
				variantID := uuid.New()
				mi := h.seedAvailableMenuItem(200.0, catalog.Variant{
					ID:          variantID,
					Name:        "Large",
					PriceDelta:  50.0,
					IsAvailable: true,
				})
				return AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					VariantID:  ptrUUID(variantID),
					Quantity:   1,
				}
			},
			wantErr: nil,
			check: func(t *testing.T, _ *testHarness, cart *Cart) {
				require.Len(t, cart.Items, 1)
				assert.Equal(t, 250.0, cart.Items[0].UnitPrice, "base 200 + variant delta 50")
				assert.Equal(t, 250.0, cart.Items[0].TotalPrice)
			},
		},
		{
			name: "rejects unavailable variant",
			setup: func(h *testHarness) AddItemRequest {
				variantID := uuid.New()
				mi := h.seedAvailableMenuItem(200.0, catalog.Variant{
					ID:          variantID,
					Name:        "XL",
					PriceDelta:  100.0,
					IsAvailable: false,
				})
				return AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					VariantID:  ptrUUID(variantID),
					Quantity:   1,
				}
			},
			wantErr: ErrVariantUnavailable,
			check:   nil,
		},
		{
			name: "rejects non-existent variant ID",
			setup: func(h *testHarness) AddItemRequest {
				mi := h.seedAvailableMenuItem(200.0, catalog.Variant{
					ID:          uuid.New(),
					Name:        "Small",
					PriceDelta:  -20.0,
					IsAvailable: true,
				})
				return AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					VariantID:  ptrUUID(uuid.New()), // different ID
					Quantity:   1,
				}
			},
			wantErr: ErrVariantUnavailable,
			check:   nil,
		},
		{
			name: "deduplicates same menu item by incrementing quantity",
			setup: func(h *testHarness) AddItemRequest {
				mi := h.seedAvailableMenuItem(100.0)
				ctx := context.Background()
				// First add.
				_, _ = h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   3,
					Notes:      "first add",
				})
				// Return the second add request for the same item.
				return AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   2,
					Notes:      "second add",
				}
			},
			wantErr: nil,
			check: func(t *testing.T, _ *testHarness, cart *Cart) {
				require.Len(t, cart.Items, 1, "should still be one item, not two")
				assert.Equal(t, 5, cart.Items[0].Quantity, "3 + 2 = 5")
				assert.Equal(t, 500.0, cart.Items[0].TotalPrice, "100 * 5")
				assert.Equal(t, "second add", cart.Items[0].Notes, "notes should be updated")
			},
		},
		{
			name: "deduplicates same item with same variant",
			setup: func(h *testHarness) AddItemRequest {
				variantID := uuid.New()
				mi := h.seedAvailableMenuItem(200.0, catalog.Variant{
					ID:          variantID,
					Name:        "Large",
					PriceDelta:  50.0,
					IsAvailable: true,
				})
				ctx := context.Background()
				_, _ = h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					VariantID:  ptrUUID(variantID),
					Quantity:   1,
				})
				return AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					VariantID:  ptrUUID(variantID),
					Quantity:   2,
				}
			},
			wantErr: nil,
			check: func(t *testing.T, _ *testHarness, cart *Cart) {
				require.Len(t, cart.Items, 1)
				assert.Equal(t, 3, cart.Items[0].Quantity)
				assert.Equal(t, 750.0, cart.Items[0].TotalPrice, "250 * 3")
			},
		},
		{
			name: "different variants create separate line items",
			setup: func(h *testHarness) AddItemRequest {
				variantA := uuid.New()
				variantB := uuid.New()
				mi := h.seedAvailableMenuItem(200.0,
					catalog.Variant{ID: variantA, Name: "Small", PriceDelta: 0, IsAvailable: true},
					catalog.Variant{ID: variantB, Name: "Large", PriceDelta: 100, IsAvailable: true},
				)
				ctx := context.Background()
				_, _ = h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					VariantID:  ptrUUID(variantA),
					Quantity:   1,
				})
				return AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					VariantID:  ptrUUID(variantB),
					Quantity:   1,
				}
			},
			wantErr: nil,
			check: func(t *testing.T, _ *testHarness, cart *Cart) {
				assert.Len(t, cart.Items, 2, "different variants should produce separate items")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHarness(t)
			req := tc.setup(h)

			ctx := context.Background()
			cart, err := h.svc.AddItem(ctx, req)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cart)
			if tc.check != nil {
				tc.check(t, h, cart)
			}
		})
	}
}

func TestUpdateItem(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(h *testHarness) UpdateItemRequest
		wantErr error
		check   func(t *testing.T, h *testHarness, cart *Cart)
	}{
		{
			name: "updates quantity",
			setup: func(h *testHarness) UpdateItemRequest {
				mi := h.seedAvailableMenuItem(150.0)
				ctx := context.Background()
				cart, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   2,
				})
				require.NoError(t, err)
				return UpdateItemRequest{
					TenantID: h.tenantID,
					CartID:   cart.ID,
					ItemID:   cart.Items[0].ID,
					Quantity: ptrInt(5),
				}
			},
			wantErr: nil,
			check: func(t *testing.T, _ *testHarness, cart *Cart) {
				require.Len(t, cart.Items, 1)
				assert.Equal(t, 5, cart.Items[0].Quantity)
				assert.Equal(t, 750.0, cart.Items[0].TotalPrice, "150 * 5 = 750")
				assert.Equal(t, 750.0, cart.Subtotal)
			},
		},
		{
			name: "removes item when quantity set to zero",
			setup: func(h *testHarness) UpdateItemRequest {
				mi := h.seedAvailableMenuItem(100.0)
				ctx := context.Background()
				cart, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   3,
				})
				require.NoError(t, err)
				return UpdateItemRequest{
					TenantID: h.tenantID,
					CartID:   cart.ID,
					ItemID:   cart.Items[0].ID,
					Quantity: ptrInt(0),
				}
			},
			wantErr: nil,
			check: func(t *testing.T, _ *testHarness, cart *Cart) {
				assert.Empty(t, cart.Items, "item should be removed")
				assert.Equal(t, 0.0, cart.Subtotal)
			},
		},
		{
			name: "removes item when quantity set to negative",
			setup: func(h *testHarness) UpdateItemRequest {
				mi := h.seedAvailableMenuItem(100.0)
				ctx := context.Background()
				cart, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   1,
				})
				require.NoError(t, err)
				return UpdateItemRequest{
					TenantID: h.tenantID,
					CartID:   cart.ID,
					ItemID:   cart.Items[0].ID,
					Quantity: ptrInt(-5),
				}
			},
			wantErr: nil,
			check: func(t *testing.T, _ *testHarness, cart *Cart) {
				assert.Empty(t, cart.Items, "item should be removed for negative qty")
			},
		},
		{
			name: "updates notes only",
			setup: func(h *testHarness) UpdateItemRequest {
				mi := h.seedAvailableMenuItem(100.0)
				ctx := context.Background()
				cart, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   1,
					Notes:      "original notes",
				})
				require.NoError(t, err)
				return UpdateItemRequest{
					TenantID: h.tenantID,
					CartID:   cart.ID,
					ItemID:   cart.Items[0].ID,
					Notes:    ptrString("updated notes"),
				}
			},
			wantErr: nil,
			check: func(t *testing.T, _ *testHarness, cart *Cart) {
				require.Len(t, cart.Items, 1)
				assert.Equal(t, "updated notes", cart.Items[0].Notes)
				assert.Equal(t, 1, cart.Items[0].Quantity, "quantity unchanged")
			},
		},
		{
			name: "rejects update on checked-out cart",
			setup: func(h *testHarness) UpdateItemRequest {
				mi := h.seedAvailableMenuItem(100.0)
				ctx := context.Background()
				cart, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   1,
				})
				require.NoError(t, err)
				// Manually mark cart as checked out.
				h.repo.mu.Lock()
				h.repo.carts[cart.ID].Status = CartStatusCheckedOut
				h.repo.mu.Unlock()

				return UpdateItemRequest{
					TenantID: h.tenantID,
					CartID:   cart.ID,
					ItemID:   cart.Items[0].ID,
					Quantity: ptrInt(10),
				}
			},
			wantErr: ErrCartAlreadyCheckedOut,
			check:   nil,
		},
		{
			name: "returns error for non-existent item",
			setup: func(h *testHarness) UpdateItemRequest {
				mi := h.seedAvailableMenuItem(100.0)
				ctx := context.Background()
				cart, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   1,
				})
				require.NoError(t, err)
				return UpdateItemRequest{
					TenantID: h.tenantID,
					CartID:   cart.ID,
					ItemID:   uuid.New(), // does not exist
					Quantity: ptrInt(5),
				}
			},
			wantErr: ErrCartItemNotFound,
			check:   nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHarness(t)
			req := tc.setup(h)

			ctx := context.Background()
			cart, err := h.svc.UpdateItem(ctx, req)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cart)
			if tc.check != nil {
				tc.check(t, h, cart)
			}
		})
	}
}

func TestRemoveItem(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(h *testHarness) (cartID, itemID uuid.UUID)
		wantErr error
		check   func(t *testing.T, h *testHarness, cart *Cart)
	}{
		{
			name: "removes item from cart",
			setup: func(h *testHarness) (uuid.UUID, uuid.UUID) {
				mi := h.seedAvailableMenuItem(200.0)
				ctx := context.Background()
				cart, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   3,
				})
				require.NoError(t, err)
				return cart.ID, cart.Items[0].ID
			},
			wantErr: nil,
			check: func(t *testing.T, _ *testHarness, cart *Cart) {
				assert.Empty(t, cart.Items)
				assert.Equal(t, 0.0, cart.Subtotal)
			},
		},
		{
			name: "removes one item, keeps others",
			setup: func(h *testHarness) (uuid.UUID, uuid.UUID) {
				mi1 := h.seedAvailableMenuItem(100.0)
				mi2 := h.seedAvailableMenuItem(200.0)
				ctx := context.Background()
				_, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi1.ID,
					Quantity:   1,
				})
				require.NoError(t, err)
				cart, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi2.ID,
					Quantity:   2,
				})
				require.NoError(t, err)
				// Remove the first item (mi1).
				var mi1ItemID uuid.UUID
				for _, item := range cart.Items {
					if item.MenuItemID == mi1.ID {
						mi1ItemID = item.ID
						break
					}
				}
				return cart.ID, mi1ItemID
			},
			wantErr: nil,
			check: func(t *testing.T, _ *testHarness, cart *Cart) {
				assert.Len(t, cart.Items, 1, "one item should remain")
				assert.Equal(t, 400.0, cart.Subtotal, "200 * 2 remaining")
			},
		},
		{
			name: "rejects removal from checked-out cart",
			setup: func(h *testHarness) (uuid.UUID, uuid.UUID) {
				mi := h.seedAvailableMenuItem(100.0)
				ctx := context.Background()
				cart, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   1,
				})
				require.NoError(t, err)

				h.repo.mu.Lock()
				h.repo.carts[cart.ID].Status = CartStatusCheckedOut
				h.repo.mu.Unlock()

				return cart.ID, cart.Items[0].ID
			},
			wantErr: ErrCartAlreadyCheckedOut,
			check:   nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHarness(t)
			cartID, itemID := tc.setup(h)

			ctx := context.Background()
			cart, err := h.svc.RemoveItem(ctx, h.tenantID, cartID, itemID)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cart)
			if tc.check != nil {
				tc.check(t, h, cart)
			}
		})
	}
}

func TestClearCart(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(h *testHarness) uuid.UUID // returns cartID
		wantErr error
		check   func(t *testing.T, h *testHarness, cart *Cart)
	}{
		{
			name: "clears all items and resets totals",
			setup: func(h *testHarness) uuid.UUID {
				mi1 := h.seedAvailableMenuItem(100.0)
				mi2 := h.seedAvailableMenuItem(250.0)
				ctx := context.Background()
				_, _ = h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi1.ID,
					Quantity:   2,
				})
				cart, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi2.ID,
					Quantity:   1,
				})
				require.NoError(t, err)
				assert.Equal(t, 450.0, cart.Subtotal, "pre-clear subtotal should be 450")
				return cart.ID
			},
			wantErr: nil,
			check: func(t *testing.T, _ *testHarness, cart *Cart) {
				assert.Empty(t, cart.Items)
				assert.Equal(t, 0.0, cart.Subtotal)
				assert.Equal(t, 0.0, cart.DiscountTotal)
				assert.Equal(t, 0.0, cart.TaxTotal)
				assert.Equal(t, 0, cart.LoyaltyPointsRedeemed)
				assert.Nil(t, cart.PromoCodeID)
			},
		},
		{
			name: "rejects clear on checked-out cart",
			setup: func(h *testHarness) uuid.UUID {
				mi := h.seedAvailableMenuItem(100.0)
				ctx := context.Background()
				cart, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   1,
				})
				require.NoError(t, err)
				h.repo.mu.Lock()
				h.repo.carts[cart.ID].Status = CartStatusCheckedOut
				h.repo.mu.Unlock()
				return cart.ID
			},
			wantErr: ErrCartAlreadyCheckedOut,
			check:   nil,
		},
		{
			name: "returns error for non-existent cart",
			setup: func(_ *testHarness) uuid.UUID {
				return uuid.New()
			},
			wantErr: ErrCartNotFound,
			check:   nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHarness(t)
			cartID := tc.setup(h)

			ctx := context.Background()
			cart, err := h.svc.ClearCart(ctx, h.tenantID, cartID)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cart)
			if tc.check != nil {
				tc.check(t, h, cart)
			}
		})
	}
}

func TestGetCartSummary(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(h *testHarness) uuid.UUID // returns cartID
		wantErr error
		check   func(t *testing.T, summary *CartSummary)
	}{
		{
			name: "calculates correct totals with items only",
			setup: func(h *testHarness) uuid.UUID {
				mi1 := h.seedAvailableMenuItem(100.0)
				mi2 := h.seedAvailableMenuItem(200.0)
				ctx := context.Background()
				_, _ = h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi1.ID,
					Quantity:   3, // 300
				})
				cart, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi2.ID,
					Quantity:   2, // 400
				})
				require.NoError(t, err)
				return cart.ID
			},
			wantErr: nil,
			check: func(t *testing.T, s *CartSummary) {
				assert.Equal(t, 700.0, s.Subtotal)
				assert.Equal(t, 0.0, s.DiscountTotal)
				assert.Equal(t, 0.0, s.TaxTotal)
				assert.Equal(t, 0.0, s.DeliveryFee)
				assert.Equal(t, 0, s.LoyaltyPointsRedeemed)
				assert.Equal(t, 0.0, s.LoyaltyDiscount)
				assert.Equal(t, 700.0, s.GrandTotal)
			},
		},
		{
			name: "calculates grand total with loyalty points and delivery fee",
			setup: func(h *testHarness) uuid.UUID {
				mi := h.seedAvailableMenuItem(500.0)
				ctx := context.Background()
				cart, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   1, // 500
				})
				require.NoError(t, err)

				// Manually set loyalty points and delivery fee on the cart.
				h.repo.mu.Lock()
				h.repo.carts[cart.ID].LoyaltyPointsRedeemed = 100 // 100 * 0.1 = 10 KES
				h.repo.carts[cart.ID].DeliveryFee = 50.0
				h.repo.carts[cart.ID].DiscountTotal = 20.0
				h.repo.carts[cart.ID].TaxTotal = 30.0
				h.repo.mu.Unlock()

				return cart.ID
			},
			wantErr: nil,
			check: func(t *testing.T, s *CartSummary) {
				assert.Equal(t, 500.0, s.Subtotal)
				assert.Equal(t, 20.0, s.DiscountTotal)
				assert.Equal(t, 30.0, s.TaxTotal)
				assert.Equal(t, 50.0, s.DeliveryFee)
				assert.Equal(t, 100, s.LoyaltyPointsRedeemed)
				assert.Equal(t, 10.0, s.LoyaltyDiscount, "100 points * 0.1 = 10 KES")
				// GrandTotal = 500 - 20 - 10 + 30 + 50 = 550
				assert.Equal(t, 550.0, s.GrandTotal)
			},
		},
		{
			name: "returns error for non-existent cart",
			setup: func(_ *testHarness) uuid.UUID {
				return uuid.New()
			},
			wantErr: ErrCartNotFound,
			check:   nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHarness(t)
			cartID := tc.setup(h)

			ctx := context.Background()
			summary, err := h.svc.GetCartSummary(ctx, h.tenantID, cartID)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, summary)
			if tc.check != nil {
				tc.check(t, summary)
			}
		})
	}
}

func TestMergeGuestCart(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		setup     func(h *testHarness, sessionID string)
		wantErr   error
		check     func(t *testing.T, h *testHarness, cart *Cart)
	}{
		{
			name:      "merges guest items into empty user cart",
			sessionID: "guest-sess-merge-1",
			setup: func(h *testHarness, sessionID string) {
				mi := h.seedAvailableMenuItem(300.0)
				ctx := context.Background()
				// Create guest cart with items.
				_, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					SessionID:  sessionID,
					MenuItemID: mi.ID,
					Quantity:   2,
				})
				require.NoError(t, err)
			},
			wantErr: nil,
			check: func(t *testing.T, h *testHarness, cart *Cart) {
				assert.NotNil(t, cart.UserID)
				assert.Equal(t, h.userID, *cart.UserID)
				require.Len(t, cart.Items, 1)
				assert.Equal(t, 2, cart.Items[0].Quantity)
				assert.Equal(t, 600.0, cart.Subtotal, "300 * 2")

				// Guest cart should be abandoned.
				h.repo.mu.RLock()
				defer h.repo.mu.RUnlock()
				abandoned := 0
				for _, c := range h.repo.carts {
					if c.Status == CartStatusAbandoned {
						abandoned++
					}
				}
				assert.Equal(t, 1, abandoned, "guest cart should be abandoned")
			},
		},
		{
			name:      "merges guest items and combines quantities for matching items",
			sessionID: "guest-sess-merge-2",
			setup: func(h *testHarness, sessionID string) {
				mi := h.seedAvailableMenuItem(100.0)
				ctx := context.Background()

				// User already has this item in their cart.
				_, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   3,
				})
				require.NoError(t, err)

				// Guest also has the same item.
				_, err = h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					SessionID:  sessionID,
					MenuItemID: mi.ID,
					Quantity:   2,
				})
				require.NoError(t, err)
			},
			wantErr: nil,
			check: func(t *testing.T, _ *testHarness, cart *Cart) {
				require.Len(t, cart.Items, 1)
				assert.Equal(t, 5, cart.Items[0].Quantity, "3 user + 2 guest = 5")
				assert.Equal(t, 500.0, cart.Subtotal, "100 * 5")
			},
		},
		{
			name:      "merges guest cart with different items into user cart",
			sessionID: "guest-sess-merge-3",
			setup: func(h *testHarness, sessionID string) {
				mi1 := h.seedAvailableMenuItem(100.0)
				mi2 := h.seedAvailableMenuItem(200.0)
				ctx := context.Background()

				// User has item 1.
				_, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi1.ID,
					Quantity:   1,
				})
				require.NoError(t, err)

				// Guest has item 2.
				_, err = h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					SessionID:  sessionID,
					MenuItemID: mi2.ID,
					Quantity:   1,
				})
				require.NoError(t, err)
			},
			wantErr: nil,
			check: func(t *testing.T, _ *testHarness, cart *Cart) {
				assert.Len(t, cart.Items, 2, "should have both items")
				assert.Equal(t, 300.0, cart.Subtotal, "100 + 200")
			},
		},
		{
			name:      "no guest cart creates fresh user cart",
			sessionID: "no-such-session",
			setup:     func(_ *testHarness, _ string) {},
			wantErr:   nil,
			check: func(t *testing.T, h *testHarness, cart *Cart) {
				assert.NotNil(t, cart.UserID)
				assert.Equal(t, h.userID, *cart.UserID)
				assert.Empty(t, cart.Items)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHarness(t)
			tc.setup(h, tc.sessionID)

			ctx := context.Background()
			cart, err := h.svc.MergeGuestCart(ctx, h.tenantID, h.cafeID, tc.sessionID, h.userID)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cart)
			if tc.check != nil {
				tc.check(t, h, cart)
			}
		})
	}
}

func TestGetCart(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(h *testHarness) uuid.UUID
		wantErr error
		check   func(t *testing.T, h *testHarness, cart *Cart)
	}{
		{
			name: "retrieves cart by ID with items loaded",
			setup: func(h *testHarness) uuid.UUID {
				mi := h.seedAvailableMenuItem(150.0)
				ctx := context.Background()
				cart, err := h.svc.AddItem(ctx, AddItemRequest{
					TenantID:   h.tenantID,
					CafeID:     h.cafeID,
					UserID:     &h.userID,
					MenuItemID: mi.ID,
					Quantity:   2,
				})
				require.NoError(t, err)
				return cart.ID
			},
			wantErr: nil,
			check: func(t *testing.T, _ *testHarness, cart *Cart) {
				require.Len(t, cart.Items, 1)
				assert.Equal(t, 2, cart.Items[0].Quantity)
			},
		},
		{
			name: "returns error for non-existent cart",
			setup: func(_ *testHarness) uuid.UUID {
				return uuid.New()
			},
			wantErr: ErrCartNotFound,
			check:   nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHarness(t)
			cartID := tc.setup(h)

			ctx := context.Background()
			cart, err := h.svc.GetCart(ctx, h.tenantID, cartID)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cart)
			if tc.check != nil {
				tc.check(t, h, cart)
			}
		})
	}
}
