// Package service orquesta los casos de uso de reserva y pago.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"cinebot/internal/domain"
)

const MaxSeats = 6

var seatRe = regexp.MustCompile(`^[A-Z][0-9]{1,2}$`)

type Booking struct {
	catalog  CatalogRepo
	res      ReservationRepo
	pay      PaymentGateway
	notifier Notifier
	hold     time.Duration
}

func NewBooking(c CatalogRepo, r ReservationRepo, p PaymentGateway, n Notifier, hold time.Duration) *Booking {
	return &Booking{catalog: c, res: r, pay: p, notifier: n, hold: hold}
}

// Details une la reserva con su función/película para presentarla.
type Details struct {
	Reservation domain.Reservation
	Showtime    domain.Showtime
}

type SeatMap struct {
	ShowtimeID string
	Available  []string
	Taken      []string
}

type ReserveInput struct {
	ShowtimeID string
	Seats      []string
	Phone      string
	Name       string
}

func (b *Booking) Movies(ctx context.Context) ([]domain.Movie, error) { return b.catalog.Movies(ctx) }

func (b *Booking) Showtimes(ctx context.Context, movieID string) ([]domain.Showtime, error) {
	return b.catalog.Showtimes(ctx, movieID)
}

func (b *Booking) Seats(ctx context.Context, showtimeID string) (*SeatMap, error) {
	st, err := b.catalog.Showtime(ctx, showtimeID)
	if err != nil {
		return nil, err
	}
	taken, err := b.res.TakenSeats(ctx, showtimeID)
	if err != nil {
		return nil, err
	}
	isTaken := make(map[string]bool, len(taken))
	for _, s := range taken {
		isTaken[s] = true
	}
	sm := &SeatMap{ShowtimeID: showtimeID, Available: []string{}, Taken: []string{}}
	for _, s := range st.AllSeats() {
		if isTaken[s] {
			sm.Taken = append(sm.Taken, s)
		} else {
			sm.Available = append(sm.Available, s)
		}
	}
	return sm, nil
}

func (b *Booking) Reserve(ctx context.Context, in ReserveInput) (*Details, error) {
	st, err := b.catalog.Showtime(ctx, in.ShowtimeID)
	if err != nil {
		return nil, err
	}
	phone := strings.TrimSpace(in.Phone)
	if len(digits(phone)) < 9 {
		return nil, fmt.Errorf("%w: phone inválido", domain.ErrInvalid)
	}
	if len(in.Seats) == 0 || len(in.Seats) > MaxSeats {
		return nil, fmt.Errorf("%w: elige entre 1 y %d asientos", domain.ErrInvalid, MaxSeats)
	}
	valid := map[string]bool{}
	for _, s := range st.AllSeats() {
		valid[s] = true
	}
	seen := map[string]bool{}
	seats := make([]string, 0, len(in.Seats))
	for _, raw := range in.Seats {
		s := strings.ToUpper(strings.TrimSpace(raw))
		if !seatRe.MatchString(s) || !valid[s] {
			return nil, fmt.Errorf("%w: el asiento %q no existe en esta sala", domain.ErrInvalid, raw)
		}
		if !seen[s] {
			seen[s] = true
			seats = append(seats, s)
		}
	}
	sort.Strings(seats)

	r, err := b.res.Hold(ctx, domain.Reservation{
		ID: newID(), ShowtimeID: st.ID, Seats: seats, Phone: phone, Name: strings.TrimSpace(in.Name),
		AmountCents: st.PriceCents * len(seats), Status: domain.StatusHeld,
	}, b.hold)
	if err != nil {
		return nil, err
	}
	return &Details{Reservation: *r, Showtime: *st}, nil
}

func (b *Booking) Get(ctx context.Context, id string) (*Details, error) {
	r, err := b.res.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return b.details(ctx, r)
}

func (b *Booking) CreatePayment(ctx context.Context, id string) (*Details, error) {
	r, err := b.res.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if r.Status == domain.StatusPaid {
		return nil, domain.ErrAlreadyPaid
	}
	if r.Status != domain.StatusHeld {
		return nil, domain.ErrNotHeld
	}
	url, err := b.pay.Prepare(ctx, *r, fmt.Sprintf("CineBot %s asientos %s", r.ShowtimeID, strings.Join(r.Seats, ",")))
	if err != nil {
		return nil, fmt.Errorf("pasarela de pago: %w", err)
	}
	if err := b.res.SetPayURL(ctx, r.ID, url); err != nil {
		return nil, err
	}
	r.PayURL = url
	return b.details(ctx, r)
}

// ConfirmPayment se invoca desde el callback de la pasarela. La pasarela es la fuente de verdad.
func (b *Booking) ConfirmPayment(ctx context.Context, paymentID int64, clientTxID string) (*Details, error) {
	if _, err := b.res.Get(ctx, clientTxID); err != nil {
		return nil, err
	}
	approved, err := b.pay.Confirm(ctx, paymentID, clientTxID)
	if err != nil {
		return nil, fmt.Errorf("pasarela de pago: %w", err)
	}
	if !approved {
		return nil, domain.ErrPaymentDeclined
	}
	r, changed, err := b.res.MarkPaid(ctx, clientTxID, fmt.Sprint(paymentID))
	if err != nil {
		return nil, err
	}
	d, err := b.details(ctx, r)
	if err != nil {
		return nil, err
	}
	if changed {
		go b.notifier.PaymentApproved(context.WithoutCancel(ctx), d)
	}
	return d, nil
}

func (b *Booking) Cancel(ctx context.Context, id string) (*Details, error) {
	r, err := b.res.Cancel(ctx, id)
	if err != nil {
		return nil, err
	}
	return b.details(ctx, r)
}

func (b *Booking) details(ctx context.Context, r *domain.Reservation) (*Details, error) {
	st, err := b.catalog.Showtime(ctx, r.ShowtimeID)
	if err != nil {
		return nil, err
	}
	return &Details{Reservation: *r, Showtime: *st}, nil
}

func newID() string {
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	return "R" + strings.ToUpper(hex.EncodeToString(buf))
}

func digits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}
