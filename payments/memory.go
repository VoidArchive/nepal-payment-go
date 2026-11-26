package payments

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
)

type memoryStore struct {
	mu       sync.Mutex
	payments map[string]*Payment
}

// NewMemoryStore returns an in-memory Store for tests and development.
// It returns copies and never aliases stored records.
func NewMemoryStore() Store {
	return &memoryStore{payments: map[string]*Payment{}}
}

func clone(p *Payment) *Payment {
	c := *p
	c.Checkout.FormFields = maps.Clone(p.Checkout.FormFields)
	return &c
}

// active returns a copy of the order's active payment, or nil. m.mu must be held.
func (m *memoryStore) active(orderID, provider string) *Payment {
	for _, q := range m.payments {
		if q.OrderID == orderID && (q.Status == StatusPaid || q.Status == StatusPending && q.Provider == provider) {
			return clone(q)
		}
	}
	return nil
}

func (m *memoryStore) Create(_ context.Context, p *Payment) (*Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if q := m.active(p.OrderID, p.Provider); q != nil {
		return q, nil
	}
	m.payments[p.ID] = clone(p)
	return nil, nil
}

func (m *memoryStore) FindActive(_ context.Context, orderID, provider string) (*Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active(orderID, provider), nil
}

func (m *memoryStore) Get(_ context.Context, id string) (*Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.payments[id]
	if !ok {
		return nil, ErrNotFound
	}
	return clone(p), nil
}

func (m *memoryStore) Update(_ context.Context, p *Payment, from Status) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.payments[p.ID]
	if !ok {
		return false, ErrNotFound
	}
	if cur.Status != from {
		return false, nil
	}
	m.payments[p.ID] = clone(p)
	return true, nil
}

func (m *memoryStore) ListDue(_ context.Context, now time.Time, limit int) ([]*Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var due []*Payment
	for _, p := range m.payments {
		if p.Status == StatusPending && !p.NextCheckAt.After(now) {
			due = append(due, clone(p))
		}
	}
	slices.SortFunc(due, func(a, b *Payment) int { return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), strings.Compare(a.ID, b.ID)) })
	return due[:min(limit, len(due))], nil
}
