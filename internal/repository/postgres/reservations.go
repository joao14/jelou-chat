package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"cinebot/internal/domain"
)

type queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// El estado HELD vencido se reporta como EXPIRED directamente en SQL (sin depender de un cron).
const selectReservation = `
SELECT r.id, r.showtime_id, r.phone, r.customer_name, r.amount_cents,
       CASE WHEN r.status = 'HELD' AND r.expires_at < now() THEN 'EXPIRED' ELSE r.status END,
       COALESCE(r.pay_url, ''), COALESCE(r.payment_ref, ''), r.created_at, r.expires_at, r.paid_at,
       COALESCE((SELECT array_agg(seat ORDER BY seat) FROM reservation_seats WHERE reservation_id = r.id), '{}')
FROM reservations r WHERE r.id = $1`

func scanReservation(q queryer, ctx context.Context, id string) (*domain.Reservation, error) {
	var r domain.Reservation
	err := q.QueryRow(ctx, selectReservation, id).Scan(&r.ID, &r.ShowtimeID, &r.Phone, &r.Name, &r.AmountCents,
		&r.Status, &r.PayURL, &r.PaymentRef, &r.CreatedAt, &r.ExpiresAt, &r.PaidAt, &r.Seats)
	if notFound(err) {
		return nil, domain.ErrNotFound
	}
	return &r, err
}

func (r *Repo) Get(ctx context.Context, id string) (*domain.Reservation, error) {
	return scanReservation(r.pool, ctx, id)
}

func (r *Repo) TakenSeats(ctx context.Context, showtimeID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT rs.seat FROM reservation_seats rs JOIN reservations r ON r.id = rs.reservation_id
		WHERE rs.showtime_id = $1 AND rs.active
		  AND (r.status = 'PAID' OR (r.status = 'HELD' AND r.expires_at > now()))`, showtimeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Hold: barre holds vencidos de la función, inserta la reserva y sus asientos.
// La exclusión mutua la garantiza el índice único parcial uq_active_seat: si dos
// transacciones piden el mismo asiento, una falla con 23505 → ErrSeatTaken.
func (r *Repo) Hold(ctx context.Context, in domain.Reservation, hold time.Duration) (*domain.Reservation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `UPDATE reservations SET status = 'EXPIRED'
		WHERE showtime_id = $1 AND status = 'HELD' AND expires_at < now()`, in.ShowtimeID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE reservation_seats SET active = false
		WHERE showtime_id = $1 AND active
		  AND reservation_id IN (SELECT id FROM reservations WHERE showtime_id = $1 AND status IN ('EXPIRED','CANCELLED'))`,
		in.ShowtimeID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO reservations
		(id, showtime_id, phone, customer_name, amount_cents, status, expires_at)
		VALUES ($1,$2,$3,$4,$5,'HELD', now() + make_interval(secs => $6))`,
		in.ID, in.ShowtimeID, in.Phone, in.Name, in.AmountCents, hold.Seconds()); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO reservation_seats (reservation_id, showtime_id, seat)
		SELECT $1, $2, unnest($3::text[])`, in.ID, in.ShowtimeID, in.Seats); err != nil {
		if isUniqueViolation(err) {
			return nil, domain.ErrSeatTaken
		}
		return nil, err
	}
	out, err := scanReservation(tx, ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}

func (r *Repo) SetPayURL(ctx context.Context, id, url string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE reservations SET pay_url = $2 WHERE id = $1`, id, url)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return err
}

func (r *Repo) MarkPaid(ctx context.Context, id, ref string) (*domain.Reservation, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM reservations WHERE id = $1 FOR UPDATE`, id).Scan(&status); err != nil {
		if notFound(err) {
			return nil, false, domain.ErrNotFound
		}
		return nil, false, err
	}
	switch status {
	case domain.StatusPaid:
		out, err := scanReservation(tx, ctx, id)
		return out, false, err
	case domain.StatusCancelled:
		return nil, false, domain.ErrNotHeld
	}
	// HELD (aunque venza) o EXPIRED: un pago aprobado gana si los asientos siguen libres.
	if _, err := tx.Exec(ctx, `UPDATE reservation_seats SET active = true WHERE reservation_id = $1`, id); err != nil {
		if isUniqueViolation(err) {
			return nil, false, domain.ErrNeedsRefund
		}
		return nil, false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE reservations SET status = 'PAID', paid_at = now(), payment_ref = $2 WHERE id = $1`, id, ref); err != nil {
		return nil, false, err
	}
	out, err := scanReservation(tx, ctx, id)
	if err != nil {
		return nil, false, err
	}
	return out, true, tx.Commit(ctx)
}

func (r *Repo) Cancel(ctx context.Context, id string) (*domain.Reservation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM reservations WHERE id = $1 FOR UPDATE`, id).Scan(&status); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if status == domain.StatusPaid {
		return nil, domain.ErrAlreadyPaid
	}
	if _, err := tx.Exec(ctx, `UPDATE reservations SET status = 'CANCELLED' WHERE id = $1`, id); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE reservation_seats SET active = false WHERE reservation_id = $1`, id); err != nil {
		return nil, err
	}
	out, err := scanReservation(tx, ctx, id)
	if err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}
