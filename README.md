# CineBot — Agente de reservas de cine con pago por WhatsApp

Reto técnico Jelou · AI Solutions Engineer (EC)

```
WhatsApp ⇄ Jelou Brain Studio (agente + tools HTTP) ⇄ API Go ⇄ PostgreSQL (local → Supabase)
                                                         └──⇄ PayPhone (Prepare / Confirm)
```

## Arquitectura (capas)
```
cmd/api/                  entrypoint: wiring, graceful shutdown
internal/config/          variables de entorno
internal/domain/          entidades y errores de negocio (sin dependencias)
internal/service/         casos de uso + puertos (interfaces) ← lógica de negocio
internal/repository/postgres/   adaptador PostgreSQL (pgx): catálogo, reservas, migrador, seed
internal/payment/         adaptadores de pasarela: PayPhone y Mock
internal/notify/          webhook opcional hacia Jelou
internal/transport/httpapi/     handlers HTTP/JSON, auth por API key, páginas de pago
migrations/               SQL versionado (embebido en el binario)
brain/                    prompt del agente, tools y guion de demo
```
El servicio depende de **interfaces**, no de Postgres ni de PayPhone: cambiar de pasarela o de BD no toca la lógica.

## Decisiones clave
- **Sin doble venta, garantizado por la BD**: `reservation_seats` + índice único parcial `(showtime_id, seat) WHERE active`. Si dos clientes piden el mismo asiento, Postgres rechaza al segundo (409). Test: 25 goroutines, gana exactamente 1.
- **Hold de 10 min**: el estado `HELD` vencido se reporta como `EXPIRED` en la propia consulta SQL (sin cron) y los asientos se liberan en el siguiente intento de reserva.
- **El pago lo confirma el servidor**: `PAID` solo se marca tras `Confirm` de PayPhone; idempotente. Pago tardío: se acepta si los asientos siguen libres; si ya los tomaron → `ErrNeedsRefund`, sin doble venta.
- **El agente nunca confía en el usuario** para el estado de pago: consulta `GET /api/reservations/{id}`.
- **Supabase listo**: migración `002` activa RLS (Supabase expone `public` por PostgREST); el backend usa el rol `postgres`.

## Correr local (Postgres en Docker)
```bash
cp .env.example .env
make tidy        # una vez: genera go.sum (commitéalo)
make db          # Postgres 16 + BD de test
make run         # migra, siembra y levanta en :8080  (API_KEY=dev-key)
make test        # tests de integración con -race
```
O todo en contenedores: `make up`.

Prueba rápida:
```bash
curl -H 'X-API-Key: dev-key' localhost:8080/api/movies
curl -H 'X-API-Key: dev-key' "localhost:8080/api/showtimes?movie_id=m1"
curl -H 'X-API-Key: dev-key' -XPOST localhost:8080/api/reservations \
  -d '{"showtime_id":"m1-1009-19","seats":["A1","A2"],"phone":"+593991234567","name":"Alex"}'
```
(El `showtime_id` real sale de `/api/showtimes`; las fechas se generan desde hoy.)

## Migrar a Supabase
1. Crea el proyecto en Supabase y copia la cadena de conexión (Project Settings → Database).
2. Usa el **Session pooler** (puerto 5432) y `sslmode=require` en `DATABASE_URL`. Si usas el *Transaction pooler* (6543) añade `&default_query_exec_mode=simple_protocol`.
3. `DATABASE_URL=... make migrate` aplica `001` y `002`. No hay que cambiar código.

## PayPhone
1. Cuenta en payphone.app → consola de desarrolladores → aplicación tipo **Web**.
2. `PAYPHONE_TOKEN` (y `PAYPHONE_STORE_ID` si aplica); configura tu `BASE_URL` como dominio/URL de respuesta.
3. Sin token → **modo MOCK** (pantalla de pago simulada) para desarrollo y demo.
> Los campos de `Prepare`/`Confirm` (`internal/payment/payphone.go`) están escritos según la API pública de PayPhone; valídalos contra su documentación oficial con tu token antes de la demo real.

## Brain Studio
1. Agente nuevo → pega `brain/agent_prompt.md`.
2. Crea las 7 tools de `brain/tools.md` (URL = tu deploy, header `X-API-Key`).
3. Conecta el canal WhatsApp y prueba el flujo. Guion de demo: `brain/demo_script.md`.

## API
| Método | Ruta | |
|---|---|---|
| GET | `/api/movies` | cartelera |
| GET | `/api/showtimes?movie_id=` | funciones futuras |
| GET | `/api/showtimes/{id}/seats` | disponibles / ocupados |
| POST | `/api/reservations` | retiene asientos 10 min |
| GET | `/api/reservations/{id}` | estado (PAID → `ticket_code`) |
| POST | `/api/reservations/{id}/payment` | genera link de pago |
| POST | `/api/reservations/{id}/cancel` | libera asientos |
| GET | `/payphone/callback` | redirect de PayPhone → confirma |
| GET | `/health` | healthcheck |

## Mejoras futuras
Webhook firmado de PayPhone, reembolsos automáticos, QR del ticket, rate limiting, observabilidad (OpenTelemetry), múltiples cines.

## Para ejecutar con URL publica
ngrok http 8080