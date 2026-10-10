#!/usr/bin/env python3
"""Genera tools/*.json y workflows/cinebot-reservas.whatsapp.json (Jelou CLI) desde una sola fuente.

Uso:
  python3 scripts/gen_jelou.py            # usa BASE_URL y API_KEY de .env (literal en el JSON)
  python3 scripts/gen_jelou.py --secret   # usa {{$secret.CINEBOT_API_KEY}} en vez de la clave literal
  python3 scripts/gen_jelou.py --check    # solo informa qué tools faltan por crear

Conserva los ids (toolId, workflowId, ids de nodos) de los archivos ya bajados con `jelou tool pull`.
Las tools nuevas se crean primero con `jelou tool create "<name>"` (ver Makefile: make jelou-create).
"""
import json, os, re, sys, hashlib
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
TOOLS_DIR = ROOT / "tools"
WF_FILE = ROOT / "workflows" / "cinebot-reservas.whatsapp.json"
PROMPT = ROOT / "brain" / "agent_prompt.md"
USE_SECRET = "--secret" in sys.argv
CHECK = "--check" in sys.argv
def load_versions():
    f = ROOT / "jelou.tool-versions.json"   # lo escribe scripts/jelou_versions.py (make jelou-publish)
    return json.loads(f.read_text()) if f.exists() else {}


VERSIONS = load_versions()


def load_env():
    env = dict(os.environ)
    f = ROOT / ".env"
    if f.exists():
        for line in f.read_text().splitlines():
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                k, v = line.split("=", 1)
                env.setdefault(k.strip(), v.strip().strip('"'))
    return env


ENV = load_env()
BASE_URL = ENV.get("BASE_URL", "").rstrip("/")
API_KEY = "{{$secret.CINEBOT_API_KEY}}" if USE_SECRET else ENV.get("API_KEY", "")
if not BASE_URL or not API_KEY:
    sys.exit("Falta BASE_URL o API_KEY en .env")


def sid(*parts):
    """id estable (determinista) para nodos nuevos."""
    return hashlib.sha1("|".join(parts).encode()).hexdigest()[:21]


# ---------------------------------------------------------------- especificación de tools
def inp(name, desc, type_="STRING", required=True):
    return {"name": name, "description": desc, "type": type_, "required": required}


TOOLS = [
    dict(name="list_movies", legacy="listar-peliculas", method="GET", path="/api/movies",
         desc="Lista la cartelera de películas", var="movies", inputs=[],
         ok_label="Películas", out_ok="movies_listed", out_ok_desc="Cartelera con id, título, género, duración y clasificación.",
         out_err="movies_error", out_err_desc="No se pudo obtener la cartelera (401 key inválida, 5xx)."),
    dict(name="list_showtimes", legacy="listar-funciones", method="GET", path="/api/showtimes",
         desc="Lista las funciones futuras; filtra por movie_id si se envía", var="showtimes",
         inputs=[inp("movie_id", "ID de la película, ej. m1. Vacío = todas.", required=False)],
         query={"movie_id": "{{$input.movie_id}}"},
         ok_label="Funciones", out_ok="showtimes_listed", out_ok_desc="Funciones con id, sala, starts_at_local y price.",
         out_err="showtimes_error", out_err_desc="No se pudieron obtener las funciones."),
    dict(name="get_seats", legacy="ver-asientos", method="GET", path="/api/showtimes/{{$input.showtime_id}}/seats",
         desc="Asientos disponibles y ocupados de una función", var="seats",
         inputs=[inp("showtime_id", "ID de la función, ej. m1-1009-19")],
         ok_label="Asientos", out_ok="seats_listed", out_ok_desc="Asientos available y taken de la función.",
         out_err="seats_error", out_err_desc="Función inexistente (404) o error de API."),
    dict(name="create_reservation", method="POST", path="/api/reservations",
         desc="Retiene asientos por 10 minutos (máx. 6). 409 si algún asiento ya fue tomado", var="reservation",
         inputs=[inp("showtime_id", "ID de la función, ej. m1-1009-19"),
                 inp("seats", "Asientos a reservar, ej. [\"A1\",\"A2\"] (máx. 6)", "ARRAY"),
                 inp("phone", "Número de WhatsApp del cliente, ej. +593991234567"),
                 inp("name", "Nombre del cliente", required=False)],
         body='{"showtime_id":"{{$input.showtime_id}}","seats":{{$input.seats}},"phone":"{{$input.phone}}","name":"{{$input.name}}"}',
         ok_label="Reserva", out_ok="reservation_held", out_ok_desc="Reserva HELD con reservation_id, amount y seconds_left.",
         out_err="reservation_error", out_err_desc="400 datos inválidos, 404 función, 409 asiento tomado."),
    dict(name="create_payment", method="POST", path="/api/reservations/{{$input.reservation_id}}/payment",
         desc="Genera el link de pago (pay_url) de una reserva HELD", var="payment",
         inputs=[inp("reservation_id", "ID de la reserva, ej. R471631AB44D1")],
         ok_label="Pago", out_ok="payment_created", out_ok_desc="Reserva con pay_url para pagar.",
         out_err="payment_error", out_err_desc="404 reserva, 409 no está HELD, 500 pasarela."),
    dict(name="get_reservation", method="GET", path="/api/reservations/{{$input.reservation_id}}",
         desc="Estado de una reserva (HELD, PAID con ticket_code, EXPIRED, CANCELLED)", var="reservation",
         inputs=[inp("reservation_id", "ID de la reserva")],
         ok_label="Reserva", out_ok="reservation_found", out_ok_desc="Reserva con status, seconds_left y ticket_code si PAID.",
         out_err="reservation_not_found", out_err_desc="404 reserva inexistente."),
    dict(name="cancel_reservation", method="POST", path="/api/reservations/{{$input.reservation_id}}/cancel",
         desc="Cancela una reserva no pagada y libera los asientos", var="reservation",
         inputs=[inp("reservation_id", "ID de la reserva")],
         ok_label="Reserva", out_ok="reservation_cancelled", out_ok_desc="Reserva CANCELLED.",
         out_err="cancel_error", out_err_desc="404 reserva, 409 ya pagada."),
]



RES_FIELDS = ["reservation_id", "status", "movie", "hall", "showtime", "showtime_local", "seats", "amount",
              "amount_cents", "seconds_left", "expires_at", "pay_url", "ticket_code"]
FIELDS = {
    "list_movies": ["movies"],
    "list_showtimes": ["showtimes"],
    "get_seats": ["showtime_id", "available", "taken"],
    "create_reservation": RES_FIELDS, "create_payment": RES_FIELDS,
    "get_reservation": RES_FIELDS, "cancel_reservation": RES_FIELDS,
}
ERR_FIELDS = ["http_status", "error"]

def slug_of(name):
    return name.replace("_", "-")


def hdr(k, v, name):
    return {"id": sid(name, k), "key": k, "value": v, "enabled": True}


def build_tool(t, old):
    n = t["name"]
    old = old or {}
    ids = {x["id"] for x in old.get("nodes", [])}
    by_type = {}
    for x in old.get("nodes", []):
        by_type.setdefault(x["type"], []).append(x)
    start_id = (by_type.get("START") or [{"id": sid(n, "start")}])[0]["id"]
    http_id = (by_type.get("HTTP") or [{"id": sid(n, "http")}])[0]["id"]
    out_ok_id = next((o["id"] for o in old.get("outputs", []) if o["type"] == "SUCCESS"), sid(n, "out_ok"))
    out_err_id = next((o["id"] for o in old.get("outputs", []) if o["type"] == "FAILED"), sid(n, "out_err"))
    end_ok = sid(n, "end_ok")
    end_err = sid(n, "end_err")
    for e in by_type.get("END", []):
        oid = e["configuration"]["outputId"]
        if oid == out_ok_id: end_ok = e["id"]
        if oid == out_err_id: end_err = e["id"]

    inputs = []
    for i, x in enumerate(t["inputs"]):
        prev = next((p for p in old.get("inputs", []) if p["name"] == x["name"]), {})
        inputs.append({"name": x["name"], "id": prev.get("id", sid(n, "in", x["name"])), "displayName": x["name"],
                       "description": x["description"], "type": x["type"], "required": x["required"],
                       "defaultValue": None, "configuration": {}, "position": i})

    outputs = [
        {"name": t["out_ok"], "id": out_ok_id, "displayName": t["out_ok"].replace("_", " ").capitalize(),
         "description": t["out_ok_desc"], "type": "SUCCESS", "required": True,
         "schema": {f: f for f in FIELDS[n]}, "position": 0},
        {"name": t["out_err"], "id": out_err_id, "displayName": t["out_err"].replace("_", " ").capitalize(),
         "description": t["out_err_desc"], "type": "FAILED", "required": True,
         "schema": {f: f for f in ERR_FIELDS}, "position": 1},
    ]

    headers = [hdr("Content-type", "application/json", n), hdr("X-API-Key", API_KEY, n),
               hdr("ngrok-skip-browser-warning", "1", n), hdr("accept", "application/json", n)]
    params = [{"id": sid(n, "q", k), "key": k, "value": v, "enabled": True} for k, v in t.get("query", {}).items()]
    body = ({"type": "application/json", "content": t["body"], "enabled": True} if t.get("body")
            else {"type": "no-content", "content": None, "enabled": False})
    http = {
        "id": http_id, "title": "API", "type": "HTTP", "position": [880, 201],
        "configuration": {
            "url": BASE_URL + t["path"], "method": t["method"], "output": t["var"],
            "headers": headers, "parameters": params, "body": body,
            "authentication": {"type": None, "enabled": True},
            "settings": {"retries": 0, "timeout": 10000, "sslCertificate": True,
                         "retryOptions": {"enabled": False, "retries": 0, "retryDelay": "no_delay", "milliseconds": 1000,
                                          "retryCondition": "is_network_or_idempotent_request_error", "shouldResetTimeout": False},
                         "proxyOptions": {"ip": "185.228.195.219", "auth": {"password": None, "username": None}, "host": None,
                                          "port": None, "enabled": False, "protocol": "https"}},
        },
    }
    code_ok, code_err = sid(n, "code_ok"), sid(n, "code_err")
    v = t["var"]
    prelude = (f"let c = null, r = null;\n"
               f"try {{ c = $context.get('{v}'); }} catch (e) {{}}\n"
               f"try {{ r = $context.getHttpResponse('{v}'); }} catch (e) {{}}\n"
               f"let body = (c && c.data !== undefined && c.data !== null) ? c.data : (r ? r.json : undefined);\n"
               f"if ((body === undefined || body === null) && r && r.raw) {{ try {{ body = JSON.parse(r.raw); }} catch (e) {{}} }}\n"
               f"if (typeof body === 'string') {{ try {{ body = JSON.parse(body); }} catch (e) {{}} }}\n"
               f"if (body === undefined || body === null) body = {{}};\n"
               f"const status = (c && c.status) || (r && r.status) || 0;\n")
    js_ok = prelude + f"$output.set('{t['out_ok']}', body);"
    js_err = prelude + (f"$output.set('{t['out_err']}', {{ http_status: status, "
                        f"error: (body && body.error) || 'No se pudo completar la solicitud' }});")
    nodes = [
        {"id": start_id, "title": "Start", "type": "START", "position": [431, 232]},
        http,
        {"id": code_ok, "title": "Devolver resultado", "type": "CODE", "position": [1120, 135],
         "configuration": {"title": "Devolver resultado", "content": js_ok}},
        {"id": code_err, "title": "Devolver error", "type": "CODE", "position": [1120, 344],
         "configuration": {"title": "Devolver error", "content": js_err}},
        {"id": end_ok, "title": "Fin éxito", "type": "END", "position": [1349, 135], "configuration": {"outputId": out_ok_id}},
        {"id": end_err, "title": "Fin error", "type": "END", "position": [1346, 344], "configuration": {"outputId": out_err_id}},
    ]
    edges = [
        {"id": sid(n, "e1"), "sourceId": start_id, "targetId": http_id, "branch": "default"},
        {"id": sid(n, "e2"), "sourceId": http_id, "targetId": code_ok, "branch": "success"},
        {"id": sid(n, "e3"), "sourceId": http_id, "targetId": code_err, "branch": "failed"},
        {"id": sid(n, "e4"), "sourceId": code_ok, "targetId": end_ok, "branch": "success"},
        {"id": sid(n, "e5"), "sourceId": code_err, "targetId": end_err, "branch": "success"},
    ]
    return {"toolId": old["toolId"], "workflowId": old["workflowId"], "toolName": n, "toolDescription": t["desc"],
            "triggerType": old.get("triggerType", "DEFAULT"), "ownership": old.get("ownership", "OWNED"),
            "inputs": inputs, "outputs": outputs, "nodes": nodes, "edges": edges}


def find_existing(t):
    for cand in (slug_of(t["name"]), t.get("legacy")):
        if cand and (TOOLS_DIR / f"{cand}.json").exists():
            return cand, json.loads((TOOLS_DIR / f"{cand}.json").read_text())
    return None, None


def build_workflow(built):
    wf = json.loads(WF_FILE.read_text())
    agent = next(x for x in wf["nodes"] if x["type"] == "AI_TASK")
    start = next(x for x in wf["nodes"] if x["type"] == "START")
    cfg = agent["configuration"]
    cfg["instructions"] = PROMPT.read_text() + (
        "\n\n## Contexto del cliente\n"
        "Identificador del usuario en el canal (en WhatsApp es su número): {{$user.id}}\n"
        "Si parece un teléfono (dígitos, con o sin +), úsalo como `phone` sin pedirlo. "
        "Si está vacío o no parece un teléfono, pídelo UNA sola vez con código de país.\n")
    cfg["variable"] = "agentResponse"
    cfg["startCommand"] = "{{$message.text}}"
    cfg["connections"] = {"mcp": [], "datum": [], "tools": [
        {"id": sid("conn", b["toolName"]), "name": b["toolName"], "description": b["toolDescription"],
         "toolId": b["toolId"], "version": VERSIONS.get(b["toolName"], "1.0.0"), "action": "none", "external": True,
         "inputs": [dict(i) for i in b["inputs"]]}  # mismo registro que tools/*.json (id, displayName, position...)
        for b in built] + [
        {"name": "get_current_date_time", "external": False, "action": "none",
         "operationParameters": {"get_current_date_time": {"timezone": {"mode": "value", "value": "America/Guayaquil"}}}},
        {"name": "get_weekday_of_a_date", "external": False, "action": "none"}]}
    cfg["nonHappyPathHandlers"] = {"error": {"mode": "message", "message":
        "Tuve un problema técnico y no pude completar tu solicitud. Intenta de nuevo en unos minutos 🙏"}}
    cfg["endFunction"] = {"description":
        "Termina cuando el cliente ya recibió su ticket (status PAID), canceló su reserva, o dejó claro que no quiere continuar. "
        "No termines mientras haya una reserva HELD sin pagar. Termina justo tras confirmar, sin despedida extra."}
    cierre = next((x for x in wf["nodes"] if x["title"] == "Cierre"), {"id": sid("wf", "cierre"), "title": "Cierre", "type": "CHANNEL_MESSAGE", "position": [1200, 60]})
    cierre["configuration"] = {"messages": [{"type": "text", "text": "¡Listo! Gracias por usar CineBot 🎬 Cuando quieras otra función, escríbeme."}]}
    expira = next((x for x in wf["nodes"] if x["title"] == "Sesión expirada"), {"id": sid("wf", "expira"), "title": "Sesión expirada", "type": "CHANNEL_MESSAGE", "position": [1200, 260]})
    expira["configuration"] = {"messages": [{"type": "text", "text": "Cerré la conversación por inactividad. Si tenías una reserva sin pagar, se libera sola a los 10 minutos. Escríbeme y retomamos."}]}
    wf["workflowDefault"] = True   # el canal de WhatsApp debe arrancar siempre en CineBot
    wf["nodes"] = [start, agent, cierre, expira]
    wf["edges"] = [
        next((e for e in wf["edges"] if e["sourceId"] == start["id"]), {"id": sid("wf", "e0"), "sourceId": start["id"], "targetId": agent["id"], "branch": "default"}),
        {"id": sid("wf", "e1"), "sourceId": agent["id"], "targetId": cierre["id"], "branch": "success"},
        {"id": sid("wf", "e2"), "sourceId": agent["id"], "targetId": expira["id"], "branch": "expire"},
    ]
    return wf


def main():
    built, missing = [], []
    for t in TOOLS:
        slug, old = find_existing(t)
        if not old:
            missing.append(t["name"]); continue
        b = build_tool(t, old)
        built.append(b)
        if not CHECK:
            (TOOLS_DIR / f"{slug}.json").write_text(json.dumps(b, indent=2, ensure_ascii=False) + "\n")
            print(f"tools/{slug}.json  ({t['name']})")
    if missing:
        print("\nFaltan por crear en Jelou (ejecuta una vez):")
        for m in missing:
            print(f'  jelou tool create --name "{m}" --profile alex-merino')
        print("y vuelve a correr este script.")
        if not CHECK: sys.exit(1)
    if not CHECK:
        WF_FILE.write_text(json.dumps(build_workflow(built), indent=2, ensure_ascii=False) + "\n")
        print(f"{WF_FILE.relative_to(ROOT)}  (AI_TASK + {len(built)} tools + cierre/expiración)")


main()
