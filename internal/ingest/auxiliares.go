package ingest

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Tabelas auxiliares da Receita: codigo ; descricao. Traduzem os codigos que
// a consulta devolve (CNAE principal, natureza juridica) em texto que quem
// abre o CSV entende. Motivos, Qualificacoes e Paises ficam de fora: nada que
// a API ou o export devolvem usa esses codigos.

// TabelaCnae: codigo de 7 digitos e descricao.
var TabelaCnae = Tabela{
	Nome:    "cnae",
	Colunas: []string{"codigo", "descricao"},
	Conv:    conversorAuxiliar(7),
}

// TabelaNatureza: codigo de 4 digitos da natureza juridica e descricao.
var TabelaNatureza = Tabela{
	Nome:    "natureza",
	Colunas: []string{"codigo", "descricao"},
	Conv:    conversorAuxiliar(4),
}

// conversorAuxiliar aceita so codigo do tamanho exato e so de digitos: e o
// que casa com char(n) na tabela de lote e com o codigo gravado em
// estabelecimento/empresa. Codigo fora disso nunca seria encontrado no join.
func conversorAuxiliar(tamanho int) Conversor {
	return func(campos []string) ([]any, error) {
		if len(campos) < 2 {
			return nil, ErrLinhaInvalida
		}
		codigo := strings.TrimSpace(campos[0])
		if len(codigo) != tamanho || strings.Trim(codigo, "0123456789") != "" {
			return nil, ErrLinhaInvalida
		}
		return []any{codigo, strings.TrimSpace(campos[1])}, nil
	}
}

// CarregarCnaes le o Cnaes.zip.
func CarregarCnaes(ctx context.Context, pool *pgxpool.Pool, schema, caminhoZip string) (int64, error) {
	return Carregar(ctx, pool, schema, TabelaCnae, caminhoZip)
}

// CarregarNaturezas le o Naturezas.zip.
func CarregarNaturezas(ctx context.Context, pool *pgxpool.Pool, schema, caminhoZip string) (int64, error) {
	return Carregar(ctx, pool, schema, TabelaNatureza, caminhoZip)
}
