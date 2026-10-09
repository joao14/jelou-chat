-- Esquema inicial de CineBot
CREATE TABLE movies (
    id           text PRIMARY KEY,
    title        text NOT NULL,
    genre        text NOT NULL,
    duration_min int  NOT NULL CHECK (duration_min > 0),
    rating       text NOT NULL
);

CREATE TABLE showtimes (
    id          text PRIMARY KEY,
    movie_id    text        NOT NULL REFERENCES movies(id),
    hall        text        NOT NULL,
    starts_at   timestamptz NOT NULL,
    price_cents int         NOT NULL CHECK (price_cents > 0),
    row_labels  text        NOT NULL,           -- p.ej. 'ABCDEF'
    cols        int         NOT NULL CHECK (cols > 0)
);
CREATE INDEX idx_showtimes_movie_start ON showtimes (movie_id, starts_at);

CREATE TABLE reservations (
    id            text PRIMARY KEY,
    showtime_id   text        NOT NULL REFERENCES showtimes(id),
    phone         text        NOT NULL,
    customer_name text        NOT NULL DEFAULT '',
    amount_cents  int         NOT NULL CHECK (amount_cents > 0),
    status        text        NOT NULL CHECK (status IN ('HELD','PAID','EXPIRED','CANCELLED')),
    pay_url       text,
    payment_ref   text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL,
    paid_at       timestamptz
);
CREATE INDEX idx_reservations_phone  ON reservations (phone);
CREATE INDEX idx_reservations_status ON reservations (status, expires_at);

-- Un asiento por fila. El índice único parcial garantiza a nivel de BD
-- que un asiento activo (HELD/PAID) no puede tener dos dueños.
CREATE TABLE reservation_seats (
    reservation_id text    NOT NULL REFERENCES reservations(id) ON DELETE CASCADE,
    showtime_id    text    NOT NULL REFERENCES showtimes(id),
    seat           text    NOT NULL,
    active         boolean NOT NULL DEFAULT true,
    PRIMARY KEY (reservation_id, seat)
);
CREATE UNIQUE INDEX uq_active_seat ON reservation_seats (showtime_id, seat) WHERE active;
