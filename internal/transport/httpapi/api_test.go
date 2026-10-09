package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cinebot/internal/notify"
	"cinebot/internal/payment"
	"cinebot/internal/service"
	"cinebot/internal/testutil"
	"cinebot/internal/transport/httpapi"
)

func call(h http.Handler, method, path, key string, body any) (int, map[string]any) {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestHTTPFlow(t *testing.T) {
	repo, _ := testutil.DB(t)
	svc := service.NewBooking(repo, repo, payment.NewMock("http://x"), notify.Noop{}, 10*time.Minute)
	h := httpapi.NewRouter(svc, httpapi.Options{APIKey: "k", MockPayments: true})

	if code, _ := call(h, "GET", "/api/movies", "", nil); code != 401 {
		t.Fatalf("sin API key esperaba 401, got %d", code)
	}
	_, movies := call(h, "GET", "/api/movies", "k", nil)
	if len(movies["movies"].([]any)) != 3 {
		t.Fatalf("películas: %v", movies)
	}
	_, sts := call(h, "GET", "/api/showtimes?movie_id=m1", "k", nil)
	st := sts["showtimes"].([]any)[0].(map[string]any)["id"].(string)

	body := map[string]any{"showtime_id": st, "seats": []string{"A3", "A4"}, "phone": "+593991234567", "name": "Alex"}
	code, res := call(h, "POST", "/api/reservations", "k", body)
	if code != 201 || res["status"] != "HELD" {
		t.Fatalf("reservar: %d %v", code, res)
	}
	id := res["reservation_id"].(string)
	if code, _ := call(h, "POST", "/api/reservations", "k", body); code != 409 {
		t.Fatalf("doble reserva esperaba 409, got %d", code)
	}
	if code, res = call(h, "POST", "/api/reservations/"+id+"/payment", "k", nil); code != 200 || res["pay_url"] == "" {
		t.Fatalf("pago: %d %v", code, res)
	}
	if code, _ := call(h, "GET", "/payphone/callback?id=1&clientTransactionId="+id, "", nil); code != 200 {
		t.Fatalf("callback: %d", code)
	}
	_, res = call(h, "GET", "/api/reservations/"+id, "k", nil)
	if res["status"] != "PAID" || res["ticket_code"] != "TKT-"+id {
		t.Fatalf("no quedó pagada: %v", res)
	}
	if code, _ := call(h, "GET", "/api/reservations/NOPE", "k", nil); code != 404 {
		t.Fatalf("esperaba 404, got %d", code)
	}
}

func TestDocsServed(t *testing.T) {
	repo, _ := testutil.DB(t)
	svc := service.NewBooking(repo, repo, payment.NewMock("http://x"), notify.Noop{}, 10*time.Minute)
	h := httpapi.NewRouter(svc, httpapi.Options{APIKey: "k"})
	for _, path := range []string{"/docs", "/openapi.yaml"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 || rec.Body.Len() == 0 {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
}
