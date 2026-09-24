# Desenvolvimento

## Requisitos

- Go 1.26+
- Node 24+
- Docker (os testes sobem Postgres real com testcontainers; não há mock de banco)

## Estrutura

```
cmd/fonteca/        binário único: migrate, ingest, ibge, serve, conta, export
internal/source/    descoberta e download dos arquivos da Receita (WebDAV)
internal/ingest/    leitura dos ZIPs e carga com COPY
internal/store/     schema, lote vigente, busca e contagem (único que conhece o SQL)
internal/tenant/    chaves de API, limites, registro de uso sob RLS
internal/api/       rotas HTTP
internal/export/    CSV em streaming
web/                tela (React + Vite + TypeScript)
deploy/             papéis do Postgres, Caddyfile, compose de desenvolvimento
scripts/            instalação e atualização
docs/               documentação
```

Leia [arquitetura.md](arquitetura.md) antes de mexer em `internal/store`.

## Rodar do código-fonte

```bash
cp .env.example .env                 # preencha POSTGRES_PASSWORD
docker compose -f deploy/docker-compose.dev.yml --env-file .env up -d   # Postgres em localhost:5433
go build -o fonteca ./cmd/fonteca

export FONTECA_DSN='postgres://fonteca:<senha>@localhost:5433/fonteca?sslmode=disable'
./fonteca migrate
psql "$FONTECA_DSN" -f deploy/papeis.sql
psql "$FONTECA_DSN" -c "ALTER ROLE fonteca_api PASSWORD 'dev'"
./fonteca ibge
./fonteca conta -nome "Dev"          # mostra a chave

export FONTECA_DSN_API='postgres://fonteca_api:dev@localhost:5433/fonteca?sslmode=disable'
./fonteca serve                      # API em :8080

cd web && npm install && npm run dev # tela em :5173, com /v1 repassado para :8080
```

O `serve` se recusa a subir conectado como dono das tabelas: sob o dono a RLS
que isola o uso entre contas não vale. Use o papel `fonteca_api`.

## Testes

```bash
gofmt -l ./internal ./cmd            # tem de sair vazio
go vet ./...
go test ./... -timeout 30m           # sobe containers Postgres efêmeros

cd web
npm test                             # vitest
npx tsc -b && npx oxlint src
npm run build
```

Os testes de banco se ancoram no seed de `internal/store/seed.go`: 12
estabelecimentos, com um contraexemplo para cada filtro, e a consulta de
referência devolve exatamente 2.

## Dados da tela

O catálogo de CNAE e o mapa das UFs vêm do IBGE e ficam versionados em
`web/src/dados/`. Para atualizar quando o IBGE publicar revisão:

```bash
python web/scripts/gerar-catalogo-cnae.py   # CNAE 2.3, subclasses e termos
python web/scripts/gerar-mapa-uf.py         # malha oficial das UFs
```

## Convenções

- Código, comentários e mensagens em português, sem acento no código Go.
  Textos da tela com acentuação correta.
- Comentário explica o porquê, não o quê.
- Toda mudança de comportamento vem com teste que falharia sem ela.
- Nada de SQL montado com dado do usuário: o nome do schema do lote é a única
  interpolação, e ele vem validado de `meta.lote`.
