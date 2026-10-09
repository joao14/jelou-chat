// Package domain contiene las entidades y errores de negocio. No depende de nada externo.
package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound        = errors.New("no encontrado")
	ErrInvalid         = errors.New("solicitud inválida")
	ErrSeatTaken       = errors.New("uno o más asientos ya no están disponibles")
	ErrNotHeld         = errors.New("la reserva no está retenida (pudo expirar o cancelarse)")
	ErrAlreadyPaid     = errors.New("la reserva ya está pagada")
	ErrNeedsRefund     = errors.New("pago aprobado pero los asientos fueron tomados: requiere reembolso")
	ErrPaymentDeclined = errors.New("pago no aprobado")
)

const (
	StatusHeld      = "HELD"
	StatusPaid      = "PAID"
	StatusExpired   = "EXPIRED"
	StatusCancelled = "CANCELLED"
)

type Movie struct {
	ID          string
	Title       string
	Genre       string
	DurationMin int
	Rating      string
}

type Showtime struct {
	ID         string
	MovieID    string
	MovieTitle string
	Hall       string
	StartsAt   time.Time
	PriceCents int
	RowLabels  string
	Cols       int
}

// AllSeats devuelve las etiquetas de asientos en orden: A1..A8, B1..B8, ...
func (s Showtime) AllSeats() []string {
	out := make([]string, 0, len(s.RowLabels)*s.Cols)
	for _, r := range s.RowLabels {
		for c := 1; c <= s.Cols; c++ {
			out = append(out, fmt.Sprintf("%c%d", r, c))
		}
	}
	return out
}

type Reservation struct {
	ID          string
	ShowtimeID  string
	Seats       []string
	Phone       string
	Name        string
	AmountCents int
	Status      string
	PayURL      string
	PaymentRef  string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	PaidAt      *time.Time
}
