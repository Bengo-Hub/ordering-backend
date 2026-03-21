package catalog

import (
	"context"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/catalogcategory"
	"github.com/bengobox/ordering-backend/internal/ent/catalogitem"
	"github.com/bengobox/ordering-backend/internal/ent/catalogitemasset"
	"github.com/bengobox/ordering-backend/internal/ent/catalogitemschedule"
	"github.com/bengobox/ordering-backend/internal/ent/dietarytag"
	"github.com/bengobox/ordering-backend/internal/ent/outlet"
	"github.com/bengobox/ordering-backend/internal/ent/user"
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
	builder := r.client.CatalogCategory.Create().
		SetNillableTenantID(category.TenantID).
		SetNillableOutletID(category.OutletID).
		SetName(category.Name).
		SetSlug(category.Slug).
		SetDescription(category.Description).
		SetImageURL(category.ImageURL).
		SetDisplayOrder(category.DisplayOrder).
		SetIsActive(category.IsActive)

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
	cat, err := r.client.CatalogCategory.Query().
		Where(
			catalogcategory.ID(categoryID),
			catalogcategory.TenantID(tenantID),
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


func (r *EntRepository) ListCategories(ctx context.Context, filter CategoryFilter) ([]Category, int, error) {
	query := r.client.CatalogCategory.Query().
		Where(catalogcategory.TenantID(filter.TenantID))

	if filter.OutletID != nil {
		query = query.Where(catalogcategory.OutletID(*filter.OutletID))
	}
	if filter.ParentID != nil {
		query = query.Where(catalogcategory.ParentID(*filter.ParentID))
	}
	if filter.IsActive != nil {
		query = query.Where(catalogcategory.IsActive(*filter.IsActive))
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	query = query.Order(ent.Asc(catalogcategory.FieldDisplayOrder))
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
	builder := r.client.CatalogCategory.UpdateOneID(category.ID).
		SetName(category.Name).
		SetSlug(category.Slug).
		SetDescription(category.Description).
		SetImageURL(category.ImageURL).
		SetDisplayOrder(category.DisplayOrder).
		SetIsActive(category.IsActive)

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
	_, err := r.client.CatalogCategory.Delete().
		Where(
			catalogcategory.ID(categoryID),
			catalogcategory.TenantID(tenantID),
		).Exec(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (r *EntRepository) CountCategoryItems(ctx context.Context, categoryID uuid.UUID) (int, error) {
	return r.client.CatalogItem.Query().
		Where(catalogitem.CategoryID(categoryID)).
		Count(ctx)
}

func (r *EntRepository) CountCategoryChildren(ctx context.Context, categoryID uuid.UUID) (int, error) {
	return r.client.CatalogCategory.Query().
		Where(catalogcategory.ParentID(categoryID)).
		Count(ctx)
}

// --- CatalogItem Methods ---

func (r *EntRepository) GetCatalogItemBySKU(ctx context.Context, tenantID uuid.UUID, sku string) (*CatalogItem, error) {
	item, err := r.client.CatalogItem.Query().
		Where(
			catalogitem.TenantID(tenantID),
			catalogitem.Sku(sku),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrCatalogItemNotFound
		}
		return nil, err
	}
	return entCatalogItemToDomain(item), nil
}

func (r *EntRepository) CreateCatalogItem(ctx context.Context, item *CatalogItem) error {
	builder := r.client.CatalogItem.Create().
		SetTenantID(item.TenantID).
		SetOutletID(item.OutletID).
		SetCategoryID(item.CategoryID).
		SetNillableInventoryItemID(item.InventoryItemID).
		SetName(item.Name).
		SetDescription(item.Description).
		SetBasePrice(item.BasePrice).
		SetCurrency(item.Currency).
		SetImageURL(item.ImageURL).
		SetIsAvailable(item.IsAvailable).
		SetIsFeatured(item.IsFeatured).
		SetLeadTimeMinutes(item.LeadTimeMinutes).
		SetSku(item.SKU).
		SetDisplayOrder(item.DisplayOrder)

	if item.RecipeID != nil {
		builder.SetRecipeID(*item.RecipeID)
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

func (r *EntRepository) GetCatalogItem(ctx context.Context, tenantID, itemID uuid.UUID) (*CatalogItem, error) {
	item, err := r.client.CatalogItem.Query().
		Where(
			catalogitem.ID(itemID),
			catalogitem.TenantID(tenantID),
		).
		WithCategory().
		WithDietaryTags().
		WithAssets().
		WithSchedules().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrCatalogItemNotFound
		}
		return nil, err
	}
	return entCatalogItemToDomain(item), nil
}


func (r *EntRepository) ListCatalogItems(ctx context.Context, filter CatalogItemFilter) ([]CatalogItem, int, error) {
	query := r.client.CatalogItem.Query().
		Where(catalogitem.TenantID(filter.TenantID))

	if filter.OutletID != nil {
		query = query.Where(catalogitem.OutletID(*filter.OutletID))
	}
	if filter.CategoryID != nil {
		query = query.Where(catalogitem.CategoryID(*filter.CategoryID))
	}
	if filter.IsAvailable != nil {
		query = query.Where(catalogitem.IsAvailable(*filter.IsAvailable))
	}
	if filter.IsFeatured != nil {
		query = query.Where(catalogitem.IsFeatured(*filter.IsFeatured))
	}
	if filter.Search != "" {
		query = query.Where(
			catalogitem.Or(
				catalogitem.NameContainsFold(filter.Search),
				catalogitem.DescriptionContainsFold(filter.Search),
				catalogitem.SkuContainsFold(filter.Search),
			),
		)
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	query = query.Order(ent.Asc(catalogitem.FieldDisplayOrder))
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	if filter.UserID != nil {
		query = query.WithFavoritedBy(func(q *ent.UserQuery) {
			q.Where(user.ID(*filter.UserID))
		})
		if filter.FavoriteOnly {
			query = query.Where(catalogitem.HasFavoritedByWith(user.ID(*filter.UserID)))
		}
	}

	items, err := query.
		WithCategory().
		WithDietaryTags().
		All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]CatalogItem, len(items))
	for i, item := range items {
		result[i] = *entCatalogItemToDomain(item)
	}
	return result, total, nil
}

func (r *EntRepository) UpdateCatalogItem(ctx context.Context, item *CatalogItem) error {
	builder := r.client.CatalogItem.UpdateOneID(item.ID).
		SetName(item.Name).
		SetDescription(item.Description).
		SetBasePrice(item.BasePrice).
		SetCurrency(item.Currency).
		SetImageURL(item.ImageURL).
		SetIsAvailable(item.IsAvailable).
		SetIsFeatured(item.IsFeatured).
		SetLeadTimeMinutes(item.LeadTimeMinutes).
		SetSku(item.SKU).
		SetDisplayOrder(item.DisplayOrder).
		SetCategoryID(item.CategoryID)

	updated, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrCatalogItemNotFound
		}
		return err
	}

	item.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *EntRepository) DeleteCatalogItem(ctx context.Context, tenantID, itemID uuid.UUID) error {
	_, err := r.client.CatalogItem.Delete().
		Where(
			catalogitem.ID(itemID),
			catalogitem.TenantID(tenantID),
		).Exec(ctx)
	return err
}

func (r *EntRepository) ToggleFavorite(ctx context.Context, userID, itemID uuid.UUID) (bool, error) {
	// Check if user has favorited this item
	exists, err := r.client.User.Query().
		Where(user.ID(userID)).
		QueryFavoriteItems().
		Where(catalogitem.ID(itemID)).
		Exist(ctx)
	if err != nil {
		return false, err
	}

	if exists {
		// Remove from favorites
		err = r.client.User.UpdateOneID(userID).
			RemoveFavoriteItemIDs(itemID).
			Exec(ctx)
		return false, err
	}

	// Add to favorites
	err = r.client.User.UpdateOneID(userID).
		AddFavoriteItemIDs(itemID).
		Exec(ctx)
	return true, err
}

// --- Translation Methods ---


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

func (r *EntRepository) AddDietaryTagToItem(ctx context.Context, catalogItemID uuid.UUID, tagCode string) error {
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
	return r.client.CatalogItem.UpdateOneID(catalogItemID).
		AddDietaryTagIDs(tag.ID).
		Exec(ctx)
}

func (r *EntRepository) RemoveDietaryTagFromItem(ctx context.Context, catalogItemID uuid.UUID, tagCode string) error {
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
	return r.client.CatalogItem.UpdateOneID(catalogItemID).
		RemoveDietaryTagIDs(tag.ID).
		Exec(ctx)
}

func (r *EntRepository) ListItemDietaryTags(ctx context.Context, catalogItemID uuid.UUID) ([]DietaryTag, error) {
	item, err := r.client.CatalogItem.Query().
		Where(catalogitem.ID(catalogItemID)).
		WithDietaryTags().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrCatalogItemNotFound
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
	created, err := r.client.CatalogItemAsset.Create().
		SetCatalogItemID(asset.CatalogItemID).
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
	a, err := r.client.CatalogItemAsset.Get(ctx, assetID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrAssetNotFound
		}
		return nil, err
	}
	return entAssetToDomain(a), nil
}

func (r *EntRepository) ListAssets(ctx context.Context, catalogItemID uuid.UUID) ([]Asset, error) {
	assets, err := r.client.CatalogItemAsset.Query().
		Where(catalogitemasset.CatalogItemID(catalogItemID)).
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
	return r.client.CatalogItemAsset.DeleteOneID(assetID).Exec(ctx)
}

// --- Schedule Methods ---

func (r *EntRepository) CreateSchedule(ctx context.Context, schedule *Schedule) error {
	created, err := r.client.CatalogItemSchedule.Create().
		SetCatalogItemID(schedule.CatalogItemID).
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
	s, err := r.client.CatalogItemSchedule.Get(ctx, scheduleID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrScheduleNotFound
		}
		return nil, err
	}
	return entScheduleToDomain(s), nil
}

func (r *EntRepository) ListSchedules(ctx context.Context, catalogItemID uuid.UUID) ([]Schedule, error) {
	schedules, err := r.client.CatalogItemSchedule.Query().
		Where(catalogitemschedule.CatalogItemID(catalogItemID)).
		Order(ent.Asc(catalogitemschedule.FieldDayOfWeek), ent.Asc(catalogitemschedule.FieldTimeStart)).
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
	_, err := r.client.CatalogItemSchedule.UpdateOneID(schedule.ID).
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
	return r.client.CatalogItemSchedule.DeleteOneID(scheduleID).Exec(ctx)
}

// --- Public API Methods ---

func (r *EntRepository) GetPublicMenu(ctx context.Context, req PublicCatalogRequest) ([]PublicCatalogItem, int, error) {
	filter := CatalogItemFilter{
		TenantID: req.TenantID,
		Search:   req.Search,
		Locale:   req.Locale,
		Limit:    req.Limit,
		Offset:   req.Offset,
	}
	if req.OutletID != nil {
		filter.OutletID = req.OutletID
	}
	if req.CategoryID != nil {
		filter.CategoryID = req.CategoryID
	}
	if req.IsFeatured != nil {
		filter.IsFeatured = req.IsFeatured
	}
	isAvailable := true
	filter.IsAvailable = &isAvailable

	if req.UserID != nil {
		filter.UserID = req.UserID
		filter.FavoriteOnly = req.FavoriteOnly
	}

	items, total, err := r.ListCatalogItems(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	result := make([]PublicCatalogItem, len(items))
	for i, item := range items {
		result[i] = toPublicCatalogItem(item, req.Locale)
	}
	return result, total, nil
}

// GetPublicCatalogItem retrieves a single catalog item for public display.
func (r *EntRepository) GetPublicCatalogItem(ctx context.Context, tenantID, itemID uuid.UUID, locale string) (*PublicCatalogItem, error) {
	item, err := r.GetCatalogItem(ctx, tenantID, itemID)
	if err != nil {
		return nil, err
	}
	if !item.IsAvailable {
		return nil, ErrCatalogItemNotFound
	}
	pub := toPublicCatalogItem(*item, locale)
	return &pub, nil
}

func (r *EntRepository) GetPublicCategories(ctx context.Context, tenantID, outletID uuid.UUID) ([]PublicCategory, error) {
	isActive := true
	cats, _, err := r.ListCategories(ctx, CategoryFilter{
		TenantID: tenantID,
		OutletID: &outletID,
		IsActive: &isActive,
	})
	if err != nil {
		return nil, err
	}

	result := make([]PublicCategory, len(cats))
	for i, cat := range cats {
		result[i] = toPublicCategory(cat)
		count, countErr := r.CountCategoryItems(ctx, cat.ID)
		if countErr == nil {
			result[i].ItemCount = count
		}
	}
	return result, nil
}

// entOutletToSummary converts an Ent Outlet entity to an OutletSummary domain object.
func entOutletToSummary(o *ent.Outlet) OutletSummary {
	return OutletSummary{
		ID:           o.ID,
		Name:         o.Name,
		Slug:         o.Slug,
		Description:  o.Description,
		Address:      o.Address,
		Phone:        o.Phone,
		Email:        o.Email,
		Location:     o.Location,
		Latitude:     o.Latitude,
		Longitude:    o.Longitude,
		OpeningHours: o.OpeningHours,
		ImageURL:     o.ImageURL,
		Status:       o.Status,
		UseCase:      o.UseCase,
	}
}

// ListOutlets returns all outlets for a tenant from the outlets table.
func (r *EntRepository) ListOutlets(ctx context.Context, tenantID uuid.UUID) ([]OutletSummary, error) {
	outlets, err := r.client.Outlet.Query().
		Where(outlet.TenantID(tenantID), outlet.StatusEQ("active")).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]OutletSummary, len(outlets))
	for i, o := range outlets {
		result[i] = entOutletToSummary(o)
	}
	return result, nil
}

// GetOutlet retrieves display information for a single outlet.
func (r *EntRepository) GetOutlet(ctx context.Context, tenantID, outletID uuid.UUID) (*OutletSummary, error) {
	o, err := r.client.Outlet.Query().
		Where(outlet.ID(outletID), outlet.TenantID(tenantID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrCatalogItemNotFound
		}
		return nil, err
	}
	summary := entOutletToSummary(o)
	return &summary, nil
}

// --- Conversion Helpers ---

func entCategoryToDomain(ec *ent.CatalogCategory) *Category {
	cat := &Category{
		ID:           ec.ID,
		TenantID:     ec.TenantID,
		OutletID:     ec.OutletID,
		ParentID:     ec.ParentID,
		Name:         ec.Name,
		Slug:         ec.Slug,
		Description:  ec.Description,
		ImageURL:     ec.ImageURL,
		DisplayOrder: ec.DisplayOrder,
		IsActive:     ec.IsActive,
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

func entCatalogItemToDomain(ei *ent.CatalogItem) *CatalogItem {
	leadTimeMinutes := 0
	if ei.LeadTimeMinutes != nil {
		leadTimeMinutes = *ei.LeadTimeMinutes
	}
	item := &CatalogItem{
		ID:              ei.ID,
		TenantID:        ei.TenantID,
		OutletID:        ei.OutletID,
		InventoryItemID: ei.InventoryItemID,
		CategoryID:      ei.CategoryID,
		Name:            ei.Name,
		Description:     ei.Description,
		BasePrice:       ei.BasePrice,
		Currency:        ei.Currency,
		IsAvailable:     ei.IsAvailable,
		IsFeatured:      ei.IsFeatured,
		LeadTimeMinutes: leadTimeMinutes,
		RecipeID:        ei.RecipeID,
		SKU:             ei.Sku,
		ImageURL:        ei.ImageURL,
		DisplayOrder:    ei.DisplayOrder,
		IsFavorite:      len(ei.Edges.FavoritedBy) > 0,
		CreatedAt:       ei.CreatedAt,
		UpdatedAt:       ei.UpdatedAt,
	}

	if ei.Edges.Category != nil {
		item.Category = entCategoryToDomain(ei.Edges.Category)
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

func entDietaryTagToDomain(edt *ent.DietaryTag) *DietaryTag {
	return &DietaryTag{
		Code:        edt.Code,
		Label:       edt.Label,
		Description: edt.Description,
		IconURL:     edt.IconURL,
		CreatedAt:   edt.CreatedAt,
	}
}

func entAssetToDomain(ea *ent.CatalogItemAsset) *Asset {
	return &Asset{
		ID:         ea.ID,
		CatalogItemID: ea.CatalogItemID,
		AssetType:  AssetType(ea.AssetType),
		URL:        ea.URL,
		Metadata:   ea.Metadata,
		CreatedAt:  ea.CreatedAt,
	}
}

func entScheduleToDomain(es *ent.CatalogItemSchedule) *Schedule {
	return &Schedule{
		ID:         es.ID,
		CatalogItemID: es.CatalogItemID,
		DayOfWeek:  es.DayOfWeek,
		TimeStart:  es.TimeStart,
		TimeEnd:    es.TimeEnd,
		CreatedAt:  es.CreatedAt,
	}
}

func toPublicCatalogItem(item CatalogItem, locale string) PublicCatalogItem {
	imageURL := item.ImageURL
	if imageURL == "" && len(item.Assets) > 0 {
		imageURL = item.Assets[0].URL
	}

	pm := PublicCatalogItem{
		ID:              item.ID,
		CategoryID:      item.CategoryID,
		Name:            item.Name,
		Description:     item.Description,
		BasePrice:       item.BasePrice,
		Currency:        item.Currency,
		ImageURL:        imageURL,
		LeadTimeMinutes: item.LeadTimeMinutes,
		DietaryTags:     item.DietaryTags,
		IsFavorite:      item.IsFavorite,
	}

	if item.Category != nil {
		pm.CategoryName = item.Category.Name
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
