package payment

import (
	"context"
	"log/slog"
	"sync"

	"cinebot/internal/domain"
)

// Gateway es lo que necesita el servicio de una pasarela (mismo contrato que service.PaymentGateway).
type Gateway interface {
	Prepare(ctx context.Context, r domain.Reservation, description string) (string, error)
	Confirm(ctx context.Context, paymentID int64, clientTxID string) (bool, error)
}

// Resilient usa la pasarela real y, solo si se configuró un respaldo (demo), cae a él cuando la real
// falla tras sus reintentos. Recuerda qué reservas salieron por el respaldo para confirmarlas ahí.
type Resilient struct {
	primary, fallback Gateway
	log               *slog.Logger
	mu                sync.Mutex
	viaFallback       map[string]bool
}

func NewResilient(primary, fallback Gateway, log *slog.Logger) *Resilient {
	if log == nil {
		log = slog.Default()
	}
	return &Resilient{primary: primary, fallback: fallback, log: log, viaFallback: map[string]bool{}}
}

func (r *Resilient) Prepare(ctx context.Context, res domain.Reservation, description string) (string, error) {
	url, err := r.primary.Prepare(ctx, res, description)
	if err == nil || r.fallback == nil {
		return url, err
	}
	r.log.Warn("pasarela principal falló; usando respaldo", "reservation", res.ID, "err", err)
	furl, ferr := r.fallback.Prepare(ctx, res, description)
	if ferr != nil {
		return "", err
	}
	r.mu.Lock()
	r.viaFallback[res.ID] = true
	r.mu.Unlock()
	return furl, nil
}

func (r *Resilient) Confirm(ctx context.Context, paymentID int64, clientTxID string) (bool, error) {
	r.mu.Lock()
	fb := r.viaFallback[clientTxID]
	r.mu.Unlock()
	if fb && r.fallback != nil {
		return r.fallback.Confirm(ctx, paymentID, clientTxID)
	}
	return r.primary.Confirm(ctx, paymentID, clientTxID)
}
