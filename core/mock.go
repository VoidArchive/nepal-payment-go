package core

import "context"

// MockProvider implements Provider for testing.
// Set the function fields to control behavior in tests.
type MockProvider struct {
	InitiateFunc func(ctx context.Context, req *PaymentRequest) (*PaymentResponse, error)
	VerifyFunc   func(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error)
}

func (m *MockProvider) InitiatePayment(ctx context.Context, req *PaymentRequest) (*PaymentResponse, error) {
	if m.InitiateFunc != nil {
		return m.InitiateFunc(ctx, req)
	}
	return &PaymentResponse{}, nil
}

func (m *MockProvider) VerifyPayment(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error) {
	if m.VerifyFunc != nil {
		return m.VerifyFunc(ctx, req)
	}
	return &VerifyResponse{Success: true}, nil
}
