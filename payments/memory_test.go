package payments_test

import (
	"testing"

	"github.com/voidarchive/nepal-payment-go/v2/payments"
	"github.com/voidarchive/nepal-payment-go/v2/payments/storetest"
)

func TestMemoryStore(t *testing.T) {
	storetest.Run(t, payments.NewMemoryStore)
}
