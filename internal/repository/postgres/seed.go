package postgres

import (
	"context"
	"fmt"
	"time"
)

// Seed inserta películas y funciones de hoy a +3 días (hora de Ecuador). Es idempotente:
// los IDs de función son determinísticos (película-fecha-hora), así cada arranque "rellena" días nuevos.
func (r *Repo) Seed(ctx context.Context) error {
	movies := []struct {
		id, title, genre, rating string
		dur                      int
	}{
		{"m1", "Dune: Parte Tres", "Ciencia ficción", "PG-13", 155},
		{"m2", "Intensamente 3", "Animación", "TP", 98},
		{"m3", "Misión Imposible: Cuenta Final", "Acción", "PG-13", 148},
	}
	for _, m := range movies {
		if _, err := r.pool.Exec(ctx, `INSERT INTO movies (id, title, genre, duration_min, rating)
			VALUES ($1,$2,$3,$4,$5) ON CONFLICT (id) DO NOTHING`, m.id, m.title, m.genre, m.dur, m.rating); err != nil {
			return err
		}
	}
	ect := time.FixedZone("ECT", -5*3600)
	today := time.Now().In(ect)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, ect)
	hours := []int{16, 19, 21}
	for i, m := range movies {
		for d := 0; d <= 3; d++ {
			day := today.AddDate(0, 0, d)
			for j, h := range hours {
				id := fmt.Sprintf("%s-%s-%d", m.id, day.Format("0102"), h)
				_, err := r.pool.Exec(ctx, `INSERT INTO showtimes (id, movie_id, hall, starts_at, price_cents, row_labels, cols)
					VALUES ($1,$2,$3,$4,$5,'ABCDEF',8) ON CONFLICT (id) DO NOTHING`,
					id, m.id, fmt.Sprintf("Sala %d", 1+(i+j)%4), day.Add(time.Duration(h)*time.Hour), 650+100*((i+j)%3))
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}
