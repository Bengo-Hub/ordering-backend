package catalog

import (
	"context"
	"strings"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/dietarytag"
	"github.com/bengobox/ordering-backend/internal/ent/menucategory"
	"github.com/bengobox/ordering-backend/internal/ent/menuitem"
	"github.com/bengobox/ordering-backend/internal/ent/menuitemasset"
	"github.com/bengobox/ordering-backend/internal/ent/menuitemschedule"
	"github.com/bengobox/ordering-backend/internal/ent/menuitemtranslation"
	"github.com/bengobox/ordering-backend/internal/ent/menuitemvariant"
	"github.com/google/uuid"
)

// EntRepository implements Repository using Ent ORM.
type EntRepository struct {
	client *ent.Client
}

// NewEntRepository creates a new Ent-based catalog repository.
func NewEntRepository(client *ent.Client) *EntRepository {
	return &EntRepository{client: client}
}

// --- Category Methods ---

func (r *EntRepository) CreateCategory(ctx context.Context, category *Category) error {
	builder := r.client.MenuCategory.Create().
		SetTenantID(category.TenantID).
		SetCafeID(category.CafeID).
		SetName(category.Name).
		SetDescription(category.Description).
		SetDisplayOrder(category.DisplayOrder).
		SetIsActive(category.IsActive).
		SetImageURL(category.ImageURL)

	if category.ParentID != nil {
		builder.SetParentID(*category.ParentID)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	category.ID = created.ID
	category.CreatedAt = created.CreatedAt
	category.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetCategory(ctx context.Context, tenantID, categoryID uuid.UUID) (*Category, error) {
	cat, err := r.client.MenuCategory.Query().
		Where(
			menucategory.ID(categoryID),
			menucategory.TenantID(tenantID),
		).
		WithChildren().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrCategoryNotFound
		}
		return nil, err
	}
	return entCategoryToDomain(cat), nil
}

func (r *EntRepository) GetCategoryByName(ctx context.Context, tenantID, cafeID uuid.UUID, name string) (*Category, error) {
	cat, err := r.client.MenuCategory.Query().
		Where(
			menucategory.TenantID(tenantID),
			menucategory.CafeID(cafeID),
			menucategory.NameEqualFold(name),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrCategoryNotFound
		}
		return nil, err
	}
	return entCategoryToDomain(cat), nil
}

func (r *EntRepository) ListCategories(ctx context.Context, filter CategoryFilter) ([]Category, int, error) {
	query := r.client.MenuCategory.Query().
		Where(menucategory.TenantID(filter.TenantID))

	if filter.CafeID != nil {
		query = query.Where(menucategory.CafeID(*filter.CafeID))
	}
	if filter.ParentID != nil {
		query = query.Where(menucategory.ParentID(*filter.ParentID))
	}
	if filter.IsActive != nil {
		query = query.Where(menucategory.IsActive(*filter.IsActive))
	}
	if filter.Search != "" {
		query = query.Where(menucategory.NameContainsFold(filter.Search))
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	query = query.Order(ent.Asc(menucategory.FieldDisplayOrder))
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	cats, err := query.WithChildren().All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]Category, len(cats))
	for i, cat := range cats {
		result[i] = *entCategoryToDomain(cat)
	}
	return result, total, nil
}

func (r *EntRepository) UpdateCategory(ctx context.Context, category *Category) error {
	builder := r.client.MenuCategory.UpdateOneID(category.ID).
		SetName(category.Name).
		SetDescription(category.Description).
		SetDisplayOrder(category.DisplayOrder).
		SetIsActive(category.IsActive).
		SetImageURL(category.ImageURL)

	if category.ParentID != nil {
		builder.SetParentID(*category.ParentID)
	} else {
		builder.ClearParentID()
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrCategoryNotFound
		}
		return err
	}

	category.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *EntRepository) DeleteCategory(ctx context.Context, tenantID, categoryID uuid.UUID) error {
	_, err := r.client.MenuCategory.Delete().
		Where(
			menucategory.ID(categoryID),
			menucategory.TenantID(tenantID),
		).Exec(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (r *EntRepository) CountCategoryItems(ctx context.Context, categoryID uuid.UUID) (int, error) {
	return r.client.MenuItem.Query().
		Where(menuitem.CategoryID(categoryID)).
		Count(ctx)
}

func (r *EntRepository) CountCategoryChildren(ctx context.Context, categoryID uuid.UUID) (int, error) {
	return r.client.MenuCategory.Query().
		Where(menucategory.ParentID(categoryID)).
		Count(ctx)
}

// --- MenuItem Methods ---

func (r *EntRepository) CreateMenuItem(ctx context.Context, item *MenuItem) error {
	builder := r.client.MenuItem.Create().
		SetTenantID(item.TenantID).
		SetCafeID(item.CafeID).
		SetCategoryID(item.CategoryID).
		SetName(item.Name).
		SetDescription(item.Description).
		SetBasePrice(item.BasePrice).
		SetCurrency(item.Currency).
		SetIsAvailable(item.IsAvailable).
		SetLeadTimeMinutes(item.LeadTimeMinutes).
		SetImageURL(item.ImageURL).
		SetSku(item.SKU).
		SetDisplayOrder(item.DisplayOrder)

	if item.Nutrition != nil {
		builder.SetNutritionJSON(item.Nutrition)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	item.ID = created.ID
	item.CreatedAt = created.CreatedAt
	item.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetMenuItem(ctx context.Context, tenantID, itemID uuid.UUID) (*MenuItem, error) {
	item, err := r.client.MenuItem.Query().
		Where(
			menuitem.ID(itemID),
			menuitem.TenantID(tenantID),
		).
		WithCategory().
		WithVariants().
		WithTranslations().
		WithDietaryTags().
		WithAssets().
		WithSchedules().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrMenuItemNotFound
		}
		return nil, err
	}
	return entMenuItemToDomain(item), nil
}

func (r *EntRepository) GetMenuItemBySKU(ctx context.Context, tenantID uuid.UUID, sku string) (*MenuItem, error) {
	item, err := r.client.MenuItem.Query().
		Where(
			menuitem.TenantID(tenantID),
			menuitem.Sku(sku),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrMenuItemNotFound
		}
		return nil, err
	}
	return entMenuItemToDomain(item), nil
}

func (r *EntRepository) ListMenuItems(ctx context.Context, filter MenuItemFilter) ([]MenuItem, int, error) {
	query := r.client.MenuItem.Query().
		Where(menuitem.TenantID(filter.TenantID))

	if filter.CafeID != nil {
		query = query.Where(menuitem.CafeID(*filter.CafeID))
	}
	if filter.CategoryID != nil {
		query = query.Where(menuitem.CategoryID(*filter.CategoryID))
	}
	if filter.IsAvailable != nil {
		query = query.Where(menuitem.IsAvailable(*filter.IsAvailable))
	}
	if filter.Search != "" {
		query = query.Where(
			menuitem.Or(
				menuitem.NameContainsFold(filter.Search),
				menuitem.DescriptionContainsFold(filter.Search),
			),
		)
	}
	if filter.MinPrice != nil {
		query = query.Where(menuitem.BasePriceGTE(*filter.MinPrice))
	}
	if filter.MaxPrice != nil {
		query = query.Where(menuitem.BasePriceLTE(*filter.MaxPrice))
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	query = query.Order(ent.Asc(menuitem.FieldDisplayOrder))
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	items, err := query.
		WithCategory().
		WithVariants().
		WithTranslations().
		WithDietaryTags().
		All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]MenuItem, len(items))
	for i, item := range items {
		result[i] = *entMenuItemToDomain(item)
	}
	return result, total, nil
}

func (r *EntRepository) UpdateMenuItem(ctx context.Context, item *MenuItem) error {
	builder := r.client.MenuItem.UpdateOneID(item.ID).
		SetName(item.Name).
		SetDescription(item.Description).
		SetBasePrice(item.BasePrice).
		SetCurrency(item.Currency).
		SetIsAvailable(item.IsAvailable).
		SetLeadTimeMinutes(item.LeadTimeMinutes).
		SetImageURL(item.ImageURL).
		SetSku(item.SKU).
		SetDisplayOrder(item.DisplayOrder).
		SetCategoryID(item.CategoryID)

	if item.Nutrition != nil {
		builder.SetNutritionJSON(item.Nutrition)
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrMenuItemNotFound
		}
		return err
	}

	item.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *EntRepository) DeleteMenuItem(ctx context.Context, tenantID, itemID uuid.UUID) error {
	_, err := r.client.MenuItem.Delete().
		Where(
			menuitem.ID(itemID),
			menuitem.TenantID(tenantID),
		).Exec(ctx)
	return err
}

// --- Variant Methods ---

func (r *EntRepository) CreateVariant(ctx context.Context, variant *Variant) error {
	created, err := r.client.MenuItemVariant.Create().
		SetMenuItemID(variant.MenuItemID).
		SetName(variant.Name).
		SetPriceDelta(variant.PriceDelta).
		SetIsAvailable(variant.IsAvailable).
		SetSku(variant.SKU).
		SetDisplayOrder(variant.DisplayOrder).
		Save(ctx)
	if err != nil {
		return err
	}

	variant.ID = created.ID
	variant.CreatedAt = created.CreatedAt
	variant.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetVariant(ctx context.Context, variantID uuid.UUID) (*Variant, error) {
	v, err := r.client.MenuItemVariant.Get(ctx, variantID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrVariantNotFound
		}
		return nil, err
	}
	return entVariantToDomain(v), nil
}

func (r *EntRepository) ListVariants(ctx context.Context, menuItemID uuid.UUID) ([]Variant, error) {
	variants, err := r.client.MenuItemVariant.Query().
		Where(menuitemvariant.MenuItemID(menuItemID)).
		Order(ent.Asc(menuitemvariant.FieldDisplayOrder)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]Variant, len(variants))
	for i, v := range variants {
		result[i] = *entVariantToDomain(v)
	}
	return result, nil
}

func (r *EntRepository) UpdateVariant(ctx context.Context, variant *Variant) error {
	updated, err := r.client.MenuItemVariant.UpdateOneID(variant.ID).
		SetName(variant.Name).
		SetPriceDelta(variant.PriceDelta).
		SetIsAvailable(variant.IsAvailable).
		SetSku(variant.SKU).
		SetDisplayOrder(variant.DisplayOrder).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrVariantNotFound
		}
		return err
	}

	variant.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *EntRepository) DeleteVariant(ctx context.Context, variantID uuid.UUID) error {
	return r.client.MenuItemVariant.DeleteOneID(variantID).Exec(ctx)
}

// --- Translation Methods ---

func (r *EntRepository) CreateTranslation(ctx context.Context, translation *Translation) error {
	created, err := r.client.MenuItemTranslation.Create().
		SetMenuItemID(translation.MenuItemID).
		SetLocale(translation.Locale).
		SetName(translation.Name).
		SetDescription(translation.Description).
		Save(ctx)
	if err != nil {
		return err
	}

	translation.ID = created.ID
	translation.CreatedAt = created.CreatedAt
	translation.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetTranslation(ctx context.Context, menuItemID uuid.UUID, locale string) (*Translation, error) {
	t, err := r.client.MenuItemTranslation.Query().
		Where(
			menuitemtranslation.MenuItemID(menuItemID),
			menuitemtranslation.Locale(locale),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrTranslationNotFound
		}
		return nil, err
	}
	return entTranslationToDomain(t), nil
}

func (r *EntRepository) ListTranslations(ctx context.Context, menuItemID uuid.UUID) ([]Translation, error) {
	translations, err := r.client.MenuItemTranslation.Query().
		Where(menuitemtranslation.MenuItemID(menuItemID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]Translation, len(translations))
	for i, t := range translations {
		result[i] = *entTranslationToDomain(t)
	}
	return result, nil
}

func (r *EntRepository) UpdateTranslation(ctx context.Context, translation *Translation) error {
	updated, err := r.client.MenuItemTranslation.UpdateOneID(translation.ID).
		SetName(translation.Name).
		SetDescription(translation.Description).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrTranslationNotFound
		}
		return err
	}

	translation.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *EntRepository) DeleteTranslation(ctx context.Context, menuItemID uuid.UUID, locale string) error {
	_, err := r.client.MenuItemTranslation.Delete().
		Where(
			menuitemtranslation.MenuItemID(menuItemID),
			menuitemtranslation.Locale(locale),
		).Exec(ctx)
	return err
}

// --- DietaryTag Methods ---

func (r *EntRepository) CreateDietaryTag(ctx context.Context, tag *DietaryTag) error {
	created, err := r.client.DietaryTag.Create().
		SetCode(tag.Code).
		SetLabel(tag.Label).
		SetDescription(tag.Description).
		SetIconURL(tag.IconURL).
		Save(ctx)
	if err != nil {
		return err
	}

	tag.CreatedAt = created.CreatedAt
	return nil
}

func (r *EntRepository) GetDietaryTag(ctx context.Context, code string) (*DietaryTag, error) {
	tag, err := r.client.DietaryTag.Query().
		Where(dietarytag.Code(code)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrDietaryTagNotFound
		}
		return nil, err
	}
	return entDietaryTagToDomain(tag), nil
}

func (r *EntRepository) ListDietaryTags(ctx context.Context) ([]DietaryTag, error) {
	tags, err := r.client.DietaryTag.Query().
		Order(ent.Asc(dietarytag.FieldLabel)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]DietaryTag, len(tags))
	for i, tag := range tags {
		result[i] = *entDietaryTagToDomain(tag)
	}
	return result, nil
}

func (r *EntRepository) DeleteDietaryTag(ctx context.Context, code string) error {
	_, err := r.client.DietaryTag.Delete().
		Where(dietarytag.Code(code)).
		Exec(ctx)
	return err
}

func (r *EntRepository) AddDietaryTagToItem(ctx context.Context, menuItemID uuid.UUID, tagCode string) error {
	// Look up tag by code to get its ID
	tag, err := r.client.DietaryTag.Query().
		Where(dietarytag.Code(tagCode)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrDietaryTagNotFound
		}
		return err
	}
	return r.client.MenuItem.UpdateOneID(menuItemID).
		AddDietaryTagIDs(tag.ID).
		Exec(ctx)
}

func (r *EntRepository) RemoveDietaryTagFromItem(ctx context.Context, menuItemID uuid.UUID, tagCode string) error {
	// Look up tag by code to get its ID
	tag, err := r.client.DietaryTag.Query().
		Where(dietarytag.Code(tagCode)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrDietaryTagNotFound
		}
		return err
	}
	return r.client.MenuItem.UpdateOneID(menuItemID).
		RemoveDietaryTagIDs(tag.ID).
		Exec(ctx)
}

func (r *EntRepository) ListItemDietaryTags(ctx context.Context, menuItemID uuid.UUID) ([]DietaryTag, error) {
	item, err := r.client.MenuItem.Query().
		Where(menuitem.ID(menuItemID)).
		WithDietaryTags().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrMenuItemNotFound
		}
		return nil, err
	}

	result := make([]DietaryTag, len(item.Edges.DietaryTags))
	for i, tag := range item.Edges.DietaryTags {
		result[i] = *entDietaryTagToDomain(tag)
	}
	return result, nil
}

// --- Asset Methods ---

func (r *EntRepository) CreateAsset(ctx context.Context, asset *Asset) error {
	created, err := r.client.MenuItemAsset.Create().
		SetMenuItemID(asset.MenuItemID).
		SetAssetType(string(asset.AssetType)).
		SetURL(asset.URL).
		SetMetadata(asset.Metadata).
		Save(ctx)
	if err != nil {
		return err
	}

	asset.ID = created.ID
	asset.CreatedAt = created.CreatedAt
	return nil
}

func (r *EntRepository) GetAsset(ctx context.Context, assetID uuid.UUID) (*Asset, error) {
	a, err := r.client.MenuItemAsset.Get(ctx, assetID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrAssetNotFound
		}
		return nil, err
	}
	return entAssetToDomain(a), nil
}

func (r *EntRepository) ListAssets(ctx context.Context, menuItemID uuid.UUID) ([]Asset, error) {
	assets, err := r.client.MenuItemAsset.Query().
		Where(menuitemasset.MenuItemID(menuItemID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]Asset, len(assets))
	for i, a := range assets {
		result[i] = *entAssetToDomain(a)
	}
	return result, nil
}

func (r *EntRepository) DeleteAsset(ctx context.Context, assetID uuid.UUID) error {
	return r.client.MenuItemAsset.DeleteOneID(assetID).Exec(ctx)
}

// --- Schedule Methods ---

func (r *EntRepository) CreateSchedule(ctx context.Context, schedule *Schedule) error {
	created, err := r.client.MenuItemSchedule.Create().
		SetMenuItemID(schedule.MenuItemID).
		SetDayOfWeek(schedule.DayOfWeek).
		SetTimeStart(schedule.TimeStart).
		SetTimeEnd(schedule.TimeEnd).
		Save(ctx)
	if err != nil {
		return err
	}

	schedule.ID = created.ID
	schedule.CreatedAt = created.CreatedAt
	return nil
}

func (r *EntRepository) GetSchedule(ctx context.Context, scheduleID uuid.UUID) (*Schedule, error) {
	s, err := r.client.MenuItemSchedule.Get(ctx, scheduleID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrScheduleNotFound
		}
		return nil, err
	}
	return entScheduleToDomain(s), nil
}

func (r *EntRepository) ListSchedules(ctx context.Context, menuItemID uuid.UUID) ([]Schedule, error) {
	schedules, err := r.client.MenuItemSchedule.Query().
		Where(menuitemschedule.MenuItemID(menuItemID)).
		Order(ent.Asc(menuitemschedule.FieldDayOfWeek), ent.Asc(menuitemschedule.FieldTimeStart)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]Schedule, len(schedules))
	for i, s := range schedules {
		result[i] = *entScheduleToDomain(s)
	}
	return result, nil
}

func (r *EntRepository) UpdateSchedule(ctx context.Context, schedule *Schedule) error {
	_, err := r.client.MenuItemSchedule.UpdateOneID(schedule.ID).
		SetDayOfWeek(schedule.DayOfWeek).
		SetTimeStart(schedule.TimeStart).
		SetTimeEnd(schedule.TimeEnd).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrScheduleNotFound
		}
		return err
	}
	return nil
}

func (r *EntRepository) DeleteSchedule(ctx context.Context, scheduleID uuid.UUID) error {
	return r.client.MenuItemSchedule.DeleteOneID(scheduleID).Exec(ctx)
}

// --- Public API Methods ---

func (r *EntRepository) GetPublicMenu(ctx context.Context, req PublicMenuRequest) ([]PublicMenuItem, int, error) {
	filter := MenuItemFilter{
		TenantID:   req.TenantID,
		Search:     req.Search,
		Locale:     req.Locale,
		Limit:      req.Limit,
		Offset:     req.Offset,
	}
	if req.CafeID != nil {
		filter.CafeID = req.CafeID
	}
	if req.CategoryID != nil {
		filter.CategoryID = req.CategoryID
	}
	isAvailable := true
	filter.IsAvailable = &isAvailable

	if filter.TenantID == uuid.Nil {
		return []PublicMenuItem{}, 0, nil
	}

	items, total, err := r.ListMenuItems(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	result := make([]PublicMenuItem, len(items))
	for i, item := range items {
		result[i] = toPublicMenuItem(item, req.Locale)
	}
	return result, total, nil
}

func (r *EntRepository) GetPublicCategories(ctx context.Context, tenantID, cafeID uuid.UUID) ([]PublicCategory, error) {
	isActive := true
	cats, _, err := r.ListCategories(ctx, CategoryFilter{
		TenantID: tenantID,
		CafeID:   &cafeID,
		IsActive: &isActive,
	})
	if err != nil {
		return nil, err
	}

	result := make([]PublicCategory, len(cats))
	for i, cat := range cats {
		result[i] = toPublicCategory(cat)
	}
	return result, nil
}

// GetDistinctCafeIDs returns distinct cafe IDs that have menu categories for the tenant.
func (r *EntRepository) GetDistinctCafeIDs(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error) {
	cats, err := r.client.MenuCategory.Query().
		Where(menucategory.TenantID(tenantID)).
		Select(menucategory.FieldCafeID).
		All(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[uuid.UUID]struct{})
	var out []uuid.UUID
	for _, c := range cats {
		id := c.CafeID
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// --- Conversion Helpers ---

func entCategoryToDomain(ec *ent.MenuCategory) *Category {
	cat := &Category{
		ID:           ec.ID,
		TenantID:     ec.TenantID,
		CafeID:       ec.CafeID,
		ParentID:     ec.ParentID,
		Name:         ec.Name,
		Description:  ec.Description,
		DisplayOrder: ec.DisplayOrder,
		IsActive:     ec.IsActive,
		ImageURL:     ec.ImageURL,
		CreatedAt:    ec.CreatedAt,
		UpdatedAt:    ec.UpdatedAt,
	}

	if ec.Edges.Children != nil {
		cat.Children = make([]Category, len(ec.Edges.Children))
		for i, child := range ec.Edges.Children {
			cat.Children[i] = *entCategoryToDomain(child)
		}
	}

	return cat
}

func entMenuItemToDomain(ei *ent.MenuItem) *MenuItem {
	leadTimeMinutes := 0
	if ei.LeadTimeMinutes != nil {
		leadTimeMinutes = *ei.LeadTimeMinutes
	}
	item := &MenuItem{
		ID:              ei.ID,
		TenantID:        ei.TenantID,
		CafeID:          ei.CafeID,
		CategoryID:      ei.CategoryID,
		Name:            ei.Name,
		Description:     ei.Description,
		BasePrice:       ei.BasePrice,
		Currency:        ei.Currency,
		IsAvailable:     ei.IsAvailable,
		LeadTimeMinutes: leadTimeMinutes,
		ImageURL:        ei.ImageURL,
		Nutrition:       ei.NutritionJSON,
		SKU:             ei.Sku,
		DisplayOrder:    ei.DisplayOrder,
		CreatedAt:       ei.CreatedAt,
		UpdatedAt:       ei.UpdatedAt,
	}

	if ei.Edges.Category != nil {
		item.Category = entCategoryToDomain(ei.Edges.Category)
	}

	if ei.Edges.Variants != nil {
		item.Variants = make([]Variant, len(ei.Edges.Variants))
		for i, v := range ei.Edges.Variants {
			item.Variants[i] = *entVariantToDomain(v)
		}
	}

	if ei.Edges.Translations != nil {
		item.Translations = make([]Translation, len(ei.Edges.Translations))
		for i, t := range ei.Edges.Translations {
			item.Translations[i] = *entTranslationToDomain(t)
		}
	}

	if ei.Edges.DietaryTags != nil {
		item.DietaryTags = make([]DietaryTag, len(ei.Edges.DietaryTags))
		for i, tag := range ei.Edges.DietaryTags {
			item.DietaryTags[i] = *entDietaryTagToDomain(tag)
		}
	}

	if ei.Edges.Assets != nil {
		item.Assets = make([]Asset, len(ei.Edges.Assets))
		for i, a := range ei.Edges.Assets {
			item.Assets[i] = *entAssetToDomain(a)
		}
	}

	if ei.Edges.Schedules != nil {
		item.Schedules = make([]Schedule, len(ei.Edges.Schedules))
		for i, s := range ei.Edges.Schedules {
			item.Schedules[i] = *entScheduleToDomain(s)
		}
	}

	return item
}

func entVariantToDomain(ev *ent.MenuItemVariant) *Variant {
	return &Variant{
		ID:           ev.ID,
		MenuItemID:   ev.MenuItemID,
		Name:         ev.Name,
		PriceDelta:   ev.PriceDelta,
		IsAvailable:  ev.IsAvailable,
		SKU:          ev.Sku,
		DisplayOrder: ev.DisplayOrder,
		CreatedAt:    ev.CreatedAt,
		UpdatedAt:    ev.UpdatedAt,
	}
}

func entTranslationToDomain(et *ent.MenuItemTranslation) *Translation {
	return &Translation{
		ID:          et.ID,
		MenuItemID:  et.MenuItemID,
		Locale:      et.Locale,
		Name:        et.Name,
		Description: et.Description,
		CreatedAt:   et.CreatedAt,
		UpdatedAt:   et.UpdatedAt,
	}
}

func entDietaryTagToDomain(edt *ent.DietaryTag) *DietaryTag {
	return &DietaryTag{
		Code:        edt.Code,
		Label:       edt.Label,
		Description: edt.Description,
		IconURL:     edt.IconURL,
		CreatedAt:   edt.CreatedAt,
	}
}

func entAssetToDomain(ea *ent.MenuItemAsset) *Asset {
	return &Asset{
		ID:         ea.ID,
		MenuItemID: ea.MenuItemID,
		AssetType:  AssetType(ea.AssetType),
		URL:        ea.URL,
		Metadata:   ea.Metadata,
		CreatedAt:  ea.CreatedAt,
	}
}

func entScheduleToDomain(es *ent.MenuItemSchedule) *Schedule {
	return &Schedule{
		ID:         es.ID,
		MenuItemID: es.MenuItemID,
		DayOfWeek:  es.DayOfWeek,
		TimeStart:  es.TimeStart,
		TimeEnd:    es.TimeEnd,
		CreatedAt:  es.CreatedAt,
	}
}

func toPublicMenuItem(item MenuItem, locale string) PublicMenuItem {
	pm := PublicMenuItem{
		ID:              item.ID,
		CategoryID:      item.CategoryID,
		Name:            item.Name,
		Description:     item.Description,
		BasePrice:       item.BasePrice,
		Currency:        item.Currency,
		ImageURL:        item.ImageURL,
		LeadTimeMinutes: item.LeadTimeMinutes,
		Variants:        item.Variants,
		DietaryTags:     item.DietaryTags,
	}

	if item.Category != nil {
		pm.CategoryName = item.Category.Name
	}

	// Apply translation if available
	if locale != "" && locale != LocaleEnglish {
		for _, t := range item.Translations {
			if strings.EqualFold(t.Locale, locale) {
				pm.Name = t.Name
				if t.Description != "" {
					pm.Description = t.Description
				}
				break
			}
		}
	}

	return pm
}

func toPublicCategory(cat Category) PublicCategory {
	pc := PublicCategory{
		ID:          cat.ID,
		Name:        cat.Name,
		Description: cat.Description,
		ImageURL:    cat.ImageURL,
		ItemCount:   cat.ItemCount,
	}

	if cat.Children != nil {
		pc.Children = make([]PublicCategory, len(cat.Children))
		for i, child := range cat.Children {
			pc.Children[i] = toPublicCategory(child)
		}
	}

	return pc
}
