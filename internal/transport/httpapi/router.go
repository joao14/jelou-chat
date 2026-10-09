// Package httpapi expone el servicio por HTTP/JSON (tools de Brain Studio) y las páginas de pago.
package httpapi

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"time"

	"cinebot/internal/service"
)

type Options struct {
	APIKey       string
	MockPayments bool
	Logger       *slog.Logger
}

type Server struct {
	svc  *service.Booking
	opts Options
	log  *slog.Logger
}

func NewRouter(svc *service.Booking, opts Options) http.Handler {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	s := &Server{svc: svc, opts: opts, log: log}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Documentación interactiva (Swagger UI) y spec OpenAPI importable en Brain Studio
	mux.HandleFunc("GET /docs", s.docs)
	mux.HandleFunc("GET /openapi.yaml", s.openapi)

	// API para las tools del agente
	mux.HandleFunc("GET /api/movies", s.auth(s.listMovies))
	mux.HandleFunc("GET /api/showtimes", s.auth(s.listShowtimes))
	mux.HandleFunc("GET /api/showtimes/{id}/seats", s.auth(s.seats))
	mux.HandleFunc("POST /api/reservations", s.auth(s.createReservation))
	mux.HandleFunc("GET /api/reservations/{id}", s.auth(s.getReservation))
	mux.HandleFunc("POST /api/reservations/{id}/payment", s.auth(s.createPayment))
	mux.HandleFunc("POST /api/reservations/{id}/cancel", s.auth(s.cancel))

	// Páginas públicas de pago (redirects de PayPhone)
	mux.HandleFunc("GET /payphone/callback", s.paymentCallback)
	mux.HandleFunc("GET /payphone/cancel", s.paymentCancelled)
	if opts.MockPayments {
		mux.HandleFunc("GET /mock/pay/{id}", s.mockPay)
	}
	return s.logging(mux)
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.opts.APIKey != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-API-Key")), []byte(s.opts.APIKey)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "API key inválida"})
			return
		}
		next(w, r)
	}
}

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := time.Now()
		next.ServeHTTP(w, r)
		s.log.Info("request", "method", r.Method, "path", r.URL.Path, "dur", time.Since(t).Round(time.Millisecond))
	})
}
