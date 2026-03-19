package catalog

import "errors"

var (
	// Category errors
	ErrCategoryNotFound      = errors.New("category not found")
	ErrCategoryAlreadyExists = errors.New("category with this name already exists")
	ErrCategoryHasItems      = errors.New("cannot delete category with items")
	ErrCategoryHasChildren   = errors.New("cannot delete category with subcategories")
	ErrInvalidCategoryParent = errors.New("invalid parent category")

	// CatalogItem errors
	ErrCatalogItemNotFound      = errors.New("catalog item not found")
	ErrCatalogItemAlreadyExists = errors.New("catalog item with this SKU already exists")
	ErrInvalidCategory          = errors.New("invalid category for catalog item")
	ErrInvalidPrice          = errors.New("price must be non-negative")
	ErrInvalidSKU            = errors.New("invalid SKU format")

	// Variant errors
	ErrVariantNotFound      = errors.New("variant not found")
	ErrVariantAlreadyExists = errors.New("variant with this name already exists")

	// Translation errors
	ErrTranslationNotFound      = errors.New("translation not found")
	ErrTranslationAlreadyExists = errors.New("translation for this locale already exists")
	ErrInvalidLocale            = errors.New("unsupported locale")

	// DietaryTag errors
	ErrDietaryTagNotFound      = errors.New("dietary tag not found")
	ErrDietaryTagAlreadyExists = errors.New("dietary tag already exists")

	// Asset errors
	ErrAssetNotFound    = errors.New("asset not found")
	ErrInvalidAssetType = errors.New("invalid asset type")
	ErrInvalidAssetURL  = errors.New("invalid asset URL")

	// Schedule errors
	ErrScheduleNotFound    = errors.New("schedule not found")
	ErrInvalidScheduleTime = errors.New("invalid schedule time format")
	ErrInvalidDayOfWeek    = errors.New("invalid day of week")

	// General errors
	ErrInvalidTenant  = errors.New("invalid tenant")
	ErrInvalidCafeID  = errors.New("invalid cafe ID")
	ErrUnauthorized   = errors.New("unauthorized access")
	ErrInternalError  = errors.New("internal server error")
)
