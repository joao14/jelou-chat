package payment

import (
	"context"
	"strings"

	"cinebot/internal/domain"
)

// Mock simula la pasarela: el "link de pago" apunta a una pantalla propia (/mock/pay/{id}).
type Mock struct{ baseURL string }

func NewMock(baseURL string) *Mock { return &Mock{baseURL: strings.TrimRight(baseURL, "/")} }

func (m *Mock) Prepare(_ context.Context, r domain.Reservation, _ string) (string, error) {
	return m.baseURL + "/mock/pay/" + r.ID, nil
}

func (m *Mock) Confirm(context.Context, int64, string) (bool, error) { return true, nil }
