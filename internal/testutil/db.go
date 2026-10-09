// Package testutil prepara una BD limpia para tests de integración.
package testutil

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"cinebot/internal/repository/postgres"
	"cinebot/migrations"
)

// DB devuelve un repositorio sobre una BD recién migrada y sembrada.
// Requiere TEST_DATABASE_URL apuntando a una BD cuyo nombre contenga "test" (se borra el esquema public).
func DB(t *testing.T) (*postgres.Repo, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL no definida; test de integración omitido")
	}
	if !strings.Contains(url, "test") {
		t.Fatal("TEST_DATABASE_URL debe apuntar a una BD con 'test' en el nombre")
	}
	ctx := context.Background()
	pool, err := postgres.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	if err := postgres.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatal(err)
	}
	repo := postgres.New(pool)
	if err := repo.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	return repo, pool
}
