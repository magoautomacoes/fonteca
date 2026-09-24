package store

import (
	"context"
	"testing"
)

func TestNomeDoSchema(t *testing.T) {
	casos := []struct {
		referencia string
		quer       string
		querErro   bool
	}{
		{"2026-09", "lote_2026_09", false},
		{"2023-05", "lote_2023_05", false},
		{"2026-9", "", true}, // mes sem zero a esquerda
		{"26-09", "", true},  // ano curto
		{"", "", true},
		{"2026-09; DROP SCHEMA public", "", true}, // tentativa de injecao
	}
	for _, c := range casos {
		got, err := NomeDoSchema(c.referencia)
		if c.querErro {
			if err == nil {
				t.Errorf("NomeDoSchema(%q) devia falhar, devolveu %q", c.referencia, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NomeDoSchema(%q) erro inesperado: %v", c.referencia, err)
		}
		if got != c.quer {
			t.Errorf("NomeDoSchema(%q) = %q; quer %q", c.referencia, got, c.quer)
		}
	}
}

func TestValidarSchema(t *testing.T) {
	casos := []struct {
		schema   string
		querErro bool
	}{
		{"lote_2026_09", false},
		{"public", true},
		{"meta", true},
		{"lote_2026_09; DROP SCHEMA public", true},
	}
	for _, c := range casos {
		err := ValidarSchema(c.schema)
		if c.querErro && err == nil {
			t.Errorf("ValidarSchema(%q) devia falhar, aceitou", c.schema)
		}
		if !c.querErro && err != nil {
			t.Errorf("ValidarSchema(%q) erro inesperado: %v", c.schema, err)
		}
	}
}

func TestCriarSchemaDeLote(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	if err := CriarSchemaDeLote(ctx, pool, "lote_2026_09"); err != nil {
		t.Fatalf("criar schema: %v", err)
	}

	for _, tabela := range []string{"estabelecimento", "empresa", "socio", "simples", "municipio", "cnae", "natureza"} {
		var existe bool
		err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'lote_2026_09' AND table_name = $1
			)`, tabela).Scan(&existe)
		if err != nil {
			t.Fatalf("consultar %s: %v", tabela, err)
		}
		if !existe {
			t.Errorf("tabela %s nao foi criada", tabela)
		}
	}
}

func TestCriarIndices(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	if err := CriarSchemaDeLote(ctx, pool, "lote_2026_09"); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if err := CriarIndices(ctx, pool, "lote_2026_09"); err != nil {
		t.Fatalf("criar indices: %v", err)
	}

	// Total esperado = 11: os 5 indices de indices_lote.sql (4 em
	// estabelecimento, 1 em socio) mais as 6 chaves primarias que ja
	// existem em cada tabela do schema (empresa, estabelecimento,
	// municipio, simples, cnae, natureza). socio nao tem chave primaria.
	var total int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_indexes WHERE schemaname = $1`, "lote_2026_09").
		Scan(&total)
	if err != nil {
		t.Fatalf("contar indices: %v", err)
	}
	if total != 11 {
		t.Errorf("total de indices = %d; quer 11", total)
	}

	var comFiltroSituacao int
	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_indexes WHERE schemaname = $1 AND indexdef LIKE '%WHERE (situacao = 2)%'`,
		"lote_2026_09").Scan(&comFiltroSituacao)
	if err != nil {
		t.Fatalf("contar indices parciais: %v", err)
	}
	if comFiltroSituacao == 0 {
		t.Error("nenhum indice tem a clausula parcial WHERE (situacao = 2); a otimizacao critica de performance sumiu")
	}
}

func TestColunaCelularEhGerada(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	if err := CriarSchemaDeLote(ctx, pool, "lote_2026_09"); err != nil {
		t.Fatalf("criar schema: %v", err)
	}

	// Celular no Brasil: 9 digitos comecando em 9 (norma da Anatel desde 2016).
	// As duas condicoes importam — so a primeira deixaria passar fixo legado.
	// A Receita NAO guarda o nono digito: medido no lote 2026-09, todos os
	// 1.240.332 telefones preenchidos tem 8 digitos e nenhum tem 9. Entao aqui
	// "8 digitos comecando em 9" e um celular com o nono digito truncado.
	casos := []struct {
		cnpj     string
		telefone string
		quer     bool
		porque   string
	}{
		{"11111111000101", "98960051", true, "8 digitos comecando em 9: celular real da base"},
		{"11111111000102", "99187128", true, "idem"},
		{"11111111000103", "90001111", true, "segundo digito 0 nao desqualifica"},
		{"22222222000101", "30568282", false, "fixo comecando em 3"},
		{"22222222000102", "80242254", false, "fixo comecando em 8"},
		{"22222222000103", "9999888", false, "7 digitos: fixo antigo, nao celular"},
		{"22222222000104", "999998888", false, "9 digitos nao existem nesta base"},
		{"22222222000105", "9", false, "um digito so"},
		{"22222222000106", "", false, "vazio"},
	}

	for _, c := range casos {
		_, err := pool.Exec(ctx, `
			INSERT INTO lote_2026_09.estabelecimento (cnpj, cnpj_basico, telefone_1)
			VALUES ($1, $2, $3)`, c.cnpj, c.cnpj[:8], c.telefone)
		if err != nil {
			t.Fatalf("inserir %s: %v", c.telefone, err)
		}
	}

	for _, c := range casos {
		var got bool
		err := pool.QueryRow(ctx,
			`SELECT celular FROM lote_2026_09.estabelecimento WHERE cnpj = $1`,
			c.cnpj).Scan(&got)
		if err != nil {
			t.Fatalf("ler %s: %v", c.telefone, err)
		}
		if got != c.quer {
			t.Errorf("celular(%q) = %v; quer %v (%s)", c.telefone, got, c.quer, c.porque)
		}
	}
}

// NULL em telefone_1 deve produzir NULL em celular, nao false: sao coisas
// diferentes — "nao tem telefone" versus "tem telefone e nao e celular".
// O indice parcial WHERE celular trata NULL como nao-casa, que e o correto.
func TestColunaCelularComTelefoneNulo(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	if err := CriarSchemaDeLote(ctx, pool, "lote_2026_09"); err != nil {
		t.Fatalf("criar schema: %v", err)
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO lote_2026_09.estabelecimento (cnpj, cnpj_basico)
		VALUES ('33333333000101', '33333333')`)
	if err != nil {
		t.Fatalf("inserir: %v", err)
	}

	var celular *bool
	err = pool.QueryRow(ctx,
		`SELECT celular FROM lote_2026_09.estabelecimento WHERE cnpj = '33333333000101'`).
		Scan(&celular)
	if err != nil {
		t.Fatalf("ler: %v", err)
	}
	if celular != nil {
		t.Errorf("celular = %v; quer NULL quando nao ha telefone", *celular)
	}

	// E o filtro da consulta-alvo nao pode incluir essa linha.
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM lote_2026_09.estabelecimento WHERE celular`).Scan(&n); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 0 {
		t.Errorf("WHERE celular devolveu %d linhas; NULL nao pode casar", n)
	}
}

func TestTrocaAtomicaDeLote(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	if err := CriarSchemaDeLote(ctx, pool, "lote_2026_09"); err != nil {
		t.Fatalf("criar primeiro schema: %v", err)
	}
	if err := TrocarLoteVigente(ctx, pool, "lote_2026_09", "2026-09", 100); err != nil {
		t.Fatalf("primeira troca: %v", err)
	}

	vigente, err := LoteVigente(ctx, pool)
	if err != nil {
		t.Fatalf("ler vigente: %v", err)
	}
	if vigente == nil || vigente.Schema != "lote_2026_09" {
		t.Fatalf("vigente = %v; quer lote_2026_09", vigente)
	}

	// Chega o lote seguinte.
	if err := CriarSchemaDeLote(ctx, pool, "lote_2026_10"); err != nil {
		t.Fatalf("criar segundo schema: %v", err)
	}
	if err := TrocarLoteVigente(ctx, pool, "lote_2026_10", "2026-10", 200); err != nil {
		t.Fatalf("segunda troca: %v", err)
	}

	vigente, err = LoteVigente(ctx, pool)
	if err != nil {
		t.Fatalf("ler vigente apos troca: %v", err)
	}
	if vigente == nil || vigente.Schema != "lote_2026_10" {
		t.Fatalf("vigente = %v; quer lote_2026_10", vigente)
	}
	if vigente.Linhas != 200 {
		t.Errorf("linhas = %d; quer 200", vigente.Linhas)
	}

	// O lote antigo continua registrado, mas nao vigente.
	var vigenteAntigo bool
	err = pool.QueryRow(ctx,
		`SELECT vigente FROM meta.lote WHERE schema = 'lote_2026_09'`).Scan(&vigenteAntigo)
	if err != nil {
		t.Fatalf("ler lote antigo: %v", err)
	}
	if vigenteAntigo {
		t.Error("lote antigo continuou vigente apos a troca")
	}
}

func TestLoteVigenteSemNenhum(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	vigente, err := LoteVigente(ctx, pool)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if vigente != nil {
		t.Errorf("vigente = %v; quer nil quando nada foi importado", vigente)
	}
}

func TestDescartarSchema(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	if err := CriarSchemaDeLote(ctx, pool, "lote_2026_09"); err != nil {
		t.Fatalf("criar: %v", err)
	}
	if err := DescartarSchema(ctx, pool, "lote_2026_09"); err != nil {
		t.Fatalf("descartar: %v", err)
	}

	var existe bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.schemata
		               WHERE schema_name = 'lote_2026_09')`).Scan(&existe)
	if err != nil {
		t.Fatalf("consultar: %v", err)
	}
	if existe {
		t.Error("schema continuou existindo apos DescartarSchema")
	}
}

func TestDescartarSchemaRecusaNomeInvalido(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	// Nunca pode aceitar nome fora do padrao lote_YYYY_MM.
	if err := DescartarSchema(ctx, pool, "public"); err == nil {
		t.Fatal("DescartarSchema aceitou 'public'; isso apagaria o banco")
	}
	if err := DescartarSchema(ctx, pool, "meta"); err == nil {
		t.Fatal("DescartarSchema aceitou 'meta'; isso apagaria contas e chaves")
	}
}
