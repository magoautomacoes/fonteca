package store

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// O brief original pedia //go:embed ../../deploy/papeis.sql, mas o Go proibe
// ".." em padroes de embed (erro "invalid pattern syntax": o padrao nao pode
// sair do diretorio do pacote — testado e confirmado no toolchain local, e
// documentado no testdata do proprio cmd/go). Por isso lemos o arquivo em
// tempo de execucao com os.ReadFile; a intencao do teste (aplicar o SQL real
// de deploy/papeis.sql contra um banco de teste) fica igual.
func lerPapeisSQL(t *testing.T) string {
	t.Helper()
	conteudo, err := os.ReadFile("../../deploy/papeis.sql")
	if err != nil {
		t.Fatalf("ler deploy/papeis.sql: %v", err)
	}
	return string(conteudo)
}

// O SQL de papeis precisa aplicar sem erro num banco recem-migrado, e o
// usuario da API NAO pode conseguir apagar dado nem criar schema.
func TestPapeisRestringemAAPI(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, lerPapeisSQL(t)); err != nil {
		t.Fatalf("aplicar papeis: %v", err)
	}

	var podeCriarSchema bool
	if err := pool.QueryRow(ctx,
		`SELECT has_database_privilege('fonteca_api', current_database(), 'CREATE')`).
		Scan(&podeCriarSchema); err != nil {
		t.Fatalf("consultar privilegio: %v", err)
	}
	if podeCriarSchema {
		t.Error("fonteca_api pode criar schema; devia estar proibido")
	}

	var podeApagarDeLote bool
	if err := pool.QueryRow(ctx,
		`SELECT has_table_privilege('fonteca_api', 'meta.lote', 'DELETE')`).
		Scan(&podeApagarDeLote); err != nil {
		t.Fatalf("consultar privilegio de tabela: %v", err)
	}
	if podeApagarDeLote {
		t.Error("fonteca_api pode apagar de meta.lote; devia ser so leitura")
	}

	var podeGravarUso bool
	if err := pool.QueryRow(ctx,
		`SELECT has_table_privilege('fonteca_api', 'meta.uso', 'INSERT')`).
		Scan(&podeGravarUso); err != nil {
		t.Fatalf("consultar privilegio de uso: %v", err)
	}
	if !podeGravarUso {
		t.Error("fonteca_api nao pode gravar em meta.uso; precisa para registrar consumo")
	}
}

// TestFontecaAPIAlcancaLoteFuturo mede acesso de verdade, nao GRANTs: cria um
// schema de lote de verdade DEPOIS de aplicar papeis.sql (o cenario da
// proxima ingestao real), conecta como fonteca_api com um DSN proprio e roda
// um SELECT contra uma tabela do lote. has_table_privilege nao bastaria aqui
// porque ele so confirma que o GRANT na tabela existe — nao pega a falta de
// USAGE no schema, que barra a conexao antes de chegar a tabela.
func TestFontecaAPIAlcancaLoteFuturo(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, lerPapeisSQL(t)); err != nil {
		t.Fatalf("aplicar papeis: %v", err)
	}

	// Senha conhecida para o teste poder conectar como fonteca_api.
	if _, err := pool.Exec(ctx, `ALTER ROLE fonteca_api PASSWORD 'senha-de-teste'`); err != nil {
		t.Fatalf("definir senha de teste: %v", err)
	}

	// Schema criado DEPOIS do papeis.sql: e exatamente o caso da proxima
	// ingestao, o que o CRITICAL da revisao identificou como quebrado.
	schema := "lote_2026_11"
	if err := CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema de lote: %v", err)
	}
	if err := SemearLoteDeTeste(ctx, pool, schema); err != nil {
		t.Fatalf("semear lote: %v", err)
	}

	var esperado int64
	if err := pool.QueryRow(ctx,
		fmt.Sprintf("SELECT count(*) FROM %s.estabelecimento", schema)).
		Scan(&esperado); err != nil {
		t.Fatalf("contar como dono, para saber o esperado: %v", err)
	}
	if esperado == 0 {
		t.Fatal("seed nao populou estabelecimento; teste nao prova nada")
	}

	cfg := pool.Config().ConnConfig
	dsnAPI := fmt.Sprintf("postgres://fonteca_api:senha-de-teste@%s:%d/%s?sslmode=disable",
		cfg.Host, cfg.Port, cfg.Database)

	poolAPI, err := Conectar(ctx, dsnAPI)
	if err != nil {
		t.Fatalf("conectar como fonteca_api: %v", err)
	}
	defer poolAPI.Close()

	var got int64
	err = poolAPI.QueryRow(ctx,
		fmt.Sprintf("SELECT count(*) FROM %s.estabelecimento", schema)).
		Scan(&got)
	if err != nil {
		t.Fatalf("fonteca_api nao conseguiu ler %s.estabelecimento: %v", schema, err)
	}
	if got != esperado {
		t.Errorf("fonteca_api contou %d linhas; quer %d (igual ao dono)", got, esperado)
	}
}

func TestPapeisSaoIdempotentes(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, lerPapeisSQL(t)); err != nil {
		t.Fatalf("primeira aplicacao: %v", err)
	}
	if _, err := pool.Exec(ctx, lerPapeisSQL(t)); err != nil {
		t.Fatalf("segunda aplicacao falhou — o script nao e idempotente: %v", err)
	}
}

// conectarComo abre um pool como outro usuario do mesmo banco de teste.
func conectarComo(t *testing.T, pool *pgxpool.Pool, usuario, senha string) *pgxpool.Pool {
	t.Helper()
	cfg := pool.Config().ConnConfig
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		usuario, senha, cfg.Host, cfg.Port, cfg.Database)
	outro, err := Conectar(context.Background(), dsn)
	if err != nil {
		t.Fatalf("conectar como %s: %v", usuario, err)
	}
	t.Cleanup(outro.Close)
	return outro
}

// A API so pode servir sob um usuario ao qual a RLS de meta.uso se aplica:
// superusuario, BYPASSRLS e dono (ou membro do dono) ignoram a politica.
func TestVerificarPapelSeguro(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, lerPapeisSQL(t)); err != nil {
		t.Fatalf("aplicar papeis: %v", err)
	}
	for _, cmd := range []string{
		`ALTER ROLE fonteca_api PASSWORD 'senha-de-teste'`,
		`CREATE ROLE papel_bypass LOGIN BYPASSRLS PASSWORD 'teste'`,
		`CREATE ROLE papel_dono LOGIN PASSWORD 'teste'`,
		`GRANT USAGE ON SCHEMA meta TO papel_dono`,
		`CREATE ROLE papel_membro LOGIN PASSWORD 'teste' IN ROLE papel_dono`,
	} {
		if _, err := pool.Exec(ctx, cmd); err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
	}

	if err := VerificarPapelSeguro(ctx, pool); err == nil {
		t.Error("superusuario passou na verificacao; a RLS nao se aplica a ele")
	}
	if err := VerificarPapelSeguro(ctx, conectarComo(t, pool, "papel_bypass", "teste")); err == nil {
		t.Error("papel com BYPASSRLS passou na verificacao")
	}
	if err := VerificarPapelSeguro(ctx, conectarComo(t, pool, "fonteca_api", "senha-de-teste")); err != nil {
		t.Errorf("fonteca_api recusado: %v", err)
	}

	// Dono de meta.uso (e quem herda do dono) tambem fica fora da RLS.
	if _, err := pool.Exec(ctx, `ALTER TABLE meta.uso OWNER TO papel_dono`); err != nil {
		t.Fatalf("trocar dono: %v", err)
	}
	if err := VerificarPapelSeguro(ctx, conectarComo(t, pool, "papel_dono", "teste")); err == nil {
		t.Error("dono de meta.uso passou na verificacao")
	}
	if err := VerificarPapelSeguro(ctx, conectarComo(t, pool, "papel_membro", "teste")); err == nil {
		t.Error("membro do dono de meta.uso passou na verificacao")
	}
}
