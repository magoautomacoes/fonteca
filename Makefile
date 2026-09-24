.PHONY: build test fmt fmt-check db-up db-down psql

build:
	go build -o fonteca ./cmd/fonteca

fmt:
	gofmt -w ./internal ./cmd

fmt-check:
	@test -z "$$(gofmt -l ./internal ./cmd)" || { echo "arquivos desformatados:"; gofmt -l ./internal ./cmd; exit 1; }

test: fmt-check
	go test ./... -v

db-up:
	docker compose -f deploy/docker-compose.dev.yml --env-file .env up -d

db-down:
	docker compose -f deploy/docker-compose.dev.yml --env-file .env down

psql:
	docker exec -it fonteca-pg-dev psql -U fonteca -d fonteca
