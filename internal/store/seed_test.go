package store

import (
	"context"
	"testing"
)

// O seed precisa cobrir exatamente os eixos da consulta-alvo:
// CNAE, periodo de abertura, situacao ATIVA, celular, UF, MEI.
func TestSeedCobreAConsultaAlvo(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	schema, err := NomeDoSchema(ReferenciaSeed)
	if err != nil {
		t.Fatalf("nome do schema: %v", err)
	}
	if err := CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if err := SemearLoteDeTeste(ctx, pool, schema); err != nil {
		t.Fatalf("semear: %v", err)
	}

	// A consulta-alvo inteira: CNAE 2512800, ativas, com celular,
	// em SP, abertas a partir de 2026-07-01, excluindo MEI.
	var encontrados int
	err = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM lote_2026_09.estabelecimento e
		JOIN lote_2026_09.empresa m USING (cnpj_basico)
		LEFT JOIN lote_2026_09.simples s USING (cnpj_basico)
		WHERE e.cnae_principal = '2512800'
		  AND e.situacao = 2
		  AND e.celular
		  AND e.uf = 'SP'
		  AND e.data_inicio >= DATE '2026-07-01'
		  AND COALESCE(s.opcao_mei, false) = false`).Scan(&encontrados)
	if err != nil {
		t.Fatalf("consulta-alvo: %v", err)
	}
	if encontrados != 2 {
		t.Errorf("consulta-alvo encontrou %d; quer 2 (ver comentarios do seed)", encontrados)
	}
}

func TestSeedTemContraExemplos(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := NomeDoSchema(ReferenciaSeed)
	if err := CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if err := SemearLoteDeTeste(ctx, pool, schema); err != nil {
		t.Fatalf("semear: %v", err)
	}

	casos := []struct {
		nome string
		sql  string
		quer int
	}{
		{"baixadas existem", `SELECT count(*) FROM lote_2026_09.estabelecimento WHERE situacao = 8`, 1},
		{"telefone fixo existe", `SELECT count(*) FROM lote_2026_09.estabelecimento WHERE NOT celular`, 3},
		{"MEI existe", `SELECT count(*) FROM lote_2026_09.simples WHERE opcao_mei`, 2},
		{"outra UF existe", `SELECT count(*) FROM lote_2026_09.estabelecimento WHERE uf <> 'SP'`, 3},
		{"acento preservado", `SELECT count(*) FROM lote_2026_09.empresa WHERE razao_social ILIKE '%ç%'`, 1},
		{"socios existem", `SELECT count(*) FROM lote_2026_09.socio`, 3},
		{"total de estabelecimentos", `SELECT count(*) FROM lote_2026_09.estabelecimento`, 12},
	}
	for _, c := range casos {
		var got int
		if err := pool.QueryRow(ctx, c.sql).Scan(&got); err != nil {
			t.Fatalf("%s: %v", c.nome, err)
		}
		if got != c.quer {
			t.Errorf("%s: got %d; quer %d", c.nome, got, c.quer)
		}
	}
}

func TestSeedEhIdempotente(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := NomeDoSchema(ReferenciaSeed)
	if err := CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if err := SemearLoteDeTeste(ctx, pool, schema); err != nil {
		t.Fatalf("primeira semeadura: %v", err)
	}
	if err := SemearLoteDeTeste(ctx, pool, schema); err != nil {
		t.Fatalf("segunda semeadura: %v", err)
	}

	var total int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM lote_2026_09.estabelecimento`).Scan(&total); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if total != 12 {
		t.Errorf("total = %d apos duas semeaduras; quer 12", total)
	}
}
