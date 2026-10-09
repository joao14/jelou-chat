# CineBot en Jelou por código (CLI)

## 0. Requisitos
```bash
npm install -g @jelou/cli
jelou login                      # instala skills en tu editor de IA
jelou --version && jelou doctor --json
```

## 1. Preparar el repo (el CLI exige worktree limpio)
```bash
cd ~/Documents/Jelou/cinebot
git init 2>/dev/null; git add -A; git commit -m "base" 
```

## 2. Crear / enlazar el proyecto (Brain)
```bash
jelou project create "CineBot" --description "Reservas de cine con pago por WhatsApp"
jelou link --project <PROJECT_ID> --authoring json
jelou context --agent            # identidad, modelos, secrets, catálogo de nodos
jelou workflow node-spec --list  # tipos de nodo disponibles
jelou workflow node-spec API     # esquema exacto del nodo API
jelou workflow node-spec AI_AGENT
```

## 3. Secrets (no pongas la key en los JSON)
```bash
jelou secret --describe
jelou secret set CINEBOT_API_KEY cinebot-2026-clave-larga
```

## 4. Tools y workflow como archivos
```
tools/<slug>.json        # una por endpoint (ver brain/tools.md)
workflows/cinebot.whatsapp.json
```
Ciclo: editar → `jelou workflow validate` → `jelou push --dry-run --json` → `jelou push`.

## 5. Probar casos
```bash
jelou test tool list_movies
jelou test tool get_seats --input id=m1-1009-19
jelou test send text --target cinebot --message "hola"
jelou test dashboard
```

## 6. Matriz de casos (éxito / error)
| Tool | Éxito | Error |
|---|---|---|
| list_movies | 200 `movies[]` | 401 key inválida |
| list_showtimes | 200 `showtimes[]` (con/sin movie_id) | 401 |
| get_seats | 200 `available/taken` | 404 función inexistente · 401 |
| create_reservation | 201 `HELD` | 400 datos · 404 · 409 asiento tomado |
| create_payment | 200 `pay_url` | 404 · 409 no HELD · 500 pasarela |
| get_reservation | `PAID`+`ticket_code` / `HELD` | `EXPIRED` · 404 |
| cancel_reservation | 200 `CANCELLED` | 409 ya pagada · 404 |

Publicar: `jelou project publish <PROJECT_ID> --branch master --commit-name "v1" --yes`
