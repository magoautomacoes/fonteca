package store

import (
	"context"
	"testing"
)

func TestMigrarMetaCriaTabelas(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t) // ja roda MigrarMeta

	for _, tabela := range []string{"lote", "conta", "api_key", "uso"} {
		var existe bool
		err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'meta' AND table_name = $1
			)`, tabela).Scan(&existe)
		if err != nil {
			t.Fatalf("consultar tabela %s: %v", tabela, err)
		}
		if !existe {
			t.Errorf("tabela meta.%s nao foi criada", tabela)
		}
	}
}

func TestMigrarMetaEhIdempotente(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	// Rodar de novo nao pode falhar nem duplicar nada.
	if err := MigrarMeta(ctx, pool); err != nil {
		t.Fatalf("segunda migracao falhou: %v", err)
	}
}

func TestApenasUmLoteVigente(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	_, err := pool.Exec(ctx,
		`INSERT INTO meta.lote (schema, referencia, vigente)
		 VALUES ('lote_2026_09', '2026-09', true)`)
	if err != nil {
		t.Fatalf("inserir primeiro lote: %v", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO meta.lote (schema, referencia, vigente)
		 VALUES ('lote_2026_10', '2026-10', true)`)
	if err == nil {
		t.Fatal("dois lotes vigentes foram aceitos; o indice unico parcial nao funciona")
	}
}

func TestSchemaDeLoteRejeitaNomeInvalido(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	_, err := pool.Exec(ctx,
		`INSERT INTO meta.lote (schema, referencia) VALUES ('public', '2026-09')`)
	if err == nil {
		t.Fatal("nome de schema invalido foi aceito; o CHECK nao funciona")
	}
}

// A tabela de DDDs e o que separa telefone plausivel de lixo: a base da Receita
// tem campos com dado errado, entao DDD invalido e esperado. Fica em meta para
// ser corrigivel sem reimportar o lote.
func TestTabelaDeDDD(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM meta.ddd`).Scan(&total); err != nil {
		t.Fatalf("contar ddd: %v", err)
	}
	// O Brasil tem 67 DDDs em uso.
	if total != 67 {
		t.Errorf("total de DDDs = %d; quer 67", total)
	}

	casos := []struct {
		ddd    string
		uf     string
		existe bool
	}{
		{"11", "SP", true},
		{"21", "RJ", true},
		{"85", "CE", true},
		{"68", "AC", true},
		{"20", "", false}, // nunca existiu
		{"23", "", false}, // nunca existiu
		{"00", "", false},
	}
	for _, c := range casos {
		var uf string
		err := pool.QueryRow(ctx, `SELECT uf FROM meta.ddd WHERE ddd = $1`, c.ddd).Scan(&uf)
		if !c.existe {
			if err == nil {
				t.Errorf("DDD %s nao devia existir, veio com uf %s", c.ddd, uf)
			}
			continue
		}
		if err != nil {
			t.Errorf("DDD %s: %v", c.ddd, err)
			continue
		}
		if uf != c.uf {
			t.Errorf("DDD %s -> uf %s; quer %s", c.ddd, uf, c.uf)
		}
	}
}

// MigrarMeta continua idempotente com a tabela nova: o ON CONFLICT dos DDDs
// nao pode duplicar nem falhar numa segunda execucao.
func TestTabelaDeDDDEhIdempotente(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	if err := MigrarMeta(ctx, pool); err != nil {
		t.Fatalf("segunda migracao: %v", err)
	}

	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM meta.ddd`).Scan(&total); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if total != 67 {
		t.Errorf("total = %d apos duas migracoes; quer 67", total)
	}
}

func TestMigrarMetaCriaColunaDeLookup(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()

	var existe bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'meta' AND table_name = 'api_key'
			  AND column_name = 'hash_lookup')`).Scan(&existe)
	if err != nil {
		t.Fatalf("consultar colunas: %v", err)
	}
	if !existe {
		t.Fatal("meta.api_key.hash_lookup nao existe")
	}
}

// Roda MigrarMeta duas vezes: a segunda nao pode falhar nem duplicar coluna.
func TestMigrarMetaEIdempotenteComALessao(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()

	if err := MigrarMeta(ctx, pool); err != nil {
		t.Fatalf("segunda migracao falhou: %v", err)
	}

	var n int
	err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'meta' AND table_name = 'api_key'
		  AND column_name = 'hash_lookup'`).Scan(&n)
	if err != nil {
		t.Fatalf("contar colunas: %v", err)
	}
	if n != 1 {
		t.Fatalf("esperava 1 coluna hash_lookup, achei %d", n)
	}
}

// Testa o cenario critico que a task existe para prevenir: uma tabela
// meta.api_key pré-existente (banco antigo, sem hash_lookup) recebendo a
// migração nova. CREATE TABLE IF NOT EXISTS ignoraria silenciosamente uma
// coluna nova ali dentro, quebrando autenticacao em runtime. Por isso usamos
// ALTER TABLE separado. Este teste prova que o ALTER efetivamente adiciona a
// coluna em banco antigo.
func TestMigrarMetaAdicionaColunaEmBancoAntigo(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()

	// Simula banco antigo dropando a coluna.
	_, err := pool.Exec(ctx, `ALTER TABLE meta.api_key DROP COLUMN hash_lookup`)
	if err != nil {
		t.Fatalf("dropar coluna (simular banco antigo): %v", err)
	}

	// Confirma que sumiu.
	var existe bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'meta' AND table_name = 'api_key'
			  AND column_name = 'hash_lookup')`).Scan(&existe)
	if err != nil {
		t.Fatalf("consultar coluna apos drop: %v", err)
	}
	if existe {
		t.Fatal("coluna nao foi dropada; teste nao esta testando nada")
	}

	// Roda migracao de novo.
	if err := MigrarMeta(ctx, pool); err != nil {
		t.Fatalf("migracao em banco antigo falhou: %v", err)
	}

	// Verifica que a coluna voltou.
	err = pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'meta' AND table_name = 'api_key'
			  AND column_name = 'hash_lookup')`).Scan(&existe)
	if err != nil {
		t.Fatalf("consultar coluna apos migracao: %v", err)
	}
	if !existe {
		t.Fatal("coluna hash_lookup nao foi adicionada ao banco antigo")
	}
}
