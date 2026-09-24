package store

import (
	"context"
	"errors"
	"testing"
)

func TestBuscarConsultaAlvo(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	schema, _ := NomeDoSchema(ReferenciaSeed)
	if err := CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if err := SemearLoteDeTeste(ctx, pool, schema); err != nil {
		t.Fatalf("semear: %v", err)
	}
	if err := TrocarLoteVigente(ctx, pool, schema, ReferenciaSeed, 12); err != nil {
		t.Fatalf("trocar: %v", err)
	}

	// A consulta que originou o projeto.
	rows, err := Buscar(ctx, pool, Filtro{
		CNAEs:    []string{"2512800"},
		UFs:      []string{"SP"},
		Situacao: SituacaoAtiva,
		// 365 dias cobre as duas linhas-alvo do seed (jul/ago 2026, ~47-69
		// dias antes de "hoje") e exclui a linha propositalmente antiga
		// (2025-01-20, ~606 dias antes de "hoje"). 3650 dias (10 anos) NAO
		// exclui essa linha antiga em nenhuma data de execucao razoavel: o
		// gap entre 2025-01-20 e o periodo-alvo e de so ~589-610 dias,
		// muito menor que uma janela de 10 anos.
		UltimosDias: 365,
		SoCelular:   true,
		ExcluirMEI:  true,
		Limite:      100,
	})
	if err != nil {
		t.Fatalf("Buscar: %v", err)
	}
	defer rows.Close()

	var n int
	for rows.Next() {
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterar: %v", err)
	}
	// O seed foi construido para que esta consulta devolva exatamente 2.
	if n != 2 {
		t.Errorf("encontrou %d; quer 2 (ver docs/docs/arquitetura.md)", n)
	}
}

func TestBuscarRecusaCNAEInvalido(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	schema, _ := NomeDoSchema(ReferenciaSeed)
	CriarSchemaDeLote(ctx, pool, schema)
	TrocarLoteVigente(ctx, pool, schema, ReferenciaSeed, 0)

	// CNAE precisa ser 7 digitos; qualquer outra coisa e recusada antes do banco.
	for _, ruim := range []string{"251", "abcdefg", "2512800; DROP TABLE", ""} {
		rows, err := Buscar(ctx, pool, Filtro{CNAEs: []string{ruim}, Limite: 10})
		if err == nil {
			rows.Close() // senao a pool esgota e o teste trava em vez de falhar
			t.Errorf("Buscar aceitou CNAE %q", ruim)
		}
		// Erro de filtro carrega o sentinela: e o que permite ao pacote api
		// distinguir "voce mandou um filtro ruim" (400) de "o banco falhou"
		// (500), ja que os dois voltam pelo mesmo tipo error.
		if !errors.Is(err, ErrFiltroInvalido) {
			t.Errorf("Buscar(%q): erro %v nao carrega ErrFiltroInvalido", ruim, err)
		}
	}
}

// Buscar tambem falha por motivo de infraestrutura (LoteVigente sem
// conseguir consultar o banco, por exemplo) — e esse erro NAO pode carregar
// ErrFiltroInvalido, senao o pacote api trataria uma falha do banco como se
// fosse culpa do cliente e devolveria o texto cru do Postgres num 400. Este
// teste reproduz exatamente o cenario que a revisao da Task 9 apontou:
// contexto cancelado entre a validacao do filtro (que passa) e a consulta a
// LoteVigente dentro de Buscar.
func TestBuscarComContextoCanceladoNaoCarregaErrFiltroInvalido(t *testing.T) {
	pool := bancoDeTeste(t)

	schema, _ := NomeDoSchema(ReferenciaSeed)
	CriarSchemaDeLote(context.Background(), pool, schema)
	TrocarLoteVigente(context.Background(), pool, schema, ReferenciaSeed, 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelado ANTES da chamada: forca LoteVigente a falhar dentro de Buscar

	rows, err := Buscar(ctx, pool, Filtro{CNAEs: []string{"2512800"}, Limite: 10})
	if err == nil {
		rows.Close()
		t.Fatal("Buscar deveria falhar com contexto cancelado")
	}
	if errors.Is(err, ErrFiltroInvalido) {
		t.Errorf("erro de infraestrutura (%v) carrega ErrFiltroInvalido; nao deveria", err)
	}
}

// O teto de linhas e do SERVIDOR, nao do cliente: sem ele um filtro amplo
// tentaria devolver 63 milhoes de linhas. E limite 0 ou negativo, que a CLI
// aceita sem reclamar, virariam "LIMIT 0" (zero leads, parecendo busca vazia)
// ou erro cru do Postgres.
func TestFiltroLimitaLinhas(t *testing.T) {
	casos := []struct {
		entrada int
		quer    int
	}{
		{999999, LimiteMaximo},
		{LimiteMaximo + 1, LimiteMaximo},
		{0, LimiteMaximo},
		{-1, LimiteMaximo},
		{10, 10},
		{LimiteMaximo, LimiteMaximo},
	}
	for _, c := range casos {
		f := Filtro{CNAEs: []string{"2512800"}, Limite: c.entrada}
		if err := f.validar(); err != nil {
			t.Fatalf("validar(%d): %v", c.entrada, err)
		}
		if f.Limite != c.quer {
			t.Errorf("Limite %d virou %d; quer %d", c.entrada, f.Limite, c.quer)
		}
	}
}

func TestBuscarSemLoteVigente(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	_, err := Buscar(ctx, pool, Filtro{CNAEs: []string{"2512800"}, Limite: 10})
	if err == nil {
		t.Fatal("devia falhar sem lote vigente")
	}
}
