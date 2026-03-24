package ordering

import (
	"context"
	"crypto/rand"
	"math/big"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// GroupOrderExpirationDuration is how long a group order session stays open.
const GroupOrderExpirationDuration = 2 * time.Hour

// GroupOrderService provides group ordering business logic.
type GroupOrderService struct {
	repo   Repository
	logger *zap.Logger
}

// NewGroupOrderService creates a new group order service.
func NewGroupOrderService(repo Repository, logger *zap.Logger) *GroupOrderService {
	return &GroupOrderService{
		repo:   repo,
		logger: logger.Named("ordering.GroupOrderService"),
	}
}

// CreateGroupOrder creates a new group ordering session.
func (s *GroupOrderService) CreateGroupOrder(ctx context.Context, tenantID, hostUserID, cartID uuid.UUID) (*GroupOrder, error) {
	inviteCode, err := generateInviteCode(6)
	if err != nil {
		s.logger.Error("failed to generate invite code", zap.Error(err))
		return nil, ErrInternalError
	}

	g := &GroupOrder{
		TenantID:        tenantID,
		HostUserID:      hostUserID,
		CartID:          cartID,
		InviteCode:      inviteCode,
		Status:          GroupOrderStatusOpen,
		MaxParticipants: 10,
		ExpiresAt:       time.Now().Add(GroupOrderExpirationDuration),
	}

	if err := s.repo.CreateGroupOrder(ctx, g); err != nil {
		s.logger.Error("failed to create group order", zap.Error(err))
		return nil, err
	}

	// Add host as first participant
	participant := &GroupParticipant{
		GroupOrderID: g.ID,
		UserID:       hostUserID,
		UserName:     "Host",
	}
	if err := s.repo.CreateGroupParticipant(ctx, participant); err != nil {
		s.logger.Error("failed to add host as participant", zap.Error(err))
	}

	s.logger.Info("group order created",
		zap.String("id", g.ID.String()),
		zap.String("invite_code", g.InviteCode))

	return s.repo.GetGroupOrder(ctx, g.ID)
}

// JoinGroupOrder adds a participant to a group order by invite code.
func (s *GroupOrderService) JoinGroupOrder(ctx context.Context, inviteCode string, userID uuid.UUID, userName string) (*GroupOrder, error) {
	g, err := s.repo.GetGroupOrderByInviteCode(ctx, inviteCode)
	if err != nil {
		return nil, ErrInvalidInviteCode
	}

	// Check expiry
	if time.Now().After(g.ExpiresAt) {
		g.Status = GroupOrderStatusExpired
		_ = s.repo.UpdateGroupOrder(ctx, g)
		return nil, ErrGroupOrderExpired
	}

	// Check status
	if g.Status != GroupOrderStatusOpen {
		return nil, ErrGroupOrderNotOpen
	}

	// Check if already a participant
	exists, err := s.repo.IsGroupParticipant(ctx, g.ID, userID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrAlreadyParticipant
	}

	// Check max participants
	count, err := s.repo.CountGroupParticipants(ctx, g.ID)
	if err != nil {
		return nil, err
	}
	if count >= g.MaxParticipants {
		return nil, ErrGroupOrderFull
	}

	participant := &GroupParticipant{
		GroupOrderID: g.ID,
		UserID:       userID,
		UserName:     userName,
	}
	if err := s.repo.CreateGroupParticipant(ctx, participant); err != nil {
		s.logger.Error("failed to add participant", zap.Error(err))
		return nil, err
	}

	s.logger.Info("user joined group order",
		zap.String("group_order_id", g.ID.String()),
		zap.String("user_id", userID.String()))

	return s.repo.GetGroupOrder(ctx, g.ID)
}

// LockGroupOrder locks a group order for checkout (host only).
func (s *GroupOrderService) LockGroupOrder(ctx context.Context, groupOrderID, hostUserID uuid.UUID) (*GroupOrder, error) {
	g, err := s.repo.GetGroupOrder(ctx, groupOrderID)
	if err != nil {
		return nil, err
	}

	if g.HostUserID != hostUserID {
		return nil, ErrNotGroupOrderHost
	}

	if g.Status != GroupOrderStatusOpen {
		return nil, ErrGroupOrderNotOpen
	}

	g.Status = GroupOrderStatusLocked
	if err := s.repo.UpdateGroupOrder(ctx, g); err != nil {
		return nil, err
	}

	s.logger.Info("group order locked",
		zap.String("group_order_id", g.ID.String()))

	return s.repo.GetGroupOrder(ctx, groupOrderID)
}

// GetGroupOrder retrieves a group order by ID.
func (s *GroupOrderService) GetGroupOrder(ctx context.Context, groupOrderID uuid.UUID) (*GroupOrder, error) {
	return s.repo.GetGroupOrder(ctx, groupOrderID)
}

// generateInviteCode generates a random alphanumeric code of the given length.
func generateInviteCode(length int) (string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // avoid ambiguous chars
	code := make([]byte, length)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		code[i] = charset[n.Int64()]
	}
	return string(code), nil
}
