package identity

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// MemoryRepository provides an in-memory Repository implementation.
type MemoryRepository struct {
	users                map[uuid.UUID]*User
	usersByEmail         map[string]*User
	usersByAuthServiceID map[uuid.UUID]*User
	tenants              map[string]*Tenant
	sessions             map[uuid.UUID]*Session
	orders               map[uuid.UUID][]*OrderSummary
	mu                   sync.RWMutex
}

// NewMemoryRepository constructs an empty repository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		users:                make(map[uuid.UUID]*User),
		usersByEmail:         make(map[string]*User),
		usersByAuthServiceID: make(map[uuid.UUID]*User),
		tenants:              make(map[string]*Tenant),
		sessions:             make(map[uuid.UUID]*Session),
		orders:               make(map[uuid.UUID][]*OrderSummary),
	}
}

// Seed registers initial users, sessions, and orders.
func (r *MemoryRepository) Seed(_ context.Context, users []*User, sessions []*Session, orders []*OrderSummary) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, user := range users {
		u := *user
		r.users[u.ID] = &u
		r.usersByEmail[normalizeEmail(u.Email)] = &u
		if u.AuthServiceUserID != nil {
			r.usersByAuthServiceID[*u.AuthServiceUserID] = &u
		}
	}

	for _, session := range sessions {
		s := *session
		r.sessions[s.ID] = &s
	}

	for _, order := range orders {
		o := *order
		userOrders := r.orders[o.UserID]
		userOrders = append(userOrders, &o)
		r.orders[o.UserID] = userOrders
	}

	return nil
}

// CreateUser stores the user.
func (r *MemoryRepository) CreateUser(_ context.Context, user *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.users[user.ID]; exists {
		return ErrInvalidCredentials
	}
	cpy := *user
	r.users[cpy.ID] = &cpy
	r.usersByEmail[normalizeEmail(cpy.Email)] = &cpy
	if cpy.AuthServiceUserID != nil {
		r.usersByAuthServiceID[*cpy.AuthServiceUserID] = &cpy
	}
	return nil
}

// UpdateUser updates an existing user.
func (r *MemoryRepository) UpdateUser(_ context.Context, user *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.users[user.ID]; !exists {
		return ErrUserNotFound
	}
	cpy := *user
	r.users[cpy.ID] = &cpy
	r.usersByEmail[normalizeEmail(cpy.Email)] = &cpy
	if cpy.AuthServiceUserID != nil {
		r.usersByAuthServiceID[*cpy.AuthServiceUserID] = &cpy
	}
	return nil
}

// FindUserByEmail returns a user by email address.
func (r *MemoryRepository) FindUserByEmail(_ context.Context, email string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, ok := r.usersByEmail[normalizeEmail(email)]
	if !ok {
		return nil, ErrUserNotFound
	}
	cpy := *user
	return &cpy, nil
}

// FindUserByID returns a user by identifier.
func (r *MemoryRepository) FindUserByID(_ context.Context, id uuid.UUID) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, ok := r.users[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	cpy := *user
	return &cpy, nil
}

// FindUserByAuthServiceID returns a user by auth-service user ID.
func (r *MemoryRepository) FindUserByAuthServiceID(_ context.Context, authServiceUserID uuid.UUID) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, ok := r.usersByAuthServiceID[authServiceUserID]
	if !ok {
		return nil, ErrUserNotFound
	}
	cpy := *user
	return &cpy, nil
}

// ListUsers returns all users.
func (r *MemoryRepository) ListUsers(_ context.Context) ([]*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*User, 0, len(r.users))
	for _, user := range r.users {
		cpy := *user
		result = append(result, &cpy)
	}
	return result, nil
}

// CreateSession stores a session.
func (r *MemoryRepository) CreateSession(_ context.Context, session *Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.sessions[session.ID]; ok {
		return ErrInvalidCredentials
	}
	cpy := *session
	r.sessions[cpy.ID] = &cpy
	return nil
}

// UpdateSession updates a session.
func (r *MemoryRepository) UpdateSession(_ context.Context, session *Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.sessions[session.ID]; !ok {
		return ErrSessionNotFound
	}
	cpy := *session
	r.sessions[cpy.ID] = &cpy
	return nil
}

// FindSessionByID retrieves a session by ID.
func (r *MemoryRepository) FindSessionByID(_ context.Context, id uuid.UUID) (*Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	session, ok := r.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	cpy := *session
	return &cpy, nil
}

// FindSessionByToken retrieves a session by refresh token.
func (r *MemoryRepository) FindSessionByToken(_ context.Context, refreshToken string) (*Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, session := range r.sessions {
		if session.RefreshToken == refreshToken {
			cpy := *session
			return &cpy, nil
		}
	}
	return nil, ErrSessionNotFound
}

// DeleteSession deletes a session.
func (r *MemoryRepository) DeleteSession(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.sessions[id]; !ok {
		return ErrSessionNotFound
	}
	delete(r.sessions, id)
	return nil
}

// DeleteSessionsByUser deletes all sessions for a user.
func (r *MemoryRepository) DeleteSessionsByUser(_ context.Context, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, session := range r.sessions {
		if session.UserID == userID {
			delete(r.sessions, id)
		}
	}
	return nil
}

// ListOrdersByUser returns order summaries for a user.
func (r *MemoryRepository) ListOrdersByUser(_ context.Context, userID uuid.UUID) ([]*OrderSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	orders, ok := r.orders[userID]
	if !ok {
		return []*OrderSummary{}, nil
	}

	result := make([]*OrderSummary, 0, len(orders))
	for _, order := range orders {
		cpy := *order
		result = append(result, &cpy)
	}
	return result, nil
}

// FindTenantBySlug finds a tenant by its slug.
func (r *MemoryRepository) FindTenantBySlug(_ context.Context, slug string) (*Tenant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tenant, ok := r.tenants[slug]
	if !ok {
		return nil, fmt.Errorf("identity: tenant not found: %s", slug)
	}
	cpy := *tenant
	return &cpy, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
