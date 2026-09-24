package export

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// bancoDeTeste sobe um Postgres 17 real para exercitar EscreverCSV contra
// pgx.Rows de verdade. Mock de pgx.Rows nao prova que a serializacao de
// tipos e NULL se comporta como no banco real.
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

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("abrir pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("banco nao respondeu: %v", err)
	}
	return pool
}

func TestEscreverCSV(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	rows, err := pool.Query(ctx, `
		SELECT * FROM (VALUES
			(1, 'ALFA'::text, 'alfa@ex.com'::text),
			(2, 'BETA'::text, NULL::text)
		) AS t(id, nome, email)
		ORDER BY id`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var saida strings.Builder
	n, err := EscreverCSV(&saida, rows)
	if err != nil {
		t.Fatalf("EscreverCSV: %v", err)
	}
	if n != 2 {
		t.Errorf("n = %d; quer 2", n)
	}

	linhas := strings.Split(strings.TrimRight(saida.String(), "\n"), "\n")
	if len(linhas) != 3 {
		t.Fatalf("esperava cabecalho + 2 linhas, veio %d: %q", len(linhas), linhas)
	}
	if linhas[0] != "id,nome,email" {
		t.Errorf("cabecalho = %q; quer %q", linhas[0], "id,nome,email")
	}
	if linhas[1] != "1,ALFA,alfa@ex.com" {
		t.Errorf("linha 1 = %q", linhas[1])
	}
	// A coluna email da segunda linha e NULL no banco; precisa virar string
	// vazia, nunca a palavra literal "<nil>".
	if linhas[2] != "2,BETA," {
		t.Errorf("linha 2 = %q; NULL devia virar campo vazio, nao <nil>", linhas[2])
	}
	if strings.Contains(saida.String(), "<nil>") {
		t.Errorf("saida contem literal <nil>: %q", saida.String())
	}
}

// O CSV e o entregavel do projeto: um vendedor abre esse arquivo. Um numeric
// cru ("{30000000 -2 false finite true}") ou um timestamp com fuso tornam a
// coluna inutil.
func TestEscreverCSVFormataNumericoEData(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	// Uma consulta que devolva um numeric e uma data conhecidos.
	rows, err := pool.Query(ctx, `
		SELECT 300000.50::numeric(18,2) AS capital,
		       DATE '2026-08-01'        AS abertura,
		       NULL::text               AS vazio`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var buf strings.Builder
	if _, err := EscreverCSV(&buf, rows); err != nil {
		t.Fatalf("EscreverCSV: %v", err)
	}

	saida := buf.String()
	if !strings.Contains(saida, "300000.50") {
		t.Errorf("capital nao saiu como decimal:\n%s", saida)
	}
	if strings.Contains(saida, "finite") || strings.Contains(saida, "{") {
		t.Errorf("numeric saiu como struct Go:\n%s", saida)
	}
	if !strings.Contains(saida, "2026-08-01") {
		t.Errorf("data nao saiu como AAAA-MM-DD:\n%s", saida)
	}
	if strings.Contains(saida, "00:00:00") || strings.Contains(saida, "UTC") {
		t.Errorf("data saiu com hora e fuso:\n%s", saida)
	}
	// NULL vira vazio, nao "<nil>".
	if strings.Contains(saida, "<nil>") {
		t.Errorf("NULL saiu como <nil>:\n%s", saida)
	}
}

func TestEscreverCSVSemLinhas(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	rows, err := pool.Query(ctx, `
		SELECT * FROM (VALUES (1, 'x'::text)) AS t(id, nome) WHERE false`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var saida strings.Builder
	n, err := EscreverCSV(&saida, rows)
	if err != nil {
		t.Fatalf("EscreverCSV: %v", err)
	}
	if n != 0 {
		t.Errorf("n = %d; quer 0", n)
	}
	if strings.TrimRight(saida.String(), "\n") != "id,nome" {
		t.Errorf("saida = %q; quer so o cabecalho", saida.String())
	}
}
