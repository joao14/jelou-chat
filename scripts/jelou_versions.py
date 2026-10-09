#!/usr/bin/env python3
"""Lee la última versión publicada de cada tool y la guarda en tools/versions.json."""
import json, subprocess, sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SLUGS = {"list_movies": "listar-peliculas", "list_showtimes": "listar-funciones", "get_seats": "ver-asientos",
         "create_reservation": "create-reservation", "create_payment": "create-payment",
         "get_reservation": "get-reservation", "cancel_reservation": "cancel-reservation"}


def latest(data):
    """Busca recursivamente una lista de dicts con 'version'; la primera es la más reciente."""
    if isinstance(data, list):
        if data and isinstance(data[0], dict) and "version" in data[0]:
            return str(data[0]["version"])
        for x in data:
            r = latest(x)
            if r: return r
    elif isinstance(data, dict):
        for v in data.values():
            r = latest(v)
            if r: return r
    return None


out = {}
for name, slug in SLUGS.items():
    p = subprocess.run(["jelou", "--profile", "alex-merino", "tool", "versions", slug, "--json"],
                       capture_output=True, text=True, cwd=ROOT)
    try:
        v = latest(json.loads(p.stdout))
    except Exception:
        v = None
    print(f"{name:20s} -> {v}")
    if v: out[name] = v
(ROOT / "tools" / "versions.json").write_text(json.dumps(out, indent=2) + "\n")
print("guardado tools/versions.json")
