// Package notify avisa a sistemas externos (p. ej. un flujo de Jelou) cuando un pago se aprueba.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"cinebot/internal/service"
)

type Noop struct{}

func (Noop) PaymentApproved(context.Context, *service.Details) {}

type Webhook struct {
	url    string
	client *http.Client
	log    *slog.Logger
}

func NewWebhook(url string, log *slog.Logger) *Webhook {
	return &Webhook{url: url, client: &http.Client{Timeout: 10 * time.Second}, log: log}
}

func (w *Webhook) PaymentApproved(ctx context.Context, d *service.Details) {
	payload, _ := json.Marshal(map[string]any{
		"event":          "payment.approved",
		"reservation_id": d.Reservation.ID,
		"phone":          d.Reservation.Phone,
		"movie":          d.Showtime.MovieTitle,
		"starts_at":      d.Showtime.StartsAt,
		"seats":          d.Reservation.Seats,
		"ticket_code":    "TKT-" + d.Reservation.ID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(payload))
	if err != nil {
		w.log.Error("webhook", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		w.log.Error("webhook", "err", err)
		return
	}
	resp.Body.Close()
}
