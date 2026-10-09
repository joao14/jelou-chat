-- Supabase expone el esquema public vía PostgREST. Activamos RLS sin políticas
-- para que la API pública no pueda leer/escribir. Nuestro backend se conecta con
-- el rol postgres (owner), que no se ve afectado. En Postgres local es inocuo.
ALTER TABLE movies            ENABLE ROW LEVEL SECURITY;
ALTER TABLE showtimes         ENABLE ROW LEVEL SECURITY;
ALTER TABLE reservations      ENABLE ROW LEVEL SECURITY;
ALTER TABLE reservation_seats ENABLE ROW LEVEL SECURITY;
