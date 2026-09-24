# Duas imagens a partir do mesmo repositorio:
#   --target fonteca  binario (API, ingestao, comandos de manutencao)
#   --target web      tela estatica servida pelo Caddy, que tambem faz o
#                     proxy de /v1 para a API e o HTTPS automatico

FROM node:24-alpine AS construir-web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS construir-go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /fonteca ./cmd/fonteca

FROM alpine:3.22 AS fonteca
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -u 10001 fonteca \
 # Diretorio dos ZIPs da Receita: persiste entre execucoes para a retomada
 # do download funcionar. O volume herda este dono na primeira montagem.
 && mkdir -p /var/lib/fonteca/lotes \
 && chown fonteca /var/lib/fonteca/lotes
COPY --from=construir-go /fonteca /usr/local/bin/fonteca
USER fonteca
ENTRYPOINT ["fonteca"]

FROM caddy:2-alpine AS web
COPY --from=construir-web /web/dist /srv
COPY deploy/Caddyfile /etc/caddy/Caddyfile
