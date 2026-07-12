package ordering

import (
	"context"
	"fmt"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/customeraddress"
	"github.com/bengobox/ordering-backend/internal/ent/loyaltyaccount"
	"github.com/bengobox/ordering-backend/internal/ent/loyaltytransaction"
	"github.com/bengobox/ordering-backend/internal/ent/outlet"
	"github.com/bengobox/ordering-backend/internal/ent/promocode"
	"github.com/bengobox/ordering-backend/internal/ent/promoredemption"
	"github.com/google/uuid"
)

// --- Outlet Methods ---

func (r *EntRepository) GetOutletLocation(ctx context.Context, tenantID, outletID uuid.UUID) (name string, lat, lng *float64, err error) {
	o, err := r.client.Outlet.Query().
		Where(
			outlet.ID(outletID),
			outlet.TenantID(tenantID),
		).
		Select(outlet.FieldName, outlet.FieldLatitude, outlet.FieldLongitude).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", nil, nil, nil
		}
		return "", nil, nil, err
	}
	return o.Name, o.Latitude, o.Longitude, nil
}

// GetOutletBookingDepositPercent returns the outlet's booking deposit % (0 when the outlet
// isn't found, so a missing outlet degrades to pay-in-full rather than erroring checkout).
func (r *EntRepository) GetOutletBookingDepositPercent(ctx context.Context, tenantID, outletID uuid.UUID) (int, error) {
	o, err := r.client.Outlet.Query().
		Where(outlet.ID(outletID), outlet.TenantID(tenantID)).
		Select(outlet.FieldBookingDepositPercent).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	return o.BookingDepositPercent, nil
}

// SetOutletBookingDepositPercent updates the outlet's booking deposit % (0-100; caller validates).
func (r *EntRepository) SetOutletBookingDepositPercent(ctx context.Context, tenantID, outletID uuid.UUID, percent int) error {
	n, err := r.client.Outlet.Update().
		Where(outlet.ID(outletID), outlet.TenantID(tenantID)).
		SetBookingDepositPercent(percent).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("outlet %s not found for tenant %s", outletID, tenantID)
	}
	return nil
}

// --- CustomerAddress Methods ---

func (r *EntRepository) CreateAddress(ctx context.Context, address *CustomerAddress) error {
	builder := r.client.CustomerAddress.Create().
		SetTenantID(address.TenantID).
		SetUserID(address.UserID).
		SetLabel(address.Label).
		SetAddressLine1(address.AddressLine1).
		SetAddressLine2(address.AddressLine2).
		SetCity(address.City).
		SetCounty(address.County).
		SetPostalCode(address.PostalCode).
		SetCountry(address.Country).
		SetPlusCode(address.PlusCode).
		SetContactName(address.ContactName).
		SetContactPhone(address.ContactPhone).
		SetInstructions(address.Instructions).
		SetIsDefault(address.IsDefault).
		SetIsVerified(address.IsVerified)

	if address.Latitude != nil {
		builder.SetLatitude(*address.Latitude)
	}
	if address.Longitude != nil {
		builder.SetLongitude(*address.Longitude)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	address.ID = created.ID
	address.CreatedAt = created.CreatedAt
	address.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetAddress(ctx context.Context, tenantID, addressID uuid.UUID) (*CustomerAddress, error) {
	addr, err := r.client.CustomerAddress.Query().
		Where(
			customeraddress.ID(addressID),
			customeraddress.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrAddressNotFound
		}
		return nil, err
	}
	return entAddressToDomain(addr), nil
}

func (r *EntRepository) GetDefaultAddress(ctx context.Context, tenantID, userID uuid.UUID) (*CustomerAddress, error) {
	addr, err := r.client.CustomerAddress.Query().
		Where(
			customeraddress.TenantID(tenantID),
			customeraddress.UserID(userID),
			customeraddress.IsDefault(true),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrAddressNotFound
		}
		return nil, err
	}
	return entAddressToDomain(addr), nil
}

func (r *EntRepository) UpdateAddress(ctx context.Context, address *CustomerAddress) error {
	builder := r.client.CustomerAddress.UpdateOneID(address.ID).
		SetLabel(address.Label).
		SetAddressLine1(address.AddressLine1).
		SetAddressLine2(address.AddressLine2).
		SetCity(address.City).
		SetCounty(address.County).
		SetPostalCode(address.PostalCode).
		SetCountry(address.Country).
		SetPlusCode(address.PlusCode).
		SetContactName(address.ContactName).
		SetContactPhone(address.ContactPhone).
		SetInstructions(address.Instructions).
		SetIsDefault(address.IsDefault).
		SetIsVerified(address.IsVerified)

	if address.Latitude != nil {
		builder.SetLatitude(*address.Latitude)
	} else {
		builder.ClearLatitude()
	}
	if address.Longitude != nil {
		builder.SetLongitude(*address.Longitude)
	} else {
		builder.ClearLongitude()
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrAddressNotFound
		}
		return err
	}

	address.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *EntRepository) DeleteAddress(ctx context.Context, tenantID, addressID uuid.UUID) error {
	_, err := r.client.CustomerAddress.Delete().
		Where(
			customeraddress.ID(addressID),
			customeraddress.TenantID(tenantID),
		).Exec(ctx)
	return err
}

func (r *EntRepository) ListAddresses(ctx context.Context, tenantID, userID uuid.UUID) ([]CustomerAddress, error) {
	addrs, err := r.client.CustomerAddress.Query().
		Where(
			customeraddress.TenantID(tenantID),
			customeraddress.UserID(userID),
		).
		Order(ent.Desc(customeraddress.FieldIsDefault), ent.Desc(customeraddress.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]CustomerAddress, len(addrs))
	for i, addr := range addrs {
		result[i] = *entAddressToDomain(addr)
	}
	return result, nil
}

func (r *EntRepository) SetDefaultAddress(ctx context.Context, tenantID, userID, addressID uuid.UUID) error {
	// Clear existing default
	_, err := r.client.CustomerAddress.Update().
		Where(
			customeraddress.TenantID(tenantID),
			customeraddress.UserID(userID),
			customeraddress.IsDefault(true),
		).
		SetIsDefault(false).
		Save(ctx)
	if err != nil {
		return err
	}

	// Set new default
	_, err = r.client.CustomerAddress.UpdateOneID(addressID).
		SetIsDefault(true).
		Save(ctx)
	return err
}

func (r *EntRepository) CountUserAddresses(ctx context.Context, tenantID, userID uuid.UUID) (int, error) {
	return r.client.CustomerAddress.Query().
		Where(
			customeraddress.TenantID(tenantID),
			customeraddress.UserID(userID),
		).
		Count(ctx)
}

// --- PromoCode Methods ---

func (r *EntRepository) CreatePromoCode(ctx context.Context, promo *PromoCode) error {
	builder := r.client.PromoCode.Create().
		SetTenantID(promo.TenantID).
		SetCode(promo.Code).
		SetName(promo.Name).
		SetDescription(promo.Description).
		SetDiscountType(promocode.DiscountType(promo.DiscountType)).
		SetDiscountValue(promo.DiscountValue).
		SetMinSubtotal(promo.MinSubtotal).
		SetUsageCount(promo.UsageCount).
		SetIsActive(promo.IsActive).
		SetFirstOrderOnly(promo.FirstOrderOnly)

	if promo.OutletID != nil {
		builder.SetOutletID(*promo.OutletID)
	}
	if promo.MaxDiscountAmount != nil {
		builder.SetMaxDiscountAmount(*promo.MaxDiscountAmount)
	}
	if promo.MaxUses != nil {
		builder.SetMaxUses(*promo.MaxUses)
	}
	if promo.MaxUsesPerUser != nil {
		builder.SetMaxUsesPerUser(*promo.MaxUsesPerUser)
	}
	if promo.StartsAt != nil {
		builder.SetStartsAt(*promo.StartsAt)
	}
	if promo.EndsAt != nil {
		builder.SetEndsAt(*promo.EndsAt)
	}
	if len(promo.EligibleCategories) > 0 {
		builder.SetEligibleCategories(promo.EligibleCategories)
	}
	if len(promo.EligibleItems) > 0 {
		builder.SetEligibleItems(promo.EligibleItems)
	}
	if promo.Metadata != nil {
		builder.SetMetadata(promo.Metadata)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	promo.ID = created.ID
	promo.CreatedAt = created.CreatedAt
	promo.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetPromoCode(ctx context.Context, tenantID, promoID uuid.UUID) (*PromoCode, error) {
	p, err := r.client.PromoCode.Query().
		Where(
			promocode.ID(promoID),
			promocode.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrPromoCodeNotFound
		}
		return nil, err
	}
	return entPromoCodeToDomain(p), nil
}

func (r *EntRepository) GetPromoCodeByCode(ctx context.Context, tenantID uuid.UUID, code string) (*PromoCode, error) {
	p, err := r.client.PromoCode.Query().
		Where(
			promocode.TenantID(tenantID),
			promocode.CodeEqualFold(code),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrPromoCodeNotFound
		}
		return nil, err
	}
	return entPromoCodeToDomain(p), nil
}

func (r *EntRepository) UpdatePromoCode(ctx context.Context, promo *PromoCode) error {
	builder := r.client.PromoCode.UpdateOneID(promo.ID).
		SetCode(promo.Code).
		SetName(promo.Name).
		SetDescription(promo.Description).
		SetDiscountType(promocode.DiscountType(promo.DiscountType)).
		SetDiscountValue(promo.DiscountValue).
		SetMinSubtotal(promo.MinSubtotal).
		SetUsageCount(promo.UsageCount).
		SetIsActive(promo.IsActive).
		SetFirstOrderOnly(promo.FirstOrderOnly)

	if promo.OutletID != nil {
		builder.SetOutletID(*promo.OutletID)
	} else {
		builder.ClearOutletID()
	}
	if promo.MaxDiscountAmount != nil {
		builder.SetMaxDiscountAmount(*promo.MaxDiscountAmount)
	} else {
		builder.ClearMaxDiscountAmount()
	}
	if promo.MaxUses != nil {
		builder.SetMaxUses(*promo.MaxUses)
	} else {
		builder.ClearMaxUses()
	}
	if promo.MaxUsesPerUser != nil {
		builder.SetMaxUsesPerUser(*promo.MaxUsesPerUser)
	} else {
		builder.ClearMaxUsesPerUser()
	}
	if promo.StartsAt != nil {
		builder.SetStartsAt(*promo.StartsAt)
	} else {
		builder.ClearStartsAt()
	}
	if promo.EndsAt != nil {
		builder.SetEndsAt(*promo.EndsAt)
	} else {
		builder.ClearEndsAt()
	}
	if len(promo.EligibleCategories) > 0 {
		builder.SetEligibleCategories(promo.EligibleCategories)
	} else {
		builder.ClearEligibleCategories()
	}
	if len(promo.EligibleItems) > 0 {
		builder.SetEligibleItems(promo.EligibleItems)
	} else {
		builder.ClearEligibleItems()
	}
	if promo.Metadata != nil {
		builder.SetMetadata(promo.Metadata)
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrPromoCodeNotFound
		}
		return err
	}

	promo.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *EntRepository) DeletePromoCode(ctx context.Context, tenantID, promoID uuid.UUID) error {
	_, err := r.client.PromoCode.Delete().
		Where(
			promocode.ID(promoID),
			promocode.TenantID(tenantID),
		).Exec(ctx)
	return err
}

func (r *EntRepository) ListPromoCodes(ctx context.Context, tenantID uuid.UUID, isActive *bool) ([]PromoCode, error) {
	query := r.client.PromoCode.Query().
		Where(promocode.TenantID(tenantID))

	if isActive != nil {
		query = query.Where(promocode.IsActive(*isActive))
	}

	promos, err := query.Order(ent.Desc(promocode.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]PromoCode, len(promos))
	for i, p := range promos {
		result[i] = *entPromoCodeToDomain(p)
	}
	return result, nil
}

func (r *EntRepository) IncrementPromoUsage(ctx context.Context, promoID uuid.UUID) error {
	_, err := r.client.PromoCode.UpdateOneID(promoID).
		AddUsageCount(1).
		Save(ctx)
	return err
}

// --- PromoRedemption Methods ---

func (r *EntRepository) CreatePromoRedemption(ctx context.Context, redemption *PromoRedemption) error {
	created, err := r.client.PromoRedemption.Create().
		SetPromoCodeID(redemption.PromoCodeID).
		SetOrderID(redemption.OrderID).
		SetUserID(redemption.UserID).
		SetDiscountAmount(redemption.DiscountAmount).
		SetRedeemedAt(redemption.RedeemedAt).
		Save(ctx)
	if err != nil {
		return err
	}

	redemption.ID = created.ID
	return nil
}

func (r *EntRepository) GetPromoRedemption(ctx context.Context, redemptionID uuid.UUID) (*PromoRedemption, error) {
	red, err := r.client.PromoRedemption.Get(ctx, redemptionID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrPromoCodeNotFound
		}
		return nil, err
	}
	return entPromoRedemptionToDomain(red), nil
}

func (r *EntRepository) CountUserPromoRedemptions(ctx context.Context, promoID, userID uuid.UUID) (int, error) {
	return r.client.PromoRedemption.Query().
		Where(
			promoredemption.PromoCodeID(promoID),
			promoredemption.UserID(userID),
		).
		Count(ctx)
}

func (r *EntRepository) ListPromoRedemptions(ctx context.Context, promoID uuid.UUID) ([]PromoRedemption, error) {
	reds, err := r.client.PromoRedemption.Query().
		Where(promoredemption.PromoCodeID(promoID)).
		Order(ent.Desc(promoredemption.FieldRedeemedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]PromoRedemption, len(reds))
	for i, red := range reds {
		result[i] = *entPromoRedemptionToDomain(red)
	}
	return result, nil
}

// --- LoyaltyAccount Methods ---

func (r *EntRepository) CreateLoyaltyAccount(ctx context.Context, account *LoyaltyAccount) error {
	created, err := r.client.LoyaltyAccount.Create().
		SetTenantID(account.TenantID).
		SetUserID(account.UserID).
		SetBalancePoints(account.BalancePoints).
		SetTier(loyaltyaccount.Tier(account.Tier)).
		SetLifetimePoints(account.LifetimePoints).
		Save(ctx)
	if err != nil {
		return err
	}

	account.ID = created.ID
	account.CreatedAt = created.CreatedAt
	account.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetLoyaltyAccount(ctx context.Context, tenantID, accountID uuid.UUID) (*LoyaltyAccount, error) {
	acc, err := r.client.LoyaltyAccount.Query().
		Where(
			loyaltyaccount.ID(accountID),
			loyaltyaccount.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrLoyaltyAccountNotFound
		}
		return nil, err
	}
	return entLoyaltyAccountToDomain(acc), nil
}

func (r *EntRepository) GetLoyaltyAccountByUser(ctx context.Context, tenantID, userID uuid.UUID) (*LoyaltyAccount, error) {
	acc, err := r.client.LoyaltyAccount.Query().
		Where(
			loyaltyaccount.TenantID(tenantID),
			loyaltyaccount.UserID(userID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrLoyaltyAccountNotFound
		}
		return nil, err
	}
	return entLoyaltyAccountToDomain(acc), nil
}

func (r *EntRepository) UpdateLoyaltyAccount(ctx context.Context, account *LoyaltyAccount) error {
	updated, err := r.client.LoyaltyAccount.UpdateOneID(account.ID).
		SetBalancePoints(account.BalancePoints).
		SetTier(loyaltyaccount.Tier(account.Tier)).
		SetLifetimePoints(account.LifetimePoints).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrLoyaltyAccountNotFound
		}
		return err
	}

	account.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *EntRepository) AddLoyaltyPoints(ctx context.Context, accountID uuid.UUID, points int) error {
	_, err := r.client.LoyaltyAccount.UpdateOneID(accountID).
		AddBalancePoints(points).
		AddLifetimePoints(points).
		Save(ctx)
	return err
}

func (r *EntRepository) DeductLoyaltyPoints(ctx context.Context, accountID uuid.UUID, points int) error {
	_, err := r.client.LoyaltyAccount.UpdateOneID(accountID).
		AddBalancePoints(-points).
		Save(ctx)
	return err
}

// --- LoyaltyTransaction Methods ---

func (r *EntRepository) CreateLoyaltyTransaction(ctx context.Context, tx *LoyaltyTransaction) error {
	builder := r.client.LoyaltyTransaction.Create().
		SetAccountID(tx.AccountID).
		SetPoints(tx.Points).
		SetTransactionType(loyaltytransaction.TransactionType(tx.TransactionType)).
		SetDescription(tx.Description).
		SetOccurredAt(tx.OccurredAt)

	if tx.OrderID != nil {
		builder.SetOrderID(*tx.OrderID)
	}
	if tx.Metadata != nil {
		builder.SetMetadata(tx.Metadata)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	tx.ID = created.ID
	return nil
}

func (r *EntRepository) ListLoyaltyTransactions(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]LoyaltyTransaction, int, error) {
	query := r.client.LoyaltyTransaction.Query().
		Where(loyaltytransaction.AccountID(accountID))

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	query = query.Order(ent.Desc(loyaltytransaction.FieldOccurredAt))
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	txs, err := query.All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]LoyaltyTransaction, len(txs))
	for i, tx := range txs {
		result[i] = *entLoyaltyTransactionToDomain(tx)
	}
	return result, total, nil
}

// --- Domain Conversion Functions ---

func entAddressToDomain(addr *ent.CustomerAddress) *CustomerAddress {
	address := &CustomerAddress{
		ID:           addr.ID,
		TenantID:     addr.TenantID,
		UserID:       addr.UserID,
		Label:        addr.Label,
		AddressLine1: addr.AddressLine1,
		AddressLine2: addr.AddressLine2,
		City:         addr.City,
		County:       addr.County,
		PostalCode:   addr.PostalCode,
		Country:      addr.Country,
		PlusCode:     addr.PlusCode,
		ContactName:  addr.ContactName,
		ContactPhone: addr.ContactPhone,
		Instructions: addr.Instructions,
		IsDefault:    addr.IsDefault,
		IsVerified:   addr.IsVerified,
		CreatedAt:    addr.CreatedAt,
		UpdatedAt:    addr.UpdatedAt,
	}

	if addr.Latitude != nil {
		address.Latitude = addr.Latitude
	}
	if addr.Longitude != nil {
		address.Longitude = addr.Longitude
	}

	return address
}

func entPromoCodeToDomain(p *ent.PromoCode) *PromoCode {
	promo := &PromoCode{
		ID:             p.ID,
		TenantID:       p.TenantID,
		Code:           p.Code,
		Name:           p.Name,
		Description:    p.Description,
		DiscountType:   PromoCodeType(p.DiscountType),
		DiscountValue:  p.DiscountValue,
		MinSubtotal:    p.MinSubtotal,
		UsageCount:     p.UsageCount,
		IsActive:       p.IsActive,
		FirstOrderOnly: p.FirstOrderOnly,
		Metadata:       p.Metadata,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}

	if p.OutletID != nil {
		promo.OutletID = p.OutletID
	}
	if p.MaxDiscountAmount != nil {
		promo.MaxDiscountAmount = p.MaxDiscountAmount
	}
	if p.MaxUses != nil {
		promo.MaxUses = p.MaxUses
	}
	if p.MaxUsesPerUser != nil {
		promo.MaxUsesPerUser = p.MaxUsesPerUser
	}
	if p.StartsAt != nil {
		promo.StartsAt = p.StartsAt
	}
	if p.EndsAt != nil {
		promo.EndsAt = p.EndsAt
	}
	if len(p.EligibleCategories) > 0 {
		promo.EligibleCategories = p.EligibleCategories
	}
	if len(p.EligibleItems) > 0 {
		promo.EligibleItems = p.EligibleItems
	}

	return promo
}

func entPromoRedemptionToDomain(r *ent.PromoRedemption) *PromoRedemption {
	return &PromoRedemption{
		ID:             r.ID,
		PromoCodeID:    r.PromoCodeID,
		OrderID:        r.OrderID,
		UserID:         r.UserID,
		DiscountAmount: r.DiscountAmount,
		RedeemedAt:     r.RedeemedAt,
	}
}

func entLoyaltyAccountToDomain(acc *ent.LoyaltyAccount) *LoyaltyAccount {
	return &LoyaltyAccount{
		ID:             acc.ID,
		TenantID:       acc.TenantID,
		UserID:         acc.UserID,
		BalancePoints:  acc.BalancePoints,
		Tier:           LoyaltyTier(acc.Tier),
		LifetimePoints: acc.LifetimePoints,
		CreatedAt:      acc.CreatedAt,
		UpdatedAt:      acc.UpdatedAt,
	}
}

func entLoyaltyTransactionToDomain(tx *ent.LoyaltyTransaction) *LoyaltyTransaction {
	lt := &LoyaltyTransaction{
		ID:              tx.ID,
		AccountID:       tx.AccountID,
		Points:          tx.Points,
		TransactionType: LoyaltyTransactionType(tx.TransactionType),
		Description:     tx.Description,
		OccurredAt:      tx.OccurredAt,
		Metadata:        tx.Metadata,
	}

	if tx.OrderID != nil {
		lt.OrderID = tx.OrderID
	}

	return lt
}
