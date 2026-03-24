package ordering

import (
	"context"
	"time"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/grouporder"
	"github.com/bengobox/ordering-backend/internal/ent/groupparticipant"
	"github.com/bengobox/ordering-backend/internal/ent/promocode"
	"github.com/google/uuid"
)

// --- GroupOrder Methods ---

func (r *EntRepository) CreateGroupOrder(ctx context.Context, g *GroupOrder) error {
	created, err := r.client.GroupOrder.Create().
		SetTenantID(g.TenantID).
		SetHostUserID(g.HostUserID).
		SetCartID(g.CartID).
		SetInviteCode(g.InviteCode).
		SetStatus(grouporder.Status(g.Status)).
		SetMaxParticipants(g.MaxParticipants).
		SetExpiresAt(g.ExpiresAt).
		Save(ctx)
	if err != nil {
		return err
	}

	g.ID = created.ID
	g.CreatedAt = created.CreatedAt
	g.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetGroupOrder(ctx context.Context, id uuid.UUID) (*GroupOrder, error) {
	g, err := r.client.GroupOrder.Query().
		Where(grouporder.ID(id)).
		WithParticipants().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrGroupOrderNotFound
		}
		return nil, err
	}
	return entGroupOrderToDomain(g), nil
}

func (r *EntRepository) GetGroupOrderByInviteCode(ctx context.Context, code string) (*GroupOrder, error) {
	g, err := r.client.GroupOrder.Query().
		Where(grouporder.InviteCode(code)).
		WithParticipants().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrGroupOrderNotFound
		}
		return nil, err
	}
	return entGroupOrderToDomain(g), nil
}

func (r *EntRepository) UpdateGroupOrder(ctx context.Context, g *GroupOrder) error {
	updated, err := r.client.GroupOrder.UpdateOneID(g.ID).
		SetStatus(grouporder.Status(g.Status)).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrGroupOrderNotFound
		}
		return err
	}

	g.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *EntRepository) CreateGroupParticipant(ctx context.Context, p *GroupParticipant) error {
	created, err := r.client.GroupParticipant.Create().
		SetGroupOrderID(p.GroupOrderID).
		SetUserID(p.UserID).
		SetUserName(p.UserName).
		Save(ctx)
	if err != nil {
		return err
	}

	p.ID = created.ID
	p.JoinedAt = created.JoinedAt
	return nil
}

func (r *EntRepository) ListGroupParticipants(ctx context.Context, groupOrderID uuid.UUID) ([]GroupParticipant, error) {
	participants, err := r.client.GroupParticipant.Query().
		Where(groupparticipant.GroupOrderID(groupOrderID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]GroupParticipant, len(participants))
	for i, p := range participants {
		result[i] = GroupParticipant{
			ID:           p.ID,
			GroupOrderID: p.GroupOrderID,
			UserID:       p.UserID,
			UserName:     p.UserName,
			JoinedAt:     p.JoinedAt,
		}
	}
	return result, nil
}

func (r *EntRepository) CountGroupParticipants(ctx context.Context, groupOrderID uuid.UUID) (int, error) {
	return r.client.GroupParticipant.Query().
		Where(groupparticipant.GroupOrderID(groupOrderID)).
		Count(ctx)
}

func (r *EntRepository) IsGroupParticipant(ctx context.Context, groupOrderID, userID uuid.UUID) (bool, error) {
	return r.client.GroupParticipant.Query().
		Where(
			groupparticipant.GroupOrderID(groupOrderID),
			groupparticipant.UserID(userID),
		).
		Exist(ctx)
}

// --- Active Promo Codes for Outlet ---

func (r *EntRepository) ListActivePromoCodesForOutlet(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID) ([]PromoCode, error) {
	now := time.Now()
	query := r.client.PromoCode.Query().
		Where(
			promocode.TenantID(tenantID),
			promocode.IsActive(true),
			promocode.Or(
				promocode.StartsAtIsNil(),
				promocode.StartsAtLTE(now),
			),
			promocode.Or(
				promocode.EndsAtIsNil(),
				promocode.EndsAtGTE(now),
			),
		)

	if outletID != nil {
		query = query.Where(
			promocode.Or(
				promocode.OutletIDIsNil(),
				promocode.OutletID(*outletID),
			),
		)
	}

	promos, err := query.All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]PromoCode, len(promos))
	for i, p := range promos {
		result[i] = *entPromoCodeToDomain(p)
	}
	return result, nil
}

// --- Domain Conversion Functions ---

func entGroupOrderToDomain(g *ent.GroupOrder) *GroupOrder {
	result := &GroupOrder{
		ID:              g.ID,
		TenantID:        g.TenantID,
		HostUserID:      g.HostUserID,
		CartID:          g.CartID,
		InviteCode:      g.InviteCode,
		Status:          GroupOrderStatus(g.Status),
		MaxParticipants: g.MaxParticipants,
		ExpiresAt:       g.ExpiresAt,
		CreatedAt:       g.CreatedAt,
		UpdatedAt:       g.UpdatedAt,
	}

	if g.Edges.Participants != nil {
		result.Participants = make([]GroupParticipant, len(g.Edges.Participants))
		for i, p := range g.Edges.Participants {
			result.Participants[i] = GroupParticipant{
				ID:           p.ID,
				GroupOrderID: p.GroupOrderID,
				UserID:       p.UserID,
				UserName:     p.UserName,
				JoinedAt:     p.JoinedAt,
			}
		}
	}

	return result
}
