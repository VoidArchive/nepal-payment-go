// Package storetest is a conformance suite for payments.Store implementations.
package storetest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/voidarchive/nepal-payment-go/v2/core"
	"github.com/voidarchive/nepal-payment-go/v2/payments"
)

// base is whole seconds in UTC so stores that round times still compare equal.
var base = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func payment(id, orderID, provider string, status payments.Status, created time.Time) *payments.Payment {
	return &payments.Payment{
		ID: id, OrderID: orderID, Provider: provider, Amount: 10050, Status: status,
		CreatedAt: created, UpdatedAt: created, NextCheckAt: created.Add(time.Minute),
	}
}

func mustCreate(t *testing.T, s payments.Store, p *payments.Payment) {
	t.Helper()
	existing, err := s.Create(context.Background(), p)
	if err != nil {
		t.Fatalf("Create(%s): %v", p.ID, err)
	}
	if existing != nil {
		t.Fatalf("Create(%s): got existing %s, want insert", p.ID, existing.ID)
	}
}

func mustGet(t *testing.T, s payments.Store, id string) *payments.Payment {
	t.Helper()
	p, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get(%s): %v", id, err)
	}
	return p
}

// Run runs the conformance suite. newStore must return an empty store on each call.
func Run(t *testing.T, newStore func() payments.Store) {
	ctx := context.Background()

	t.Run("RoundTrip", func(t *testing.T) {
		s := newStore()
		want := payment("p1", "o1", "esewa", payments.StatusPending, base)
		want.ProviderStatus = core.StatusPending
		want.ProviderRef = "ref1"
		want.TransactionID = "tx1"
		want.Checks = 3
		want.UpdatedAt = base.Add(time.Second)
		want.NextCheckAt = base.Add(time.Hour)
		want.Checkout = core.PaymentResponse{
			IsFormPOST: true, TargetURL: "https://gw.example/pay", TransactionID: "ref1",
			FormFields: map[string]string{"amount": "100.50", "signature": "abc"},
		}
		mustCreate(t, s, want)
		got := mustGet(t, s, "p1")

		if got.ID != want.ID || got.OrderID != want.OrderID || got.Provider != want.Provider ||
			got.Amount != want.Amount || got.Status != want.Status || got.ProviderStatus != want.ProviderStatus ||
			got.ProviderRef != want.ProviderRef || got.TransactionID != want.TransactionID || got.Checks != want.Checks {
			t.Errorf("got %+v, want %+v", got, want)
		}
		gc, wc := got.Checkout, want.Checkout
		if gc.IsFormPOST != wc.IsFormPOST || gc.TargetURL != wc.TargetURL || gc.TransactionID != wc.TransactionID ||
			fmt.Sprint(gc.FormFields) != fmt.Sprint(wc.FormFields) {
			t.Errorf("Checkout: got %+v, want %+v", gc, wc)
		}
		if !got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) || !got.NextCheckAt.Equal(want.NextCheckAt) {
			t.Errorf("times: got %v %v %v, want %v %v %v",
				got.CreatedAt, got.UpdatedAt, got.NextCheckAt, want.CreatedAt, want.UpdatedAt, want.NextCheckAt)
		}
	})

	t.Run("GetUnknown", func(t *testing.T) {
		if _, err := newStore().Get(ctx, "nope"); !errors.Is(err, payments.ErrNotFound) {
			t.Errorf("got %v, want ErrNotFound", err)
		}
	})

	t.Run("CreateRules", func(t *testing.T) {
		tests := []struct {
			name         string
			oldStatus    payments.Status
			newProvider  string
			wantExisting bool
		}{
			{"pending same provider", payments.StatusPending, "esewa", true},
			{"pending other provider", payments.StatusPending, "khalti", false},
			{"paid same provider", payments.StatusPaid, "esewa", true},
			{"paid other provider", payments.StatusPaid, "khalti", true},
			{"failed", payments.StatusFailed, "esewa", false},
			{"expired", payments.StatusExpired, "esewa", false},
			{"refunded", payments.StatusRefunded, "esewa", false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				s := newStore()
				mustCreate(t, s, payment("old", "o1", "esewa", tt.oldStatus, base))
				mustCreate(t, s, payment("other-order", "o2", tt.newProvider, payments.StatusPending, base))

				existing, err := s.Create(ctx, payment("new", "o1", tt.newProvider, payments.StatusPending, base))
				if err != nil {
					t.Fatal(err)
				}
				_, getErr := s.Get(ctx, "new")
				if tt.wantExisting {
					if existing == nil || existing.ID != "old" {
						t.Errorf("existing = %+v, want old", existing)
					}
					if !errors.Is(getErr, payments.ErrNotFound) {
						t.Errorf("new payment was inserted: Get err %v", getErr)
					}
				} else {
					if existing != nil {
						t.Errorf("existing = %+v, want nil", existing)
					}
					if getErr != nil {
						t.Errorf("new payment not inserted: %v", getErr)
					}
				}
			})
		}
	})

	t.Run("Update", func(t *testing.T) {
		s := newStore()
		mustCreate(t, s, payment("p1", "o1", "esewa", payments.StatusPending, base))

		p := mustGet(t, s, "p1")
		p.Status = payments.StatusPaid
		p.TransactionID = "tx"
		ok, err := s.Update(ctx, p, payments.StatusPending)
		if err != nil || !ok {
			t.Fatalf("Update from pending: ok=%v err=%v", ok, err)
		}
		if got := mustGet(t, s, "p1"); got.Status != payments.StatusPaid || got.TransactionID != "tx" {
			t.Errorf("after update: %+v", got)
		}

		p.Status = payments.StatusFailed
		p.TransactionID = "other"
		ok, err = s.Update(ctx, p, payments.StatusPending)
		if err != nil || ok {
			t.Fatalf("Update from stale status: ok=%v err=%v, want false, nil", ok, err)
		}
		if got := mustGet(t, s, "p1"); got.Status != payments.StatusPaid || got.TransactionID != "tx" {
			t.Errorf("stale update was written: %+v", got)
		}
	})

	t.Run("ListDue", func(t *testing.T) {
		s := newStore()
		now := base.Add(time.Hour)
		due := func(id string, created, next time.Time, status payments.Status) {
			p := payment(id, id, "esewa", status, created)
			p.NextCheckAt = next
			mustCreate(t, s, p)
		}
		due("newer", base.Add(2*time.Minute), now.Add(-time.Minute), payments.StatusPending)
		due("oldest", base, now.Add(-30*time.Minute), payments.StatusPending)
		due("exactly-now", base.Add(time.Minute), now, payments.StatusPending)
		due("future", base, now.Add(time.Second), payments.StatusPending)
		due("paid", base, now.Add(-time.Hour), payments.StatusPaid)
		due("failed", base, now.Add(-time.Hour), payments.StatusFailed)

		got, err := s.ListDue(ctx, now, 10)
		if err != nil {
			t.Fatal(err)
		}
		if ids := idsOf(got); fmt.Sprint(ids) != "[oldest exactly-now newer]" {
			t.Errorf("ListDue = %v, want [oldest exactly-now newer]", ids)
		}
		got, err = s.ListDue(ctx, now, 2)
		if err != nil {
			t.Fatal(err)
		}
		if ids := idsOf(got); fmt.Sprint(ids) != "[oldest exactly-now]" {
			t.Errorf("ListDue limit 2 = %v", ids)
		}
	})

	t.Run("ReturnsCopies", func(t *testing.T) {
		s := newStore()
		p := payment("p1", "o1", "esewa", payments.StatusPending, base)
		p.Checkout.FormFields = map[string]string{"k": "v"}
		mustCreate(t, s, p)
		p.Amount = 1
		p.Checkout.FormFields["k"] = "changed by caller after Create"

		got := mustGet(t, s, "p1")
		got.Amount = 2
		got.Checkout.FormFields["k"] = "changed by caller after Get"

		due, err := s.ListDue(ctx, base.Add(time.Hour), 10)
		if err != nil || len(due) != 1 {
			t.Fatalf("ListDue: %v %v", due, err)
		}
		due[0].Amount = 3
		due[0].Checkout.FormFields["k"] = "changed by caller after ListDue"

		upd := mustGet(t, s, "p1")
		upd.Checks = 1
		if ok, err := s.Update(ctx, upd, payments.StatusPending); !ok || err != nil {
			t.Fatalf("Update: %v %v", ok, err)
		}
		upd.Checkout.FormFields["k"] = "changed by caller after Update"

		got = mustGet(t, s, "p1")
		if got.Amount != 10050 || got.Checkout.FormFields["k"] != "v" {
			t.Errorf("store aliased a caller's record: Amount %d, FormFields %v", got.Amount, got.Checkout.FormFields)
		}
	})

	t.Run("ConcurrentUpdate", func(t *testing.T) {
		s := newStore()
		mustCreate(t, s, payment("p1", "o1", "esewa", payments.StatusPending, base))
		var wins atomic.Int32
		var wg sync.WaitGroup
		for range 20 {
			wg.Go(func() {
				p, err := s.Get(ctx, "p1")
				if err != nil {
					t.Error(err)
					return
				}
				p.Status = payments.StatusPaid
				ok, err := s.Update(ctx, p, payments.StatusPending)
				if err != nil {
					t.Error(err)
				}
				if ok {
					wins.Add(1)
				}
			})
		}
		wg.Wait()
		if wins.Load() != 1 {
			t.Errorf("%d updates won, want 1", wins.Load())
		}
	})

	t.Run("ConcurrentCreate", func(t *testing.T) {
		s := newStore()
		var inserts atomic.Int32
		var wg sync.WaitGroup
		for i := range 20 {
			wg.Go(func() {
				existing, err := s.Create(ctx, payment(fmt.Sprint("p", i), "o1", "esewa", payments.StatusPending, base))
				if err != nil {
					t.Error(err)
				}
				if existing == nil {
					inserts.Add(1)
				}
			})
		}
		wg.Wait()
		if inserts.Load() != 1 {
			t.Errorf("%d creates inserted, want 1", inserts.Load())
		}
	})
}

func idsOf(ps []*payments.Payment) []string {
	ids := make([]string, len(ps))
	for i, p := range ps {
		ids[i] = p.ID
	}
	return ids
}
