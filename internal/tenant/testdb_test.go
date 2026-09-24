package tenant

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/magoautomacoes/fonteca/internal/store"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func bancoDeTeste(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("fonteca_test"),
		tcpostgres.WithUsername("fonteca"),
		tcpostgres.WithPassword("teste"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatalf("subir postgres de teste: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("encerrar container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("obter dsn: %v", err)
	}
	pool, err := store.Conectar(ctx, dsn)
	if err != nil {
		t.Fatalf("conectar: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := store.MigrarMeta(ctx, pool); err != nil {
		t.Fatalf("migrar meta: %v", err)
	}
	return pool
}

// criarConta insere uma conta com uma chave e devolve a chave em claro.
func criarConta(t *testing.T, pool *pgxpool.Pool, nome string) (contaID, chave string) {
	t.Helper()
	ctx := context.Background()

	if err := pool.QueryRow(ctx,
		`INSERT INTO meta.conta (nome) VALUES ($1) RETURNING id`, nome).
		Scan(&contaID); err != nil {
		t.Fatalf("criar conta: %v", err)
	}

	chave, lookup, argon, err := GerarChave()
	if err != nil {
		t.Fatalf("gerar chave: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO meta.api_key (hash, hash_lookup, conta_id, nome)
		 VALUES ($1, $2, $3, 'teste')`, argon, lookup, contaID); err != nil {
		t.Fatalf("inserir chave: %v", err)
	}
	return contaID, chave
}
