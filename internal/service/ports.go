package service

import (
	"context"
	"time"

	"cinebot/internal/domain"
)

// Puertos: el servicio define lo que necesita; los adaptadores (Postgres, PayPhone) los implementan.

type CatalogRepo interface {
	Movies(ctx context.Context) ([]domain.Movie, error)
	Showtimes(ctx context.Context, movieID string) ([]domain.Showtime, error)
	Showtime(ctx context.Context, id string) (*domain.Showtime, error)
}

type ReservationRepo interface {
	// Hold persiste la reserva y sus asientos atómicamente. Devuelve ErrSeatTaken si alguno está ocupado.
	Hold(ctx context.Context, r domain.Reservation, hold time.Duration) (*domain.Reservation, error)
	Get(ctx context.Context, id string) (*domain.Reservation, error)
	TakenSeats(ctx context.Context, showtimeID string) ([]string, error)
	SetPayURL(ctx context.Context, id, url string) error
	// MarkPaid es idempotente; changed=true solo en la transición a PAID.
	MarkPaid(ctx context.Context, id, paymentRef string) (r *domain.Reservation, changed bool, err error)
	Cancel(ctx context.Context, id string) (*domain.Reservation, error)
}

type PaymentGateway interface {
	Prepare(ctx context.Context, r domain.Reservation, description string) (payURL string, err error)
	Confirm(ctx context.Context, paymentID int64, clientTxID string) (approved bool, err error)
}

type Notifier interface {
	PaymentApproved(ctx context.Context, d *Details)
}
