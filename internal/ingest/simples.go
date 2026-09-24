package ingest

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TabelaSimples mapeia o arquivo do Simples Nacional. Layout medido em
// 17/09/2026, 7 campos:
//
//	cnpj_basico ; opcao_simples ; data_opcao ; data_exclusao ;
//	opcao_mei ; data_opcao_mei ; data_exclusao_mei
//
// Guardamos so os tres primeiros que interessam ao follow-up: quem e do
// Simples e quem e MEI. As datas ficam de fora (o design e enxuto).
var TabelaSimples = Tabela{
	Nome:    "simples",
	Colunas: []string{"cnpj_basico", "opcao_simples", "opcao_mei"},
	Conv: func(campos []string) ([]any, error) {
		if len(campos) < 5 {
			return nil, ErrLinhaInvalida
		}
		if campos[0] == "" {
			return nil, ErrLinhaInvalida
		}
		return []any{campos[0], campos[1] == "S", campos[4] == "S"}, nil
	},
}

// CarregarSimples le o arquivo do Simples Nacional. Sao ~50 milhoes de linhas
// (medido), entao vai por streaming como todo o resto.
func CarregarSimples(ctx context.Context, pool *pgxpool.Pool, schema, caminhoZip string) (int64, error) {
	return Carregar(ctx, pool, schema, TabelaSimples, caminhoZip)
}
