package ingest

import (
	"context"
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TabelaEmpresa mapeia o arquivo de Empresas. Layout de 7 campos:
//
//	cnpj_basico ; razao_social ; natureza_juridica ;
//	qualificacao_responsavel ; capital_social ; porte ; ente_federativo
//
// Guardamos cinco: o que identifica e dimensiona a empresa no follow-up.
var TabelaEmpresa = Tabela{
	Nome:    "empresa",
	Colunas: []string{"cnpj_basico", "razao_social", "natureza_juridica", "capital_social", "porte"},
	Conv: func(campos []string) ([]any, error) {
		if len(campos) < 6 {
			return nil, ErrLinhaInvalida
		}
		if campos[0] == "" {
			return nil, ErrLinhaInvalida
		}

		capital, err := ParsearDecimal(campos[4])
		if err != nil {
			// Capital ilegivel nao descarta a empresa: o nome e o CNPJ
			// continuam valendo para a prospeccao.
			capital = pgtype.Numeric{Int: big.NewInt(0), Valid: true}
		}

		natureza := campos[2]
		if len(natureza) > 4 {
			natureza = natureza[:4]
		}
		porte := campos[5]
		if len(porte) > 2 {
			porte = porte[:2]
		}

		return []any{campos[0], campos[1], natureza, capital, porte}, nil
	},
}

// CarregarEmpresas le o arquivo de Empresas (sao dez, Empresas0 a Empresas9).
func CarregarEmpresas(ctx context.Context, pool *pgxpool.Pool, schema, caminhoZip string) (int64, error) {
	return Carregar(ctx, pool, schema, TabelaEmpresa, caminhoZip)
}
