package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port          string
	BaseURL       string
	DatabaseURL   string
	APIKey        string
	PayPhoneToken string
	PayPhoneStore string
	NotifyURL     string
	AutoMigrate   bool
	Seed          bool
	HoldDuration  time.Duration
}

func Load() Config {
	port := get("PORT", "8080")
	return Config{
		Port:          port,
		BaseURL:       strings.TrimRight(get("BASE_URL", "http://localhost:"+port), "/"),
		DatabaseURL:   get("DATABASE_URL", "postgres://cinebot:cinebot@localhost:5432/cinebot?sslmode=disable"),
		APIKey:        os.Getenv("API_KEY"),
		PayPhoneToken: os.Getenv("PAYPHONE_TOKEN"),
		PayPhoneStore: os.Getenv("PAYPHONE_STORE_ID"),
		NotifyURL:     os.Getenv("JELOU_WEBHOOK_URL"),
		AutoMigrate:   getBool("AUTO_MIGRATE", true),
		Seed:          getBool("SEED", true),
		HoldDuration:  time.Duration(getInt("HOLD_MINUTES", 10)) * time.Minute,
	}
}

func get(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func getBool(k string, d bool) bool {
	if v, err := strconv.ParseBool(os.Getenv(k)); err == nil {
		return v
	}
	return d
}

func getInt(k string, d int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return d
}
