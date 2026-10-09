package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"time"

	"cinebot/internal/domain"
	"cinebot/internal/service"
)

// Ecuador no tiene horario de verano: UTC-5 fijo, sin depender de tzdata.
var ecuador = time.FixedZone("ECT", -5*60*60)

const localLayout = "2006-01-02 15:04"

func money(cents int) string { return fmt.Sprintf("$%d.%02d", cents/100, cents%100) }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// statusFor traduce errores de dominio a códigos HTTP.
func statusFor(err error) int {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrSeatTaken),
		errors.Is(err, domain.ErrNotHeld),
		errors.Is(err, domain.ErrAlreadyPaid),
		errors.Is(err, domain.ErrNeedsRefund):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// fail responde con el código adecuado; los errores internos no filtran detalles al cliente.
func (s *Server) fail(w http.ResponseWriter, err error) {
	code := statusFor(err)
	if code == http.StatusInternalServerError {
		s.log.Error("error interno", "err", err)
		writeErr(w, code, "error interno")
		return
	}
	writeErr(w, code, err.Error())
}

func page(w http.ResponseWriter, code int, title, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><meta name=viewport content="width=device-width,initial-scale=1"><body style="font-family:system-ui;max-width:420px;margin:15vh auto;padding:0 20px;text-align:center"><h2>%s</h2><p>%s</p></body>`,
		html.EscapeString(title), html.EscapeString(msg))
}

type movieView struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Genre       string `json:"genre"`
	DurationMin int    `json:"duration_min"`
	Rating      string `json:"rating"`
}

func movieJSON(m domain.Movie) movieView {
	return movieView{ID: m.ID, Title: m.Title, Genre: m.Genre, DurationMin: m.DurationMin, Rating: m.Rating}
}

type showtimeView struct {
	ID            string    `json:"id"`
	MovieID       string    `json:"movie_id"`
	Title         string    `json:"title"`
	Hall          string    `json:"hall"`
	StartsAt      time.Time `json:"starts_at"`
	StartsAtLocal string    `json:"starts_at_local"`
	Price         string    `json:"price"`
	PriceCents    int       `json:"price_cents"`
}

func showtimeJSON(st domain.Showtime) showtimeView {
	return showtimeView{
		ID: st.ID, MovieID: st.MovieID, Title: st.MovieTitle, Hall: st.Hall,
		StartsAt: st.StartsAt, StartsAtLocal: st.StartsAt.In(ecuador).Format(localLayout),
		Price: money(st.PriceCents), PriceCents: st.PriceCents,
	}
}

type seatMapView struct {
	ShowtimeID string   `json:"showtime_id"`
	Available  []string `json:"available"`
	Taken      []string `json:"taken"`
}

type reservationView struct {
	ID            string   `json:"reservation_id"`
	Status        string   `json:"status"`
	Movie         string   `json:"movie"`
	Hall          string   `json:"hall"`
	Showtime      string   `json:"showtime"`
	ShowtimeLocal string   `json:"showtime_local"`
	Seats         []string `json:"seats"`
	Amount        string   `json:"amount"`
	AmountCents   int      `json:"amount_cents"`
	SecondsLeft   int      `json:"seconds_left"`
	ExpiresAt     string   `json:"expires_at"`
	PayURL        string   `json:"pay_url,omitempty"`
	TicketCode    string   `json:"ticket_code,omitempty"`
}

// reservationJSON presenta la reserva para el LLM. Un HELD vencido se reporta como EXPIRED.
func reservationJSON(d *service.Details, now time.Time) reservationView {
	r, st := d.Reservation, d.Showtime
	status := r.Status
	left := 0
	if status == domain.StatusHeld {
		if left = int(r.ExpiresAt.Sub(now).Seconds()); left <= 0 {
			left, status = 0, domain.StatusExpired
		}
	}
	v := reservationView{
		ID: r.ID, Status: status, Movie: st.MovieTitle, Hall: st.Hall,
		Showtime: st.StartsAt.Format(time.RFC3339), ShowtimeLocal: st.StartsAt.In(ecuador).Format(localLayout),
		Seats: r.Seats, Amount: money(r.AmountCents), AmountCents: r.AmountCents,
		SecondsLeft: left, ExpiresAt: r.ExpiresAt.Format(time.RFC3339), PayURL: r.PayURL,
	}
	if status == domain.StatusPaid {
		v.TicketCode = "TKT-" + r.ID
	}
	return v
}
