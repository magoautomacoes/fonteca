#!/usr/bin/env bash
# Carrega o lote mais recente da Receita, se ainda nao estiver carregado.
# Feito para rodar todo dia no cron: quando nao ha lote novo, sai sem fazer
# nada. Repassa as opcoes para "fonteca ingest" (ex: -cnae 5611201,5611203).
#
#   crontab -e
#   0 4 * * * /caminho/da/fonteca/scripts/atualizar.sh -cnae 5611201 >> /var/log/fonteca.log 2>&1
set -euo pipefail
cd "$(dirname "$0")/.."
exec docker compose run --rm cli ingest "$@"
