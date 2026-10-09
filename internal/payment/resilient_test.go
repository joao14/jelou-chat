package payment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"cinebot/internal/domain"
)

func testPayPhone(url string) *PayPhone {
	p := NewPayPhone("tok", "store", "http://cinebot.test")
	p.apiBase = url
	p.backoff = time.Millisecond
	return p
}

var testRes = domain.Reservation{ID: "R1", Phone: "+593991234567", AmountCents: 1500}

// 5xx transitorios se reintentan y al final el link se obtiene.
func TestPayPhoneRetriesTransientErrors(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"payWithPayPhone":"https://pay.example/ok"}`))
	}))
	defer srv.Close()

	url, err := testPayPhone(srv.URL).Prepare(context.Background(), testRes, "x")
	if err != nil || url != "https://pay.example/ok" {
		t.Fatalf("url=%q err=%v", url, err)
	}
	if calls.Load() != 3 {
		t.Fatalf("llamadas=%d, esperaba 3", calls.Load())
	}
}

// Un 401 (token inválido) no se reintenta.
func TestPayPhoneDoesNotRetryClientErrors(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "no autorizado", http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := testPayPhone(srv.URL).Prepare(context.Background(), testRes, "x"); err == nil {
		t.Fatal("esperaba error")
	}
	if calls.Load() != 1 {
		t.Fatalf("llamadas=%d, esperaba 1", calls.Load())
	}
}

// Con respaldo configurado, si PayPhone cae se entrega el link del respaldo y se confirma ahí.
func TestResilientFallsBackAndConfirmsOnFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "caído", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	p := testPayPhone(srv.URL)
	p.attempts = 2

	g := NewResilient(p, NewMock("http://cinebot.test"), nil)
	url, err := g.Prepare(context.Background(), testRes, "x")
	if err != nil || url != "http://cinebot.test/mock/pay/R1" {
		t.Fatalf("url=%q err=%v", url, err)
	}
	if ok, err := g.Confirm(context.Background(), 1, "R1"); err != nil || !ok {
		t.Fatalf("confirmar por respaldo: ok=%v err=%v", ok, err)
	}
}

// Sin respaldo, el error de PayPhone se propaga (no se inventan links).
func TestResilientWithoutFallbackPropagatesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "caído", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	p := testPayPhone(srv.URL)
	p.attempts = 1

	if _, err := NewResilient(p, nil, nil).Prepare(context.Background(), testRes, "x"); err == nil {
		t.Fatal("esperaba error")
	}
}
