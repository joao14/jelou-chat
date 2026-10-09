# Tools para Brain Studio (HTTP / API tools)

Base URL: `https://TU-DEPLOY` · Header en todas: `X-API-Key: <API_KEY>` · `Content-Type: application/json`

| Tool | Método | Ruta | Parámetros | Cuándo usarla |
|---|---|---|---|---|
| `list_movies` | GET | `/api/movies` | — | Inicio de la conversación |
| `list_showtimes` | GET | `/api/showtimes?movie_id={movie_id}` | `movie_id` (string, opcional) | Tras elegir película |
| `get_seats` | GET | `/api/showtimes/{showtime_id}/seats` | `showtime_id` (string) | Antes de ofrecer asientos y tras un 409 |
| `create_reservation` | POST | `/api/reservations` | body: `showtime_id` (string), `seats` (array de string, ej. ["A1","A2"]), `phone` (string, número de WhatsApp), `name` (string) | Cliente confirmó el resumen |
| `create_payment` | POST | `/api/reservations/{reservation_id}/payment` | `reservation_id` (string) | Justo después de reservar |
| `get_reservation` | GET | `/api/reservations/{reservation_id}` | `reservation_id` (string) | Verificar si pagó |
| `cancel_reservation` | POST | `/api/reservations/{reservation_id}/cancel` | `reservation_id` (string) | Cliente se arrepiente |

Variables de conversación a guardar: `movie_id`, `showtime_id`, `seats`, `reservation_id`, `phone`, `name`.

Ejemplos de respuesta:
- `create_reservation` → `{"reservation_id":"R3F9A...","status":"HELD","seats":["A1","A2"],"amount":"$14.00","seconds_left":600,"movie":"Dune: Parte Tres"}`
- `create_payment` → mismo objeto + `"pay_url":"https://.../pay..."`
- `get_reservation` pagada → `{"status":"PAID","ticket_code":"TKT-R3F9A...", ...}`

Notas:
- Los `showtime_id` son legibles: `m1-1009-19` = película m1, 9 de octubre, 19:00.
- Para mostrar horas al cliente usa `starts_at_local` / `showtime_local` (hora de Ecuador), no los campos ISO.
- Errores: 400 datos inválidos · 401 API key · 404 no existe · 409 asiento tomado / reserva no retenida (el body trae `{"error": "..."}` en español; léelo y actúa).
