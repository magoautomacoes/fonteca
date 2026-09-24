package tenant

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/magoautomacoes/fonteca/internal/store"
)

// RegistrarUso soma uma consulta e as linhas devolvidas ao total do dia.
func RegistrarUso(ctx context.Context, pool *pgxpool.Pool, contaID string, linhas int64) error {
	return comIdentidade(ctx, pool, contaID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO meta.uso (conta_id, dia, consultas, linhas_retornadas)
			VALUES ($1, CURRENT_DATE, 1, $2)
			ON CONFLICT (conta_id, dia) DO UPDATE
			SET consultas = meta.uso.consultas + 1,
			    linhas_retornadas = meta.uso.linhas_retornadas + EXCLUDED.linhas_retornadas`,
			contaID, linhas)
		return err
	})
}

// LerUso devolve o consumo da conta no mes corrente.
//
// Isolamento em duas camadas: a query filtra `conta_id = $1` E a RLS de
// meta.uso filtra pela identidade declarada em comIdentidade. O filtro na
// query existe porque a RLS nao se aplica ao dono da tabela (nem a
// superusuario ou BYPASSRLS) — sem ele, um pool conectado como dono devolveria
// o uso de TODAS as contas misturado, em silencio. A RLS continua valendo para
// fonteca_api, que e o usuario da API (ver store.VerificarPapelSeguro).
func LerUso(ctx context.Context, pool *pgxpool.Pool, contaID string) ([]store.Uso, error) {
	var linhas []store.Uso
	err := comIdentidade(ctx, pool, contaID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT conta_id, dia, consultas, linhas_retornadas
			FROM meta.uso
			WHERE conta_id = $1
			  AND dia >= date_trunc('month', CURRENT_DATE)
			ORDER BY dia`, contaID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var u store.Uso
			if err := rows.Scan(&u.ContaID, &u.Dia, &u.Consultas, &u.LinhasRetornadas); err != nil {
				return err
			}
			linhas = append(linhas, u)
		}
		return rows.Err()
	})
	return linhas, err
}

// comIdentidade abre transacao, declara a conta e roda a funcao.
//
// SET LOCAL, nao SET: o valor morre no COMMIT. Uma conexao devolvida ao pool
// nao carrega a identidade da requisicao anterior — que e exatamente o bug
// que fez o projeto recusar `search_path` no docs/arquitetura.md.
//
// A politica de RLS so tem efeito para quem NAO e dono da tabela. Sob o DSN
// de dono ela existe mas fica inerte; por isso a API roda como fonteca_api.
func comIdentidade(ctx context.Context, pool *pgxpool.Pool, contaID string, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abrir transacao: %w", err)
	}
	defer tx.Rollback(ctx) // no-op depois do commit

	// set_config com is_local=true e o equivalente parametrizavel de SET LOCAL:
	// SET LOCAL nao aceita bind parameter.
	if _, err := tx.Exec(ctx,
		`SELECT set_config('fonteca.conta_id', $1, true)`, contaID); err != nil {
		return fmt.Errorf("declarar conta: %w", err)
	}

	if err := fn(tx); err != nil {
		return fmt.Errorf("operacao de uso: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmar uso: %w", err)
	}
	return nil
}
