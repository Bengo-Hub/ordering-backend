package compliance

import (
	"context"
	"time"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/cart"
	"github.com/bengobox/ordering-backend/internal/ent/customeraddress"
	"github.com/bengobox/ordering-backend/internal/ent/datadeletionjob"
	"github.com/bengobox/ordering-backend/internal/ent/dataexportjob"
	"github.com/bengobox/ordering-backend/internal/ent/datasubjectrequest"
	"github.com/bengobox/ordering-backend/internal/ent/loyaltyaccount"
	entorder "github.com/bengobox/ordering-backend/internal/ent/order"
	"github.com/bengobox/ordering-backend/internal/ent/paymentmethod"
	"github.com/bengobox/ordering-backend/internal/ent/user"
	"github.com/google/uuid"
)

// EntRepository implements Repository using Ent ORM.
type EntRepository struct {
	client *ent.Client
}

// NewEntRepository creates a new EntRepository.
func NewEntRepository(client *ent.Client) *EntRepository {
	return &EntRepository{client: client}
}

// --- Data Subject Requests ---

func (r *EntRepository) CreateRequest(ctx context.Context, req *DataSubjectRequest) error {
	create := r.client.DataSubjectRequest.Create().
		SetID(req.ID).
		SetTenantID(req.TenantID).
		SetUserID(req.UserID).
		SetRequestType(datasubjectrequest.RequestType(req.RequestType)).
		SetStatus(datasubjectrequest.Status(req.Status)).
		SetSubmittedAt(req.SubmittedAt)

	if req.Description != "" {
		create = create.SetDescription(req.Description)
	}
	if req.Notes != "" {
		create = create.SetNotes(req.Notes)
	}

	created, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return ErrRequestAlreadyExists
		}
		return err
	}

	req.ID = created.ID
	req.CreatedAt = created.CreatedAt
	req.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetRequest(ctx context.Context, tenantID, id uuid.UUID) (*DataSubjectRequest, error) {
	req, err := r.client.DataSubjectRequest.Query().
		Where(
			datasubjectrequest.ID(id),
			datasubjectrequest.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrRequestNotFound
		}
		return nil, err
	}
	return entRequestToModel(req), nil
}

func (r *EntRepository) UpdateRequest(ctx context.Context, req *DataSubjectRequest) error {
	update := r.client.DataSubjectRequest.UpdateOneID(req.ID).
		SetStatus(datasubjectrequest.Status(req.Status)).
		SetUpdatedAt(time.Now())

	if req.Notes != "" {
		update = update.SetNotes(req.Notes)
	}
	if req.ResultURL != "" {
		update = update.SetResultURL(req.ResultURL)
	}
	if req.ProcessedAt != nil {
		update = update.SetProcessedAt(*req.ProcessedAt)
	}

	_, err := update.Save(ctx)
	return err
}

func (r *EntRepository) ListRequests(ctx context.Context, filter DataSubjectRequestFilter) ([]DataSubjectRequest, int, error) {
	query := r.client.DataSubjectRequest.Query().
		Where(datasubjectrequest.TenantID(filter.TenantID))

	if filter.UserID != nil {
		query = query.Where(datasubjectrequest.UserID(*filter.UserID))
	}
	if filter.RequestType != nil {
		query = query.Where(datasubjectrequest.RequestTypeEQ(datasubjectrequest.RequestType(*filter.RequestType)))
	}
	if filter.Status != nil {
		query = query.Where(datasubjectrequest.StatusEQ(datasubjectrequest.Status(*filter.Status)))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}

	requests, err := query.Order(ent.Desc(datasubjectrequest.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]DataSubjectRequest, len(requests))
	for i, req := range requests {
		result[i] = *entRequestToModel(req)
	}
	return result, total, nil
}

// --- Export Jobs ---

func (r *EntRepository) CreateExportJob(ctx context.Context, job *DataExportJob) error {
	create := r.client.DataExportJob.Create().
		SetID(job.ID).
		SetTenantID(job.TenantID).
		SetUserID(job.UserID).
		SetFormat(dataexportjob.Format(job.Format)).
		SetStatus(dataexportjob.Status(job.Status)).
		SetRequestedAt(job.RequestedAt)

	if len(job.IncludedData) > 0 {
		create = create.SetIncludedData(job.IncludedData)
	}
	if job.ExpiresAt != nil {
		create = create.SetExpiresAt(*job.ExpiresAt)
	}

	created, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return ErrExportJobAlreadyExists
		}
		return err
	}

	job.ID = created.ID
	job.CreatedAt = created.CreatedAt
	job.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetExportJob(ctx context.Context, tenantID, id uuid.UUID) (*DataExportJob, error) {
	job, err := r.client.DataExportJob.Query().
		Where(
			dataexportjob.ID(id),
			dataexportjob.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrExportJobNotFound
		}
		return nil, err
	}
	return entExportJobToModel(job), nil
}

func (r *EntRepository) GetExportJobByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*DataExportJob, error) {
	// Find any in-progress export job for this user
	job, err := r.client.DataExportJob.Query().
		Where(
			dataexportjob.TenantID(tenantID),
			dataexportjob.UserID(userID),
			dataexportjob.StatusIn(dataexportjob.StatusPending, dataexportjob.StatusInProgress),
		).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil // No active job found
		}
		return nil, err
	}
	return entExportJobToModel(job), nil
}

func (r *EntRepository) UpdateExportJob(ctx context.Context, job *DataExportJob) error {
	update := r.client.DataExportJob.UpdateOneID(job.ID).
		SetStatus(dataexportjob.Status(job.Status)).
		SetUpdatedAt(time.Now())

	if job.StorageURL != "" {
		update = update.SetStorageURL(job.StorageURL)
	}
	if job.ErrorMessage != "" {
		update = update.SetErrorMessage(job.ErrorMessage)
	}
	if job.FileSizeBytes != nil {
		update = update.SetFileSizeBytes(*job.FileSizeBytes)
	}
	if job.RecordsExported != nil {
		update = update.SetRecordsExported(*job.RecordsExported)
	}
	if job.StartedAt != nil {
		update = update.SetStartedAt(*job.StartedAt)
	}
	if job.CompletedAt != nil {
		update = update.SetCompletedAt(*job.CompletedAt)
	}

	_, err := update.Save(ctx)
	return err
}

func (r *EntRepository) ListExportJobs(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]DataExportJob, int, error) {
	query := r.client.DataExportJob.Query().
		Where(dataexportjob.TenantID(tenantID))

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	if offset > 0 {
		query = query.Offset(offset)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	jobs, err := query.Order(ent.Desc(dataexportjob.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]DataExportJob, len(jobs))
	for i, job := range jobs {
		result[i] = *entExportJobToModel(job)
	}
	return result, total, nil
}

// --- Deletion Jobs ---

func (r *EntRepository) CreateDeletionJob(ctx context.Context, job *DataDeletionJob) error {
	create := r.client.DataDeletionJob.Create().
		SetID(job.ID).
		SetTenantID(job.TenantID).
		SetUserID(job.UserID).
		SetDeletionType(datadeletionjob.DeletionType(job.DeletionType)).
		SetStatus(datadeletionjob.Status(job.Status)).
		SetConfirmed(job.Confirmed).
		SetRetentionDays(job.RetentionDays).
		SetRequestedAt(job.RequestedAt)

	if job.Reason != "" {
		create = create.SetReason(job.Reason)
	}
	if job.ScheduledFor != nil {
		create = create.SetScheduledFor(*job.ScheduledFor)
	}

	created, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return ErrDeletionJobAlreadyExists
		}
		return err
	}

	job.ID = created.ID
	job.CreatedAt = created.CreatedAt
	job.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetDeletionJob(ctx context.Context, tenantID, id uuid.UUID) (*DataDeletionJob, error) {
	job, err := r.client.DataDeletionJob.Query().
		Where(
			datadeletionjob.ID(id),
			datadeletionjob.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrDeletionJobNotFound
		}
		return nil, err
	}
	return entDeletionJobToModel(job), nil
}

func (r *EntRepository) GetDeletionJobByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*DataDeletionJob, error) {
	// Find any in-progress deletion job for this user
	job, err := r.client.DataDeletionJob.Query().
		Where(
			datadeletionjob.TenantID(tenantID),
			datadeletionjob.UserID(userID),
			datadeletionjob.StatusIn(datadeletionjob.StatusPending, datadeletionjob.StatusScheduled, datadeletionjob.StatusInProgress),
		).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil // No active job found
		}
		return nil, err
	}
	return entDeletionJobToModel(job), nil
}

func (r *EntRepository) UpdateDeletionJob(ctx context.Context, job *DataDeletionJob) error {
	update := r.client.DataDeletionJob.UpdateOneID(job.ID).
		SetStatus(datadeletionjob.Status(job.Status)).
		SetUpdatedAt(time.Now())

	if job.ErrorMessage != "" {
		update = update.SetErrorMessage(job.ErrorMessage)
	}
	if job.DeletionSummary != nil {
		update = update.SetDeletionSummary(job.DeletionSummary)
	}
	if job.StartedAt != nil {
		update = update.SetStartedAt(*job.StartedAt)
	}
	if job.CompletedAt != nil {
		update = update.SetCompletedAt(*job.CompletedAt)
	}

	_, err := update.Save(ctx)
	return err
}

func (r *EntRepository) ListDeletionJobs(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]DataDeletionJob, int, error) {
	query := r.client.DataDeletionJob.Query().
		Where(datadeletionjob.TenantID(tenantID))

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	if offset > 0 {
		query = query.Offset(offset)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	jobs, err := query.Order(ent.Desc(datadeletionjob.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]DataDeletionJob, len(jobs))
	for i, job := range jobs {
		result[i] = *entDeletionJobToModel(job)
	}
	return result, total, nil
}

// --- User Data Access (for export) ---

func (r *EntRepository) GetUserProfile(ctx context.Context, tenantID, userID uuid.UUID) (map[string]interface{}, error) {
	u, err := r.client.User.Query().
		Where(
			user.ID(userID),
			user.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return map[string]interface{}{
		"id":         u.ID.String(),
		"email":      u.Email,
		"full_name":  u.FullName,
		"phone":      u.Phone,
		"status":     u.Status,
		"created_at": u.CreatedAt,
		"updated_at": u.UpdatedAt,
	}, nil
}

func (r *EntRepository) GetUserOrders(ctx context.Context, tenantID, userID uuid.UUID) ([]map[string]interface{}, error) {
	orders, err := r.client.Order.Query().
		Where(
			entorder.TenantID(tenantID),
			entorder.CustomerID(userID),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, len(orders))
	for i, o := range orders {
		result[i] = map[string]interface{}{
			"id":            o.ID.String(),
			"order_number":  o.OrderNumber,
			"status":        o.Status,
			"subtotal":      o.Subtotal,
			"grand_total":   o.GrandTotal,
			"currency":      o.Currency,
			"created_at":    o.CreatedAt,
			"updated_at":    o.UpdatedAt,
		}
	}
	return result, nil
}

func (r *EntRepository) GetUserAddresses(ctx context.Context, tenantID, userID uuid.UUID) ([]map[string]interface{}, error) {
	addresses, err := r.client.CustomerAddress.Query().
		Where(
			customeraddress.TenantID(tenantID),
			customeraddress.UserID(userID),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, len(addresses))
	for i, a := range addresses {
		result[i] = map[string]interface{}{
			"id":            a.ID.String(),
			"label":         a.Label,
			"address_line1": a.AddressLine1,
			"address_line2": a.AddressLine2,
			"city":          a.City,
			"county":        a.County,
			"postal_code":   a.PostalCode,
			"country":       a.Country,
			"is_default":    a.IsDefault,
			"created_at":    a.CreatedAt,
		}
	}
	return result, nil
}

func (r *EntRepository) GetUserCarts(ctx context.Context, tenantID, userID uuid.UUID) ([]map[string]interface{}, error) {
	carts, err := r.client.Cart.Query().
		Where(
			cart.TenantID(tenantID),
			cart.UserID(userID),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, len(carts))
	for i, c := range carts {
		result[i] = map[string]interface{}{
			"id":         c.ID.String(),
			"status":     c.Status,
			"subtotal":   c.Subtotal,
			"currency":   c.Currency,
			"created_at": c.CreatedAt,
			"updated_at": c.UpdatedAt,
		}
	}
	return result, nil
}

func (r *EntRepository) GetUserLoyaltyData(ctx context.Context, tenantID, userID uuid.UUID) (map[string]interface{}, error) {
	account, err := r.client.LoyaltyAccount.Query().
		Where(
			loyaltyaccount.TenantID(tenantID),
			loyaltyaccount.UserID(userID),
		).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return map[string]interface{}{"message": "no loyalty account"}, nil
		}
		return nil, err
	}

	return map[string]interface{}{
		"id":              account.ID.String(),
		"balance_points":  account.BalancePoints,
		"lifetime_points": account.LifetimePoints,
		"tier":            account.Tier,
		"created_at":      account.CreatedAt,
	}, nil
}

func (r *EntRepository) GetUserPaymentMethods(ctx context.Context, tenantID, userID uuid.UUID) ([]map[string]interface{}, error) {
	methods, err := r.client.PaymentMethod.Query().
		Where(
			paymentmethod.TenantID(tenantID),
			paymentmethod.UserID(userID),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, len(methods))
	for i, m := range methods {
		result[i] = map[string]interface{}{
			"id":         m.ID.String(),
			"provider":   m.Provider,
			"Type":       m.Type,
			"mask":       m.Mask,
			"is_default": m.IsDefault,
			"created_at": m.CreatedAt,
		}
	}
	return result, nil
}

// --- User Data Deletion ---

func (r *EntRepository) SoftDeleteUser(ctx context.Context, tenantID, userID uuid.UUID) error {
	// Update user status to deactivated
	_, err := r.client.User.UpdateOneID(userID).
		SetStatus("deactivated").
		Save(ctx)
	return err
}

func (r *EntRepository) AnonymizeUser(ctx context.Context, tenantID, userID uuid.UUID) error {
	anonymizedEmail := "anonymized_" + userID.String()[:8] + "@deleted.local"

	_, err := r.client.User.UpdateOneID(userID).
		SetEmail(anonymizedEmail).
		SetFullName("Deleted User").
		SetPhone("").
		SetStatus("anonymized").
		Save(ctx)
	return err
}

func (r *EntRepository) HardDeleteUser(ctx context.Context, tenantID, userID uuid.UUID) (map[string]int, error) {
	summary := make(map[string]int)

	// Delete user's carts
	n, err := r.client.Cart.Delete().
		Where(
			cart.TenantID(tenantID),
			cart.UserID(userID),
		).
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	summary["carts"] = n

	// Delete user's addresses
	n, err = r.client.CustomerAddress.Delete().
		Where(
			customeraddress.TenantID(tenantID),
			customeraddress.UserID(userID),
		).
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	summary["addresses"] = n

	// Delete user's payment methods
	n, err = r.client.PaymentMethod.Delete().
		Where(
			paymentmethod.TenantID(tenantID),
			paymentmethod.UserID(userID),
		).
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	summary["payment_methods"] = n

	// Note: Orders are typically retained for legal/accounting purposes
	// They should be anonymized rather than deleted
	summary["orders"] = 0 // Orders not deleted, just anonymized

	// Finally delete the user
	err = r.client.User.DeleteOneID(userID).Exec(ctx)
	if err != nil {
		return nil, err
	}
	summary["users"] = 1

	return summary, nil
}

// --- Converters ---

func entRequestToModel(r *ent.DataSubjectRequest) *DataSubjectRequest {
	return &DataSubjectRequest{
		ID:          r.ID,
		TenantID:    r.TenantID,
		UserID:      r.UserID,
		RequestType: DataSubjectRequestType(r.RequestType),
		Status:      DataSubjectRequestStatus(r.Status),
		Description: r.Description,
		Notes:       r.Notes,
		ResultURL:   r.ResultURL,
		SubmittedAt: r.SubmittedAt,
		ProcessedAt: r.ProcessedAt,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func entExportJobToModel(j *ent.DataExportJob) *DataExportJob {
	return &DataExportJob{
		ID:              j.ID,
		TenantID:        j.TenantID,
		UserID:          j.UserID,
		Format:          ExportFormat(j.Format),
		Status:          ExportJobStatus(j.Status),
		IncludedData:    j.IncludedData,
		StorageURL:      j.StorageURL,
		ErrorMessage:    j.ErrorMessage,
		FileSizeBytes:   j.FileSizeBytes,
		RecordsExported: j.RecordsExported,
		RequestedAt:     j.RequestedAt,
		StartedAt:       j.StartedAt,
		CompletedAt:     j.CompletedAt,
		ExpiresAt:       j.ExpiresAt,
		CreatedAt:       j.CreatedAt,
		UpdatedAt:       j.UpdatedAt,
	}
}

func entDeletionJobToModel(j *ent.DataDeletionJob) *DataDeletionJob {
	return &DataDeletionJob{
		ID:              j.ID,
		TenantID:        j.TenantID,
		UserID:          j.UserID,
		DeletionType:    DeletionType(j.DeletionType),
		Status:          DeletionJobStatus(j.Status),
		Reason:          j.Reason,
		Confirmed:       j.Confirmed,
		RetentionDays:   j.RetentionDays,
		ErrorMessage:    j.ErrorMessage,
		DeletionSummary: j.DeletionSummary,
		RequestedAt:     j.RequestedAt,
		ScheduledFor:    j.ScheduledFor,
		StartedAt:       j.StartedAt,
		CompletedAt:     j.CompletedAt,
		CreatedAt:       j.CreatedAt,
		UpdatedAt:       j.UpdatedAt,
	}
}
