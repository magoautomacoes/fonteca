package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestBuscarPaginaComCursorSemPularNemRepetir cria seu proprio lote (nao usa
// SemearLoteDeTeste) para controlar de proposito duas situacoes que o seed
// padrao nao cobre: linhas com data_inicio EMPATADA (mesmo dia) e linhas com
// data_inicio NULA. As duas quebram uma paginacao por cursor ingenua:
//   - empate sem CNPJ no ORDER BY: linhas do mesmo dia podem ser puladas ou
//     repetidas entre paginas, dependendo da ordem fisica que o Postgres
//     devolve.
//   - NULL com a comparacao de tupla simples do cursor: uma vez que o cursor
//     aponta para uma linha com data, "(data_inicio, cnpj) < (cursor)" nunca
//     e verdadeiro para data_inicio NULL (NULL nao compara), entao as linhas
//     sem data ficam inalcançaveis; e se o cursor partir de uma linha NULL,
//     a mesma comparacao e sempre falsa e o cliente reinicia do comeco.
//
// O filtro usa UltimosDias 0 (desligado) para que as linhas de data nula
// sejam elegiveis — ver o tratamento de UltimosDias=0 em Buscar.
func TestBuscarPaginaComCursorSemPularNemRepetir(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	schema, err := NomeDoSchema("2026-10")
	if err != nil {
		t.Fatalf("nome do schema: %v", err)
	}
	if err := CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if err := TrocarLoteVigente(ctx, pool, schema, "2026-10", 0); err != nil {
		t.Fatalf("trocar vigente: %v", err)
	}

	// cnpjBasico -> (cnpj, data_inicio ou nil para NULL). Duas linhas
	// empatadas em 2026-07-10, duas com data_inicio NULL, e uma terceira
	// data distinta para garantir que a ordenacao por data ainda importa.
	type linha struct {
		cnpjBasico string
		cnpj       string
		data       *string // "2026-07-10" ou nil
	}
	data1 := "2026-07-10"
	data2 := "2026-08-01"
	linhas := []linha{
		{"20000001", "20000001000101", &data1}, // empate A
		{"20000002", "20000002000102", &data1}, // empate B (mesmo dia)
		{"20000003", "20000003000103", &data2}, // data distinta, mais recente
		{"20000004", "20000004000104", nil},    // NULL A
		{"20000005", "20000005000105", nil},    // NULL B
	}

	for _, l := range linhas {
		if _, err := pool.Exec(ctx, fmt.Sprintf(
			`INSERT INTO %s.empresa (cnpj_basico, razao_social, natureza_juridica, capital_social, porte)
			 VALUES ($1, $2, '2062', 10000.00, '01')`, schema),
			l.cnpjBasico, "EMPRESA "+l.cnpjBasico); err != nil {
			t.Fatalf("inserir empresa %s: %v", l.cnpjBasico, err)
		}
		var dataInicio any
		if l.data != nil {
			dataInicio = *l.data
		}
		if _, err := pool.Exec(ctx, fmt.Sprintf(
			`INSERT INTO %s.estabelecimento
			   (cnpj, cnpj_basico, matriz_filial, nome_fantasia, situacao,
			    data_situacao, data_inicio, cnae_principal, cnae_secundarios,
			    uf, municipio, ddd_1, telefone_1, email)
			 VALUES ($1, $2, 1, $3, 2, '2026-07-01', $4, '2512800', '',
			         'SP', 7107, '11', '98888000', 'x@ex.com')`, schema),
			l.cnpj, l.cnpjBasico, "NOME "+l.cnpjBasico, dataInicio); err != nil {
			t.Fatalf("inserir estabelecimento %s: %v", l.cnpj, err)
		}
	}

	// Ordem esperada: data_inicio DESC NULLS LAST, cnpj DESC.
	// data2 (2026-08-01): 20000003
	// data1 (2026-07-10), empate, cnpj DESC: 20000002, depois 20000001
	// NULL, cnpj DESC: 20000005, depois 20000004
	esperado := []string{
		"20000003000103",
		"20000002000102",
		"20000001000101",
		"20000005000105",
		"20000004000104",
	}

	filtroBase := Filtro{
		CNAEs:       []string{"2512800"},
		UFs:         []string{"SP"},
		Situacao:    SituacaoAtiva,
		UltimosDias: 0, // desligado: elegibiliza as linhas com data_inicio NULL
		SoCelular:   false,
		ExcluirMEI:  false,
		Limite:      1,
	}

	var (
		visitados    []string
		aposData     *time.Time
		aposCNPJ     string
		maxIteracoes = len(esperado) + 3 // guarda contra loop infinito em regressao
	)
	for i := 0; i < maxIteracoes; i++ {
		f := filtroBase
		f.AposData = aposData
		f.AposCNPJ = aposCNPJ

		rows, err := Buscar(ctx, pool, f)
		if err != nil {
			t.Fatalf("Buscar (iteracao %d): %v", i, err)
		}

		var (
			cnpj          string
			razaoSocial   string
			nomeFantasia  *string
			ddd, telefone *string
			email         *string
			cnaePrincipal string
			cnaeDescricao *string
			dataInicio    *time.Time
			uf            string
			municipio     *string
			capitalSocial float64
			porte         *string
			natureza      *string
			n             int
		)
		for rows.Next() {
			n++
			if err := rows.Scan(&cnpj, &razaoSocial, &nomeFantasia,
				&ddd, &telefone, &email,
				&cnaePrincipal, &cnaeDescricao, &dataInicio, &uf, &municipio,
				&capitalSocial, &porte, &natureza); err != nil {
				rows.Close()
				t.Fatalf("scan (iteracao %d): %v", i, err)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterar (iteracao %d): %v", i, err)
		}
		rows.Close()

		if n == 0 {
			break // fim da paginacao
		}
		if n != 1 {
			t.Fatalf("iteracao %d trouxe %d linhas, esperava 1 (Limite=1)", i, n)
		}

		visitados = append(visitados, cnpj)
		aposData = dataInicio
		aposCNPJ = cnpj
	}

	if len(visitados) == maxIteracoes {
		t.Fatalf("paginacao nao terminou em %d iteracoes; possivel loop infinito", maxIteracoes)
	}

	if len(visitados) != len(esperado) {
		t.Fatalf("visitou %d CNPJs (%v), esperava %d (%v)", len(visitados), visitados, len(esperado), esperado)
	}
	for i, cnpj := range esperado {
		if visitados[i] != cnpj {
			t.Errorf("posicao %d: visitou %s, esperava %s (ordem completa visitada: %v)", i, visitados[i], cnpj, visitados)
		}
	}

	// Confere tambem que nenhum CNPJ foi visitado mais de uma vez.
	vistos := make(map[string]int)
	for _, c := range visitados {
		vistos[c]++
	}
	for c, n := range vistos {
		if n != 1 {
			t.Errorf("CNPJ %s visitado %d vezes, esperava 1", c, n)
		}
	}
}
