package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/magoautomacoes/fonteca/internal/store"
)

// Tabela descreve como um arquivo da Receita vira linhas no banco.
type Tabela struct {
	Nome    string
	Colunas []string
	Conv    Conversor
}

// ErrForaDoFiltro faz a linha ser descartada em silencio, sem contar como
// malformada. E diferente de ErrLinhaInvalida: a linha esta bem formada, so
// nao interessa. Medido no lote 2026-09: filtrar estabelecimentos pelos CNAEs
// de um ramo (quatro codigos) descarta 98% da base — 1,23 milhao de linhas em vez de 63
// milhoes — e derruba a carga de horas para minutos.
var ErrForaDoFiltro = errors.New("linha fora do filtro")

// Carregar le o ZIP e grava na tabela usando COPY em streaming. Nenhuma linha
// alem da corrente existe em memoria — e o que torna viavel um arquivo de
// 2,14 GB comprimido (cerca de 22 GB descompactado).
func Carregar(ctx context.Context, pool *pgxpool.Pool, schema string, t Tabela, caminhoZip string) (int64, error) {
	if err := store.ValidarSchema(schema); err != nil {
		return 0, err
	}

	leitor, err := AbrirZip(caminhoZip)
	if err != nil {
		return 0, err
	}
	defer leitor.Close()

	fonte := NovaFonteDeLinhas(leitor, t.Conv)

	// O COPY vai para uma tabela TEMPORARIA, sem chave primaria, e so depois as
	// linhas passam para a tabela real com ON CONFLICT DO NOTHING.
	//
	// O motivo e concreto: a base da Receita tem CNPJ repetido. No lote 2026-09,
	// o Empresas2.zip traz o basico 08314885 duas vezes — uma linha real
	// ("FLAVIO PAVAO DE SOUZA") e uma linha-fantasma vazia, com natureza "0000"
	// e razao social em branco. Com COPY direto na tabela real, essa UMA linha
	// em 4,5 milhoes aborta a carga inteira com "duplicate key value violates
	// unique constraint". A primeira ocorrencia de cada chave vence.
	temp := "tmp_" + t.Nome

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("abrir transacao: %w", err)
	}
	defer tx.Rollback(ctx)

	// ON COMMIT DROP: a temporaria some junto com a transacao, de sucesso ou nao.
	criarTemp := fmt.Sprintf(
		"CREATE TEMP TABLE %s (LIKE %s.%s INCLUDING DEFAULTS) ON COMMIT DROP",
		temp, schema, t.Nome)
	if _, err := tx.Exec(ctx, criarTemp); err != nil {
		return 0, fmt.Errorf("criar temporaria de %s: %w", t.Nome, err)
	}

	lidas, err := tx.CopyFrom(ctx, pgx.Identifier{temp}, t.Colunas, fonte)
	if err != nil {
		return 0, fmt.Errorf("COPY para %s: %w", temp, err)
	}
	if err := fonte.Err(); err != nil {
		return 0, fmt.Errorf("ler %s: %w", caminhoZip, err)
	}

	colunas := strings.Join(t.Colunas, ", ")
	mover := fmt.Sprintf(
		"INSERT INTO %s.%s (%s) SELECT %s FROM %s ON CONFLICT DO NOTHING",
		schema, t.Nome, colunas, colunas, temp)
	tag, err := tx.Exec(ctx, mover)
	if err != nil {
		return 0, fmt.Errorf("mover para %s.%s: %w", schema, t.Nome, err)
	}
	n := tag.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("confirmar carga de %s: %w", t.Nome, err)
	}

	if dup := lidas - n; dup > 0 {
		// Duplicata e esperada em pequena quantidade (lixo da origem); um
		// numero alto significa arquivo repetido ou faixa sobreposta.
		slog.Warn("linhas duplicadas descartadas",
			"tabela", t.Nome, "duplicadas", dup, "carregadas", n)
	}
	if d := fonte.Descartadas(); d > 0 {
		// Quanto o filtro economizou. Numero alto e o esperado numa carga
		// filtrada — sao 98% da base para um recorte de quatro CNAEs.
		slog.Info("linhas descartadas pelo filtro",
			"tabela", t.Nome, "descartadas", d, "carregadas", n)
	}
	if ig := fonte.Ignoradas(); ig > 0 {
		// Numero alto aqui e sinal de que o layout do arquivo mudou.
		slog.Warn("linhas ignoradas por malformacao",
			"tabela", t.Nome, "ignoradas", ig, "carregadas", n)
	}
	return n, nil
}
