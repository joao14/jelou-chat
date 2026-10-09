package postgres

import (
	"context"

	"cinebot/internal/domain"
)

func (r *Repo) Movies(ctx context.Context) ([]domain.Movie, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, title, genre, duration_min, rating FROM movies ORDER BY title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Movie{}
	for rows.Next() {
		var m domain.Movie
		if err := rows.Scan(&m.ID, &m.Title, &m.Genre, &m.DurationMin, &m.Rating); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

const showtimeCols = `s.id, s.movie_id, m.title, s.hall, s.starts_at, s.price_cents, s.row_labels, s.cols`

func (r *Repo) Showtimes(ctx context.Context, movieID string) ([]domain.Showtime, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+showtimeCols+`
		FROM showtimes s JOIN movies m ON m.id = s.movie_id
		WHERE ($1 = '' OR s.movie_id = $1) AND s.starts_at > now()
		ORDER BY s.starts_at`, movieID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Showtime{}
	for rows.Next() {
		var s domain.Showtime
		if err := rows.Scan(&s.ID, &s.MovieID, &s.MovieTitle, &s.Hall, &s.StartsAt, &s.PriceCents, &s.RowLabels, &s.Cols); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repo) Showtime(ctx context.Context, id string) (*domain.Showtime, error) {
	var s domain.Showtime
	err := r.pool.QueryRow(ctx, `SELECT `+showtimeCols+`
		FROM showtimes s JOIN movies m ON m.id = s.movie_id WHERE s.id = $1`, id).
		Scan(&s.ID, &s.MovieID, &s.MovieTitle, &s.Hall, &s.StartsAt, &s.PriceCents, &s.RowLabels, &s.Cols)
	if notFound(err) {
		return nil, domain.ErrNotFound
	}
	return &s, err
}
