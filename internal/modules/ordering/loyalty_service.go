package ordering

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/platform/posloyalty"
)

// LoyaltyService provides loyalty program business logic.
//
// SOURCE OF TRUTH: pos-api is the authoritative loyalty balance store (keyed on
// tenant + customer_phone). This service's local LoyaltyAccount (keyed on user_id) is a LEGACY
// MIRROR kept for backward compatibility during the transition; EarnPoints/RedeemPoints write
// locally AND mirror to pos-api over S2S (best-effort). Once all readers consume pos-api directly,
// the local entity + writes can be removed (full cutover). Do not add new readers of the local
// balance.
type LoyaltyService struct {
	repo      Repository
	logger    *zap.Logger
	posClient *posloyalty.Client
}

// NewLoyaltyService creates a new loyalty service.
func NewLoyaltyService(repo Repository, logger *zap.Logger) *LoyaltyService {
	return &LoyaltyService{
		repo:   repo,
		logger: logger,
	}
}

// SetPOSLoyaltyClient wires the pos-api loyalty S2S client. pos-api is the loyalty source of
// truth; once set, earn/redeem are mirrored to it (best-effort). When nil (or the client is
// disabled), only the legacy local balance is written.
func (s *LoyaltyService) SetPOSLoyaltyClient(c *posloyalty.Client) {
	s.posClient = c
}

// mirrorEarnToPOS forwards an earn to pos-api (the loyalty SoT). Best-effort: errors are logged and
// swallowed so a pos-api outage never fails the order. Requires the customer's phone (pos-api keys
// loyalty on phone); when phone is empty the mirror is skipped and only the local balance applies.
func (s *LoyaltyService) mirrorEarnToPOS(ctx context.Context, tenantID uuid.UUID, phone, name string, points int, orderID *uuid.UUID) {
	if s.posClient == nil || !s.posClient.Enabled() {
		return
	}
	if err := s.posClient.Earn(ctx, tenantID, phone, name, points, orderID); err != nil {
		s.logger.Warn("loyalty: pos-api earn mirror failed (continuing; pos-api is SoT, local is legacy mirror)", zap.Error(err))
	}
}

// mirrorRedeemToPOS forwards a redeem to pos-api (the loyalty SoT). Best-effort (see mirrorEarnToPOS).
func (s *LoyaltyService) mirrorRedeemToPOS(ctx context.Context, tenantID uuid.UUID, phone string, points int, orderID *uuid.UUID) {
	if s.posClient == nil || !s.posClient.Enabled() {
		return
	}
	if err := s.posClient.Redeem(ctx, tenantID, phone, points, orderID); err != nil {
		s.logger.Warn("loyalty: pos-api redeem mirror failed (continuing; pos-api is SoT, local is legacy mirror)", zap.Error(err))
	}
}

// GetOrCreateAccount gets or creates a loyalty account for a user.
func (s *LoyaltyService) GetOrCreateAccount(ctx context.Context, tenantID, userID uuid.UUID) (*LoyaltyAccount, error) {
	account, err := s.repo.GetLoyaltyAccountByUser(ctx, tenantID, userID)
	if err == nil && account != nil {
		return account, nil
	}

	// Create new account
	account = &LoyaltyAccount{
		TenantID:       tenantID,
		UserID:         userID,
		BalancePoints:  0,
		Tier:           LoyaltyTierBronze,
		LifetimePoints: 0,
	}

	if err := s.repo.CreateLoyaltyAccount(ctx, account); err != nil {
		s.logger.Error("failed to create loyalty account", zap.Error(err))
		return nil, err
	}

	s.logger.Info("loyalty account created",
		zap.String("id", account.ID.String()),
		zap.String("userId", userID.String()))

	return account, nil
}

// GetAccountByUser retrieves a loyalty account by user ID.
func (s *LoyaltyService) GetAccountByUser(ctx context.Context, tenantID, userID uuid.UUID) (*LoyaltyAccount, error) {
	return s.repo.GetLoyaltyAccountByUser(ctx, tenantID, userID)
}

// GetBalance retrieves the loyalty points balance for a user.
func (s *LoyaltyService) GetBalance(ctx context.Context, tenantID, userID uuid.UUID) (int, error) {
	account, err := s.GetOrCreateAccount(ctx, tenantID, userID)
	if err != nil {
		return 0, err
	}
	return account.BalancePoints, nil
}

// EarnPoints adds points to a user's loyalty account.
//
// customerPhone/customerName let the change be mirrored to pos-api, the loyalty source of truth
// (pos-api keys loyalty on phone). The local write below is the LEGACY MIRROR kept for backward
// compatibility; the pos-api mirror is best-effort and never fails the caller.
func (s *LoyaltyService) EarnPoints(ctx context.Context, tenantID, userID uuid.UUID, points int, orderID *uuid.UUID, description, customerPhone, customerName string) error {
	if points <= 0 {
		return ErrInvalidLoyaltyPoints
	}

	// Mirror to pos-api (loyalty SoT) first — best-effort, does not block the local write.
	s.mirrorEarnToPOS(ctx, tenantID, customerPhone, customerName, points, orderID)

	account, err := s.GetOrCreateAccount(ctx, tenantID, userID)
	if err != nil {
		return err
	}

	// Add points
	if err := s.repo.AddLoyaltyPoints(ctx, account.ID, points); err != nil {
		return err
	}

	// Update tier based on lifetime points
	newLifetimePoints := account.LifetimePoints + points
	newTier := s.calculateTier(newLifetimePoints)
	if newTier != account.Tier {
		account.Tier = newTier
		if err := s.repo.UpdateLoyaltyAccount(ctx, account); err != nil {
			s.logger.Error("failed to update loyalty tier", zap.Error(err))
		}
	}

	// Record transaction
	tx := &LoyaltyTransaction{
		AccountID:       account.ID,
		OrderID:         orderID,
		Points:          points,
		TransactionType: LoyaltyTransactionTypeEarn,
		Description:     description,
		OccurredAt:      time.Now(),
	}

	if err := s.repo.CreateLoyaltyTransaction(ctx, tx); err != nil {
		s.logger.Error("failed to record loyalty transaction", zap.Error(err))
	}

	s.logger.Info("loyalty points earned",
		zap.String("userId", userID.String()),
		zap.Int("points", points))

	return nil
}

// RedeemPoints deducts points from a user's loyalty account.
//
// customerPhone lets the change be mirrored to pos-api, the loyalty source of truth. The local
// write below is the LEGACY MIRROR kept for backward compatibility; the pos-api mirror is
// best-effort and never fails the caller.
func (s *LoyaltyService) RedeemPoints(ctx context.Context, tenantID, userID uuid.UUID, points int, orderID *uuid.UUID, description, customerPhone string) error {
	if points <= 0 {
		return ErrInvalidLoyaltyPoints
	}

	// Mirror to pos-api (loyalty SoT) — best-effort, does not block the local write.
	s.mirrorRedeemToPOS(ctx, tenantID, customerPhone, points, orderID)

	account, err := s.repo.GetLoyaltyAccountByUser(ctx, tenantID, userID)
	if err != nil {
		return ErrLoyaltyAccountNotFound
	}

	if account.BalancePoints < points {
		return ErrInsufficientLoyaltyPoints
	}

	// Deduct points
	if err := s.repo.DeductLoyaltyPoints(ctx, account.ID, points); err != nil {
		return err
	}

	// Record transaction
	tx := &LoyaltyTransaction{
		AccountID:       account.ID,
		OrderID:         orderID,
		Points:          -points, // Negative for redemption
		TransactionType: LoyaltyTransactionTypeRedeem,
		Description:     description,
		OccurredAt:      time.Now(),
	}

	if err := s.repo.CreateLoyaltyTransaction(ctx, tx); err != nil {
		s.logger.Error("failed to record loyalty transaction", zap.Error(err))
	}

	s.logger.Info("loyalty points redeemed",
		zap.String("userId", userID.String()),
		zap.Int("points", points))

	return nil
}

// AdjustPoints manually adjusts a user's loyalty points (for admin use).
func (s *LoyaltyService) AdjustPoints(ctx context.Context, tenantID, userID uuid.UUID, points int, description string) error {
	account, err := s.GetOrCreateAccount(ctx, tenantID, userID)
	if err != nil {
		return err
	}

	if points > 0 {
		if err := s.repo.AddLoyaltyPoints(ctx, account.ID, points); err != nil {
			return err
		}
	} else if points < 0 {
		if account.BalancePoints < -points {
			return ErrInsufficientLoyaltyPoints
		}
		if err := s.repo.DeductLoyaltyPoints(ctx, account.ID, -points); err != nil {
			return err
		}
	}

	// Record transaction
	tx := &LoyaltyTransaction{
		AccountID:       account.ID,
		Points:          points,
		TransactionType: LoyaltyTransactionTypeAdjustment,
		Description:     description,
		OccurredAt:      time.Now(),
	}

	if err := s.repo.CreateLoyaltyTransaction(ctx, tx); err != nil {
		s.logger.Error("failed to record loyalty transaction", zap.Error(err))
	}

	s.logger.Info("loyalty points adjusted",
		zap.String("userId", userID.String()),
		zap.Int("points", points))

	return nil
}

// AddBonusPoints adds bonus points to a user's account.
func (s *LoyaltyService) AddBonusPoints(ctx context.Context, tenantID, userID uuid.UUID, points int, description string) error {
	if points <= 0 {
		return ErrInvalidLoyaltyPoints
	}

	account, err := s.GetOrCreateAccount(ctx, tenantID, userID)
	if err != nil {
		return err
	}

	if err := s.repo.AddLoyaltyPoints(ctx, account.ID, points); err != nil {
		return err
	}

	// Record transaction
	tx := &LoyaltyTransaction{
		AccountID:       account.ID,
		Points:          points,
		TransactionType: LoyaltyTransactionTypeBonus,
		Description:     description,
		OccurredAt:      time.Now(),
	}

	if err := s.repo.CreateLoyaltyTransaction(ctx, tx); err != nil {
		s.logger.Error("failed to record loyalty transaction", zap.Error(err))
	}

	s.logger.Info("bonus loyalty points added",
		zap.String("userId", userID.String()),
		zap.Int("points", points))

	return nil
}

// GetTransactions retrieves loyalty transactions for a user.
func (s *LoyaltyService) GetTransactions(ctx context.Context, tenantID, userID uuid.UUID, limit, offset int) ([]LoyaltyTransaction, int, error) {
	account, err := s.repo.GetLoyaltyAccountByUser(ctx, tenantID, userID)
	if err != nil {
		return nil, 0, ErrLoyaltyAccountNotFound
	}

	return s.repo.ListLoyaltyTransactions(ctx, account.ID, limit, offset)
}

// CalculatePointsValue calculates the monetary value of loyalty points.
func (s *LoyaltyService) CalculatePointsValue(points int) float64 {
	return float64(points) * LoyaltyPointValue
}

// CalculatePointsForAmount calculates the points earned for a given amount,
// applying the base rate of 1 point per KES 100.
func (s *LoyaltyService) CalculatePointsForAmount(amount float64) int {
	return int(amount / 100 * float64(LoyaltyPointsPerHundred))
}

// CalculatePointsForAmountWithTier calculates points earned for a given amount,
// applying the tier-specific multiplier.
// Tiers: Bronze 1x, Silver 1.2x, Gold 1.5x, Platinum 2x per KES 100.
func (s *LoyaltyService) CalculatePointsForAmountWithTier(amount float64, tier LoyaltyTier) int {
	basePoints := amount / 100 * float64(LoyaltyPointsPerHundred)
	multiplier := 1.0
	switch tier {
	case LoyaltyTierPlatinum:
		multiplier = 2.0
	case LoyaltyTierGold:
		multiplier = 1.5
	case LoyaltyTierSilver:
		multiplier = 1.2
	default: // Bronze
		multiplier = 1.0
	}
	return int(basePoints * multiplier)
}

// calculateTier determines the loyalty tier based on lifetime points.
// Tiers aligned with cafe-website:
//   - Bronze:   0-500 points
//   - Silver:   501-2000 points
//   - Gold:     2001-5000 points
//   - Platinum: 5000+ points
func (s *LoyaltyService) calculateTier(lifetimePoints int) LoyaltyTier {
	switch {
	case lifetimePoints > 5000:
		return LoyaltyTierPlatinum
	case lifetimePoints > 2000:
		return LoyaltyTierGold
	case lifetimePoints > 500:
		return LoyaltyTierSilver
	default:
		return LoyaltyTierBronze
	}
}

// GetTierBenefits returns the benefits for a loyalty tier.
func (s *LoyaltyService) GetTierBenefits(tier LoyaltyTier) map[string]interface{} {
	benefits := make(map[string]interface{})

	switch tier {
	case LoyaltyTierPlatinum:
		benefits["pointsMultiplier"] = 2.0
		benefits["freeDelivery"] = true
		benefits["prioritySupport"] = true
		benefits["exclusiveOffers"] = true
	case LoyaltyTierGold:
		benefits["pointsMultiplier"] = 1.5
		benefits["freeDelivery"] = true
		benefits["prioritySupport"] = false
		benefits["exclusiveOffers"] = true
	case LoyaltyTierSilver:
		benefits["pointsMultiplier"] = 1.2
		benefits["freeDelivery"] = false
		benefits["prioritySupport"] = false
		benefits["exclusiveOffers"] = false
	default: // Bronze
		benefits["pointsMultiplier"] = 1.0
		benefits["freeDelivery"] = false
		benefits["prioritySupport"] = false
		benefits["exclusiveOffers"] = false
	}

	return benefits
}
