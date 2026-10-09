// Package payment contiene adaptadores de pasarelas: PayPhone real y un mock para demos/tests.
package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cinebot/internal/domain"
)

const payphoneBase = "https://pay.payphonetodoesposible.com/api/button"

type PayPhone struct {
	token, storeID, baseURL string
	apiBase                 string // sobreescribible en tests
	client                  *http.Client
	attempts                int
	backoff                 time.Duration
}

func NewPayPhone(token, storeID, baseURL string) *PayPhone {
	return &PayPhone{token: token, storeID: storeID, baseURL: strings.TrimRight(baseURL, "/"),
		apiBase: payphoneBase, attempts: 3, backoff: 500 * time.Millisecond,
		client: &http.Client{Timeout: 10 * time.Second}}
}

// Prepare crea el link de pago. Montos en centavos de USD.
func (p *PayPhone) Prepare(ctx context.Context, r domain.Reservation, description string) (string, error) {
	phone, country := splitPhone(r.Phone)
	body := map[string]any{
		"amount":              r.AmountCents,
		"amountWithoutTax":    r.AmountCents, // precio final; desglosa IVA aquí si lo necesitas
		"amountWithTax":       0,
		"tax":                 0,
		"service":             0,
		"tip":                 0,
		"currency":            "USD",
		"clientTransactionId": r.ID, // máx. 50 caracteres
		"reference":           truncate(description, 100),
		"responseUrl":         p.baseURL + "/payphone/callback",
		"cancellationUrl":     p.baseURL + "/payphone/cancel",
		"phoneNumber":         phone,
		"countryCode":         country,
	}
	if p.storeID != "" {
		body["storeId"] = p.storeID
	}
	var out struct {
		PayWithPayPhone string `json:"payWithPayPhone"`
		PayWithCard     string `json:"payWithCard"`
	}
	if err := p.post(ctx, "/Prepare", body, &out); err != nil {
		return "", err
	}
	switch {
	case out.PayWithPayPhone != "":
		return out.PayWithPayPhone, nil
	case out.PayWithCard != "":
		return out.PayWithCard, nil
	}
	return "", fmt.Errorf("PayPhone no devolvió URL de pago")
}

// Confirm se llama tras el redirect a responseUrl; sin esto el cobro no se finaliza.
func (p *PayPhone) Confirm(ctx context.Context, paymentID int64, clientTxID string) (bool, error) {
	var out struct {
		StatusCode        int    `json:"statusCode"`
		TransactionStatus string `json:"transactionStatus"`
	}
	if err := p.post(ctx, "/V2/Confirm", map[string]any{"id": paymentID, "clientTxId": clientTxID}, &out); err != nil {
		return false, err
	}
	return out.StatusCode == 3 || strings.EqualFold(out.TransactionStatus, "Approved"), nil
}

// post reintenta con backoff exponencial ante errores transitorios (red, 429, 5xx).
// Los 4xx (token/datos inválidos) no se reintentan: fallarían igual.
func (p *PayPhone) post(ctx context.Context, path string, in, out any) error {
	b, _ := json.Marshal(in)
	var lastErr error
	for i := 0; i < p.attempts; i++ {
		if i > 0 {
			select {
			case <-time.After(p.backoff << (i - 1)):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		retry, err := p.postOnce(ctx, path, b, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	return lastErr
}

func (p *PayPhone) postOnce(ctx context.Context, path string, body []byte, out any) (retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiBase+path, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := p.client.Do(req)
	if err != nil {
		return ctx.Err() == nil, err // timeout / red caída
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		transient := res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500
		return transient, fmt.Errorf("payphone %s: HTTP %d %s", path, res.StatusCode, truncate(string(raw), 300))
	}
	return false, json.Unmarshal(raw, out)
}

// splitPhone: "+593991234567" → ("991234567", "593").
func splitPhone(p string) (phone, country string) {
	d := strings.NewReplacer("+", "", " ", "", "-", "").Replace(p)
	if strings.HasPrefix(d, "593") {
		return strings.TrimPrefix(d, "593"), "593"
	}
	return strings.TrimPrefix(d, "0"), "593"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
