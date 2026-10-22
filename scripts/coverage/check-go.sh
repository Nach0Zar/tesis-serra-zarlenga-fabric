#!/usr/bin/env bash
# Verifica la cobertura total de un perfil de `go test -coverprofile` contra un
# minimo. Uso: check-go.sh <perfil> <minimo>
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "uso: $0 <perfil> <minimo>" >&2
  exit 2
fi
profile=$1
minimum=$2

total=$(go tool cover -func="$profile" | awk '/^total:/ { sub("%", "", $NF); print $NF }')
echo "Cobertura total: ${total}% (minimo ${minimum}%)"
if ! awk -v total="$total" -v minimum="$minimum" 'BEGIN { exit !(total + 0 >= minimum + 0) }'; then
  echo "error: la cobertura ${total}% es menor que el minimo ${minimum}%" >&2
  exit 1
fi
