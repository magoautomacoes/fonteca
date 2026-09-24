package store

import (
	"context"
	"errors"
	"testing"
)

// Os quatro municipios do seed, com os codigos reais de SIAFI e IBGE.
func deParaDeTeste() []MunicipioIBGE {
	return []MunicipioIBGE{
		{CodigoSIAFI: 7107, CodigoIBGE: 3550308, Nome: "SAO PAULO", UF: "SP"},
		{CodigoSIAFI: 6001, CodigoIBGE: 3304557, Nome: "RIO DE JANEIRO", UF: "RJ"},
		{CodigoSIAFI: 4123, CodigoIBGE: 3106200, Nome: "BELO HORIZONTE", UF: "MG"},
		{CodigoSIAFI: 7535, CodigoIBGE: 4106902, Nome: "CURITIBA", UF: "PR"},
	}
}

func TestCarregarDeParaIBGE(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	if err := CarregarDeParaIBGE(ctx, pool, deParaDeTeste()); err != nil {
		t.Fatalf("carregar: %v", err)
	}

	var total int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM meta.municipio_ibge`).Scan(&total); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if total != 4 {
		t.Errorf("total = %d; quer 4", total)
	}
}

func TestIBGEPorSIAFI(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	if err := CarregarDeParaIBGE(ctx, pool, deParaDeTeste()); err != nil {
		t.Fatalf("carregar: %v", err)
	}

	// Sao Paulo: SIAFI 7107 e IBGE 3550308. Numeros diferentes -- e exatamente
	// por isso que o de-para existe.
	ibge, err := IBGEPorSIAFI(ctx, pool, 7107)
	if err != nil {
		t.Fatalf("traduzir 7107: %v", err)
	}
	if ibge != 3550308 {
		t.Errorf("IBGEPorSIAFI(7107) = %d; quer 3550308", ibge)
	}
}

func TestIBGEPorSIAFIDesconhecido(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	if err := CarregarDeParaIBGE(ctx, pool, deParaDeTeste()); err != nil {
		t.Fatalf("carregar: %v", err)
	}

	_, err := IBGEPorSIAFI(ctx, pool, 9999)
	if !errors.Is(err, ErrMunicipioDesconhecido) {
		t.Errorf("erro = %v; quer ErrMunicipioDesconhecido", err)
	}
}

func TestCarregarDeParaEhIdempotente(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	if err := CarregarDeParaIBGE(ctx, pool, deParaDeTeste()); err != nil {
		t.Fatalf("primeira carga: %v", err)
	}
	if err := CarregarDeParaIBGE(ctx, pool, deParaDeTeste()); err != nil {
		t.Fatalf("segunda carga: %v", err)
	}

	var total int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM meta.municipio_ibge`).Scan(&total); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if total != 4 {
		t.Errorf("total = %d apos duas cargas; quer 4", total)
	}
}

// O join geografico so funciona atraves do de-para. Este teste prova que
// o caminho completo -- estabelecimento (SIAFI) para IBGE -- fecha.
func TestJoinGeograficoAtravesDoDePara(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	if err := CarregarDeParaIBGE(ctx, pool, deParaDeTeste()); err != nil {
		t.Fatalf("carregar de-para: %v", err)
	}

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

	var codigoIBGE int32
	err = pool.QueryRow(ctx, `
		SELECT mi.codigo_ibge
		FROM lote_2026_09.estabelecimento e
		JOIN meta.municipio_ibge mi ON mi.codigo_siafi = e.municipio
		WHERE e.cnpj = '10000001000101'`).Scan(&codigoIBGE)
	if err != nil {
		t.Fatalf("join geografico: %v", err)
	}
	if codigoIBGE != 3550308 {
		t.Errorf("codigo IBGE = %d; quer 3550308 (Sao Paulo)", codigoIBGE)
	}
}
