#!/usr/bin/env bash
# Conversaciones de punta a punta contra el agente (borrador). Imprime solo lo que responde el bot.
# Uso: bash scripts/jelou_e2e.sh [feliz|error|todo]
TARGET="cinebot-reservas-whatsapp"
J="jelou --profile alex-merino"
EXEC=""

say() {  # say "mensaje"
  local out
  if [ -z "$EXEC" ]; then
    out=$($J test send text --target "$TARGET" --message "$1" --json 2>/dev/null)
  else
    out=$($J test send text --target "$TARGET" --execution "$EXEC" --message "$1" --json 2>/dev/null)
  fi
  EXEC=$(printf '%s' "$out" | python3 -c 'import sys,json; print(json.load(sys.stdin)["data"]["execution_id"])')
  echo; echo "👤 $1"
  printf '%s' "$out" | python3 -c '
import sys,json
d=json.load(sys.stdin)["data"]
for m in d.get("new_assistant_messages",[]): print("🤖",m)'
}

feliz() {
  echo "════ CASO FELIZ: película → función → asientos → reserva → link de pago ════"
  EXEC=""
  say "hola"
  say "quiero ver Dune"
  say "la de las 19:00 de hoy"
  say "2 personas"
  # asientos al azar en las filas C–F para no chocar con corridas anteriores (los pagados quedan ocupados)
  rows=(C D E F); R=${rows[RANDOM % 4]}; N=$((RANDOM % 7 + 1))
  say "${SEATS:-${R}${N} y ${R}$((N + 1))}, a nombre de Alex"
  say "mi número de WhatsApp es ${PHONE:-+593991234567}"
  say "sí, confirmo la reserva"
  say "quiero pagar, envíame el link"
  echo; read -r -p "👉 Abre el link de pago, pulsa «Pagar» y luego presiona Enter aquí... " _
  say "ya pagué"
}

error() {
  echo "════ CASO ERROR: asiento ya tomado (409) → alternativas ════"
  EXEC=""
  say "hola, quiero Dune hoy a las 19:00 para 1 persona"
  say "el asiento A1, a nombre de Pedro, mi número es ${PHONE:-+593991234567}"
  say "sí, confirmo"
  echo "(si A1 ya estaba reservado por el caso feliz, el agente debe ofrecer otros asientos)"
  EXEC=""
  say "quiero la función zzz-9999 para 2 personas"
}

case "${1:-todo}" in
  feliz) feliz ;;
  error) error ;;
  *) feliz; error ;;
esac
