package ingest

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TabelaMunicipio: codigo SIAFI da Receita e nome. NAO e codigo IBGE —
// ver meta.municipio_ibge para o de-para.
var TabelaMunicipio = Tabela{
	Nome:    "municipio",
	Colunas: []string{"codigo", "nome"},
	Conv: func(campos []string) ([]any, error) {
		if len(campos) < 2 {
			return nil, ErrLinhaInvalida
		}
		codigo, err := strconv.Atoi(campos[0])
		if err != nil {
			return nil, ErrLinhaInvalida
		}
		return []any{int32(codigo), campos[1]}, nil
	},
}

// CarregarMunicipios mantem a assinatura que o comando ja usa.
func CarregarMunicipios(ctx context.Context, pool *pgxpool.Pool, schema, caminhoZip string) (int64, error) {
	return Carregar(ctx, pool, schema, TabelaMunicipio, caminhoZip)
}
