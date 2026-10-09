package service_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cinebot/internal/domain"
	"cinebot/internal/notify"
	"cinebot/internal/payment"
	"cinebot/internal/service"
	"cinebot/internal/testutil"
)

func newSvc(t *testing.T) (*service.Booking, string) {
	t.Helper()
	repo, _ := testutil.DB(t)
	svc := service.NewBooking(repo, repo, payment.NewMock("http://x"), notify.Noop{}, 10*time.Minute)
	sts, err := svc.Showtimes(context.Background(), "m1")
	if err != nil || len(sts) == 0 {
		t.Fatalf("sin funciones: %v", err)
	}
	return svc, sts[0].ID
}

func reserve(svc *service.Booking, st, phone string, seats ...string) (*service.Details, error) {
	return svc.Reserve(context.Background(), service.ReserveInput{ShowtimeID: st, Seats: seats, Phone: phone, Name: "Test"})
}

// Muchos clientes pelean por el mismo asiento: exactamente uno debe ganar.
func TestConcurrentHoldOnlyOneWins(t *testing.T) {
	svc, st := newSvc(t)
	var ok, taken atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := reserve(svc, st, "+593991234567", "C4")
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, domain.ErrSeatTaken):
				taken.Add(1)
			default:
				t.Errorf("error inesperado: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || taken.Load() != 24 {
		t.Fatalf("ok=%d taken=%d (esperaba 1 y 24)", ok.Load(), taken.Load())
	}
}

func TestPaymentFlowAndIdempotency(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	d, err := reserve(svc, st, "+593991234567", "a1", "A2", "a1")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Reservation.Seats) != 2 || d.Reservation.AmountCents != 2*d.Showtime.PriceCents {
		t.Fatalf("reserva inesperada: %+v", d.Reservation)
	}
	if d, err = svc.CreatePayment(ctx, d.Reservation.ID); err != nil || d.Reservation.PayURL == "" {
		t.Fatalf("pago: %v", err)
	}
	for i := 0; i < 2; i++ { // el callback puede repetirse
		if d, err = svc.ConfirmPayment(ctx, 1, d.Reservation.ID); err != nil || d.Reservation.Status != domain.StatusPaid {
			t.Fatalf("confirmar #%d: %v", i, err)
		}
	}
	if _, err := svc.Cancel(ctx, d.Reservation.ID); !errors.Is(err, domain.ErrAlreadyPaid) {
		t.Fatalf("cancelar pagada debía fallar: %v", err)
	}
	sm, _ := svc.Seats(ctx, st)
	if len(sm.Taken) != 2 {
		t.Fatalf("taken=%v", sm.Taken)
	}
}

func TestValidation(t *testing.T) {
	svc, st := newSvc(t)
	for name, c := range map[string]struct {
		seats []string
		phone string
	}{
		"asiento inexistente": {[]string{"Z9"}, "+593991234567"},
		"sin asientos":        {nil, "+593991234567"},
		"demasiados":          {[]string{"A1", "A2", "A3", "A4", "A5", "A6", "A7"}, "+593991234567"},
		"teléfono inválido":   {[]string{"A1"}, "123"},
	} {
		if _, err := reserve(svc, st, c.phone, c.seats...); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: esperaba ErrInvalid, got %v", name, err)
		}
	}
}

func TestExpiredHoldReleasesSeats(t *testing.T) {
	repo, pool := testutil.DB(t)
	svc := service.NewBooking(repo, repo, payment.NewMock("http://x"), notify.Noop{}, 10*time.Minute)
	ctx := context.Background()
	sts, _ := svc.Showtimes(ctx, "m1")
	st := sts[0].ID

	d, err := reserve(svc, st, "+593991111111", "B2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE reservations SET expires_at = now() - interval '1 minute' WHERE id = $1`, d.Reservation.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Get(ctx, d.Reservation.ID); got.Reservation.Status != domain.StatusExpired {
		t.Fatalf("estado=%s, esperaba EXPIRED", got.Reservation.Status)
	}
	if _, err := svc.CreatePayment(ctx, d.Reservation.ID); !errors.Is(err, domain.ErrNotHeld) {
		t.Fatalf("pago sobre expirada: %v", err)
	}
	// otro cliente puede tomar el asiento liberado
	d2, err := reserve(svc, st, "+593992222222", "B2")
	if err != nil {
		t.Fatalf("asiento no liberado: %v", err)
	}
	// y si el primero paga tarde, requiere reembolso (no hay doble venta)
	if _, err := svc.ConfirmPayment(ctx, 9, d.Reservation.ID); !errors.Is(err, domain.ErrNeedsRefund) {
		t.Fatalf("pago tardío: %v", err)
	}
	if got, _ := svc.Get(ctx, d2.Reservation.ID); got.Reservation.Status != domain.StatusHeld {
		t.Fatalf("el segundo cliente perdió su hold: %s", got.Reservation.Status)
	}
}

func TestLatePaymentWinsIfSeatsStillFree(t *testing.T) {
	repo, pool := testutil.DB(t)
	svc := service.NewBooking(repo, repo, payment.NewMock("http://x"), notify.Noop{}, 10*time.Minute)
	ctx := context.Background()
	sts, _ := svc.Showtimes(ctx, "m2")
	d, _ := reserve(svc, sts[0].ID, "+593991111111", "D1")
	_, _ = pool.Exec(ctx, `UPDATE reservations SET expires_at = now() - interval '1 minute' WHERE id = $1`, d.Reservation.ID)
	got, err := svc.ConfirmPayment(ctx, 5, d.Reservation.ID)
	if err != nil || got.Reservation.Status != domain.StatusPaid {
		t.Fatalf("pago tardío con asientos libres debía aceptarse: %v", err)
	}
}
