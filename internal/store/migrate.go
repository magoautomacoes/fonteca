package store

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema_meta.sql
var schemaMeta string

//go:embed schema_ibge.sql
var schemaIBGE string

// Conectar abre um pool de conexoes e verifica que o banco responde.
func Conectar(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("abrir pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("banco nao respondeu: %w", err)
	}
	return pool, nil
}

// MigrarMeta cria o schema permanente. E idempotente: pode rodar sempre.
func MigrarMeta(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, schemaMeta); err != nil {
		return fmt.Errorf("aplicar schema meta: %w", err)
	}
	if _, err := pool.Exec(ctx, schemaIBGE); err != nil {
		return fmt.Errorf("aplicar schema de municipios: %w", err)
	}
	return nil
}
