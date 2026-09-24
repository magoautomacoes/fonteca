#!/usr/bin/env bash
# Instala a Fonteca com Docker: cria o .env com senhas aleatorias, sobe o
# banco, aplica o schema e os papeis, carrega o de-para de municipios, sobe a
# API e a tela e emite a primeira chave de API.
#
#   scripts/instalar.sh                                   maquina local
#   DOMINIO=fonteca.seudominio.com.br scripts/instalar.sh VM com HTTPS
#
# Pode rodar de novo sem medo: nao troca senhas ja criadas nem emite chave
# nova se ja existir uma conta.
set -euo pipefail
cd "$(dirname "$0")/.."

falhar() { echo "erro: $*" >&2; exit 1; }
passo() { printf '\n==> %s\n' "$*"; }

command -v docker >/dev/null 2>&1 || falhar "Docker nao encontrado. Instale em https://docs.docker.com/get-docker/"
docker compose version >/dev/null 2>&1 || falhar "o plugin 'docker compose' nao foi encontrado (Docker Compose v2)."

senha() { head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n'; }

# Acrescenta ao .env so as variaveis que ainda nao estao la.
garantir() {
  local nome="$1" valor="$2"
  if ! grep -q "^${nome}=" .env 2>/dev/null; then
    printf '%s=%s\n' "$nome" "$valor" >> .env
  fi
}

passo "Configuracao (.env)"
[ -f .env ] || { : > .env; chmod 600 .env; }
if [ -n "${DOMINIO:-}" ]; then
  porta_http=80; porta_https=443
else
  porta_http=8080; porta_https=8443
fi
garantir POSTGRES_PASSWORD "$(senha)"
garantir FONTECA_API_SENHA "$(senha)"
garantir FONTECA_INGEST_SENHA "$(senha)"
garantir FONTECA_LEITURA_SENHA "$(senha)"
garantir DOMINIO "${DOMINIO:-}"
garantir PORTA_HTTP "${PORTA_HTTP:-$porta_http}"
garantir PORTA_HTTPS "${PORTA_HTTPS:-$porta_https}"
garantir PG_SHARED_BUFFERS "${PG_SHARED_BUFFERS:-512MB}"
set -a; . ./.env; set +a
echo "ok ($(grep -c '=' .env) variaveis em .env)"

passo "Construindo as imagens (a primeira vez leva alguns minutos)"
docker compose build

passo "Subindo o banco"
docker compose up -d --wait db

psql() { docker compose exec -T db psql -v ON_ERROR_STOP=1 -q -U fonteca -d fonteca "$@"; }

passo "Aplicando o schema"
docker compose run --rm cli migrate

passo "Criando os papeis do banco (a API roda com permissao minima)"
psql < deploy/papeis.sql
psql -c "ALTER ROLE fonteca_api PASSWORD '${FONTECA_API_SENHA}'" \
     -c "ALTER ROLE fonteca_ingest PASSWORD '${FONTECA_INGEST_SENHA}'" \
     -c "ALTER ROLE fonteca_leitura PASSWORD '${FONTECA_LEITURA_SENHA}'"

passo "Carregando o de-para de municipios (Tesouro Transparente)"
docker compose run --rm cli ibge || echo "aviso: nao foi possivel baixar agora; rode depois: docker compose run --rm cli ibge"

passo "Subindo a API e a tela"
docker compose up -d --wait api web

passo "Conta e chave de API"
contas="$(psql -tAc 'SELECT count(*) FROM meta.conta')"
if [ "$contas" = "0" ]; then
  saida="$(docker compose run --rm cli conta -nome "${CONTA:-Principal}")"
  chave="$(printf '%s\n' "$saida" | sed -n 's/^chave: *//p')"
else
  chave=""
  echo "ja existe conta; nenhuma chave nova emitida (para outra: docker compose run --rm cli conta -nome \"Nome\")"
fi

if [ -n "${DOMINIO:-}" ]; then url="https://${DOMINIO}"; else url="http://localhost:${PORTA_HTTP}"; fi

cat <<FIM

Fonteca instalada.

  Tela:  ${url}
  API:   ${url}/v1/health
FIM
if [ -n "$chave" ]; then
  cat <<FIM

  Sua chave de API (guarde agora; so o hash fica no banco):

    ${chave}
FIM
fi
cat <<FIM

Proximo passo: carregar os dados da Receita Federal. Baixa ~7 GB do portal
de dados abertos; o download retoma se cair.

  So alguns ramos (minutos para carregar, pouco disco):
    docker compose run --rm cli ingest -cnae 5611201,5611203

  Base completa (horas para carregar, ~30 GB de disco):
    docker compose run --rm cli ingest

Enquanto isso, a tela ja funciona no modo Demonstracao.
FIM
