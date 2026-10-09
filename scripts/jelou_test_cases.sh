#!/usr/bin/env bash
# Casos de éxito y error de cada tool, ejecutados server-side con `jelou test tool` (borrador vivo).
# Uso: bash scripts/jelou_test_cases.sh   (requiere `make jelou-push`)
# Slugs: list_movies=listar-peliculas · list_showtimes=listar-funciones · get_seats=ver-asientos
J="jelou --profile alex-merino"
run() { local label="$1"; shift; echo; echo "▶ $label"; $J test tool "$@" --agent 2>&1 | head -c 600; echo; }

echo "===== ÉXITO ====="
run "list_movies"                       listar-peliculas
run "list_showtimes (todas)"            listar-funciones
run "list_showtimes (movie_id=m1)"      listar-funciones --arg movie_id=m1
run "get_seats"                         ver-asientos --arg showtime_id=m1-1009-19

echo; echo "===== ERROR (se espera salida FAILED) ====="
run "get_seats · función inexistente 404"      ver-asientos --arg showtime_id=nope-0000-00
run "get_reservation · inexistente 404"        get-reservation --arg reservation_id=R000000000000
run "create_payment · inexistente 404"         create-payment --arg reservation_id=R000000000000
run "cancel_reservation · inexistente 404"     cancel-reservation --arg reservation_id=R000000000000
run "create_reservation · >6 asientos 400"     create-reservation --arg showtime_id=m1-1009-19 --arg phone=+593991234567 --arg-json '{"seats":["A1","A2","A3","A4","A5","A6","A7"]}'

echo; echo "===== FLUJO COMPLETO (ids reales) ====="
echo "Reserva:  jelou test tool create-reservation --arg showtime_id=m1-1009-19 --arg phone=+593991234567 --arg name=Alex --arg-json '{\"seats\":[\"A1\",\"A2\"]}' --profile alex-merino --agent"
echo "Repetir la misma reserva => 409 (asiento tomado)"
echo "Pago:     jelou test tool create-payment --arg reservation_id=<ID> --profile alex-merino --agent"
echo "Estado:   jelou test tool get-reservation --arg reservation_id=<ID> --profile alex-merino --agent"
echo "Cancelar: jelou test tool cancel-reservation --arg reservation_id=<ID> --profile alex-merino --agent"
