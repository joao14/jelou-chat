package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cinebot/internal/domain"
	"cinebot/internal/service"
)

func (s *Server) listMovies(w http.ResponseWriter, r *http.Request) {
	movies, err := s.svc.Movies(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]movieView, 0, len(movies))
	for _, m := range movies {
		out = append(out, movieJSON(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"movies": out})
}

func (s *Server) listShowtimes(w http.ResponseWriter, r *http.Request) {
	sts, err := s.svc.Showtimes(r.Context(), r.URL.Query().Get("movie_id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]showtimeView, 0, len(sts))
	for _, st := range sts {
		out = append(out, showtimeJSON(st))
	}
	writeJSON(w, http.StatusOK, map[string]any{"showtimes": out})
}

func (s *Server) seats(w http.ResponseWriter, r *http.Request) {
	sm, err := s.svc.Seats(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, seatMapView{ShowtimeID: sm.ShowtimeID, Available: sm.Available, Taken: sm.Taken})
}

type reserveRequest struct {
	ShowtimeID string   `json:"showtime_id"`
	Seats      []string `json:"seats"`
	Phone      string   `json:"phone"`
	Name       string   `json:"name"`
}

func (s *Server) createReservation(w http.ResponseWriter, r *http.Request) {
	var in reserveRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	d, err := s.svc.Reserve(r.Context(), service.ReserveInput{
		ShowtimeID: in.ShowtimeID, Seats: in.Seats, Phone: in.Phone, Name: in.Name,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, reservationJSON(d, time.Now()))
}

func (s *Server) getReservation(w http.ResponseWriter, r *http.Request) {
	s.reservation(w, r, s.svc.Get)
}

func (s *Server) createPayment(w http.ResponseWriter, r *http.Request) {
	s.reservation(w, r, s.svc.CreatePayment)
}

func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	s.reservation(w, r, s.svc.Cancel)
}

// reservation resuelve los endpoints que reciben {id} y devuelven una reserva.
func (s *Server) reservation(w http.ResponseWriter, r *http.Request, op func(ctx context.Context, id string) (*service.Details, error)) {
	d, err := op(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reservationJSON(d, time.Now()))
}

// paymentCallback: PayPhone redirige con ?id=<paymentId>&clientTransactionId=<reserva>.
func (s *Server) paymentCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	txID := q.Get("clientTransactionId")
	paymentID, err := strconv.ParseInt(q.Get("id"), 10, 64)
	if txID == "" || err != nil {
		page(w, http.StatusBadRequest, "Enlace de pago inválido", "Faltan datos del pago. Vuelve a WhatsApp para intentar de nuevo.")
		return
	}
	d, err := s.svc.ConfirmPayment(r.Context(), paymentID, txID)
	switch {
	case err == nil:
		page(w, http.StatusOK, "¡Pago confirmado! 🎟️",
			"Reserva "+d.Reservation.ID+" · Asientos "+strings.Join(d.Reservation.Seats, ", ")+". Vuelve a WhatsApp: tu ticket ya está listo.")
	case errors.Is(err, domain.ErrNotFound):
		page(w, http.StatusNotFound, "Reserva no encontrada", "No pudimos asociar este pago a una reserva.")
	case errors.Is(err, domain.ErrPaymentDeclined):
		page(w, http.StatusOK, "Pago no aprobado", "No se realizó ningún cobro. Vuelve a WhatsApp para intentar de nuevo.")
	case errors.Is(err, domain.ErrNeedsRefund):
		page(w, http.StatusConflict, "Pago recibido, requiere revisión",
			"Tu pago fue aprobado pero los asientos cambiaron. Te contactaremos para el reembolso. Ref: "+txID)
	default:
		s.log.Error("confirmar pago", "err", err, "reservation", txID)
		page(w, http.StatusBadGateway, "No pudimos confirmar el pago", "Si se te cobró, escríbenos por WhatsApp con el código "+txID+".")
	}
}

func (s *Server) paymentCancelled(w http.ResponseWriter, r *http.Request) {
	page(w, http.StatusOK, "Pago cancelado",
		"No se realizó ningún cobro. Tus asientos siguen retenidos unos minutos; vuelve a WhatsApp para continuar.")
}

// mockPay simula la pantalla de PayPhone (solo con MockPayments).
func (s *Server) mockPay(w http.ResponseWriter, r *http.Request) {
	d, err := s.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	id := html.EscapeString(d.Reservation.ID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><meta name=viewport content="width=device-width,initial-scale=1"><body style="font-family:system-ui;max-width:420px;margin:12vh auto;padding:0 20px;text-align:center">
<h2>PayPhone (simulado)</h2><p>Reserva %s<br>Total <b>%s</b></p>
<a href="/payphone/callback?id=1&clientTransactionId=%s" style="display:block;background:#ff6a00;color:#fff;padding:14px;border-radius:10px;text-decoration:none;font-weight:600">Pagar</a>
<p><a href="/payphone/cancel?tx=%s">Cancelar</a></p></body>`, id, money(d.Reservation.AmountCents), id, id)
}
