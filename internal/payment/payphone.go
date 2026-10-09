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
	client                  *http.Client
}

func NewPayPhone(token, storeID, baseURL string) *PayPhone {
	return &PayPhone{token: token, storeID: storeID, baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{Timeout: 20 * time.Second}}
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

func (p *PayPhone) post(ctx context.Context, path string, in, out any) error {
	b, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, payphoneBase+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("payphone %s: HTTP %d %s", path, res.StatusCode, truncate(string(raw), 300))
	}
	return json.Unmarshal(raw, out)
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
