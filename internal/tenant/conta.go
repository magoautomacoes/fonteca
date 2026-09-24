package tenant

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CriarConta cria a conta e a primeira chave. A chave em claro e devolvida
// UMA vez — nao ha como recupera-la depois, porque so o hash e guardado.
func CriarConta(ctx context.Context, pool *pgxpool.Pool, nome string) (contaID, chave string, err error) {
	chave, lookup, argon, err := GerarChave()
	if err != nil {
		return "", "", err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", "", fmt.Errorf("abrir transacao: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := tx.QueryRow(ctx,
		`INSERT INTO meta.conta (nome) VALUES ($1) RETURNING id`, nome).
		Scan(&contaID); err != nil {
		return "", "", fmt.Errorf("inserir conta: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO meta.api_key (hash, hash_lookup, conta_id, nome)
		 VALUES ($1, $2, $3, $4)`, argon, lookup, contaID, "chave inicial"); err != nil {
		return "", "", fmt.Errorf("inserir chave: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", "", fmt.Errorf("confirmar: %w", err)
	}
	return contaID, chave, nil
}
