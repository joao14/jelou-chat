package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cinebot/internal/config"
	"cinebot/internal/notify"
	"cinebot/internal/payment"
	"cinebot/internal/repository/postgres"
	"cinebot/internal/service"
	"cinebot/internal/transport/httpapi"
	"cinebot/migrations"
)

func main() {
	migrateOnly := flag.Bool("migrate-only", false, "aplica migraciones y termina")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if cfg.AutoMigrate || *migrateOnly {
		if err := postgres.Migrate(ctx, pool, migrations.FS); err != nil {
			log.Error("migraciones", "err", err)
			os.Exit(1)
		}
		log.Info("migraciones aplicadas")
		if *migrateOnly {
			return
		}
	}
	repo := postgres.New(pool)
	if cfg.Seed {
		if err := repo.Seed(ctx); err != nil {
			log.Error("seed", "err", err)
			os.Exit(1)
		}
	}

	var gateway service.PaymentGateway
	mock := cfg.PayPhoneToken == ""
	if mock {
		log.Warn("PAYPHONE_TOKEN vacío: pagos en modo MOCK")
		gateway = payment.NewMock(cfg.BaseURL)
	} else {
		var fallback payment.Gateway
		if cfg.FallbackMock {
			log.Warn("PAYMENT_FALLBACK_MOCK=true: si PayPhone falla se ofrecerá el link MOCK (solo demo)")
			fallback = payment.NewMock(cfg.BaseURL)
		}
		gateway = payment.NewResilient(payment.NewPayPhone(cfg.PayPhoneToken, cfg.PayPhoneStore, cfg.BaseURL), fallback, log)
	}
	var notifier service.Notifier = notify.Noop{}
	if cfg.NotifyURL != "" {
		notifier = notify.NewWebhook(cfg.NotifyURL, log)
	}

	svc := service.NewBooking(repo, repo, gateway, notifier, cfg.HoldDuration)
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewRouter(svc, httpapi.Options{APIKey: cfg.APIKey, MockPayments: mock || cfg.FallbackMock, Logger: log}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Info("CineBot listo", "addr", srv.Addr, "base_url", cfg.BaseURL)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}
