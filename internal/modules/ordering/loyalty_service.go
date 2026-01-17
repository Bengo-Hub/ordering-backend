package ordering

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// LoyaltyService provides loyalty program business logic.
type LoyaltyService struct {
	repo   Repository
	logger *zap.Logger
}

// NewLoyaltyService creates a new loyalty service.
func NewLoyaltyService(repo Repository, logger *zap.Logger) *LoyaltyService {
	return &LoyaltyService{
		repo:   repo,
		logger: logger,
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
func (s *LoyaltyService) EarnPoints(ctx context.Context, tenantID, userID uuid.UUID, points int, orderID *uuid.UUID, description string) error {
	if points <= 0 {
		return ErrInvalidLoyaltyPoints
	}

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
func (s *LoyaltyService) RedeemPoints(ctx context.Context, tenantID, userID uuid.UUID, points int, orderID *uuid.UUID, description string) error {
	if points <= 0 {
		return ErrInvalidLoyaltyPoints
	}

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

// CalculatePointsForAmount calculates the points earned for a given amount.
func (s *LoyaltyService) CalculatePointsForAmount(amount float64) int {
	return int(amount * float64(LoyaltyPointsPerUnit))
}

// calculateTier determines the loyalty tier based on lifetime points.
func (s *LoyaltyService) calculateTier(lifetimePoints int) LoyaltyTier {
	switch {
	case lifetimePoints >= 10000:
		return LoyaltyTierPlatinum
	case lifetimePoints >= 5000:
		return LoyaltyTierGold
	case lifetimePoints >= 1000:
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
		benefits["pointsMultiplier"] = 1.25
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
