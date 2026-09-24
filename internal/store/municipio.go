package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrMunicipioDesconhecido indica codigo SIAFI ausente do de-para.
var ErrMunicipioDesconhecido = errors.New("municipio sem de-para SIAFI/IBGE")

// CarregarDeParaIBGE grava (ou atualiza) o de-para. E idempotente.
func CarregarDeParaIBGE(ctx context.Context, pool *pgxpool.Pool, linhas []MunicipioIBGE) error {
	if len(linhas) == 0 {
		return nil
	}

	lote := &pgx.Batch{}
	for _, m := range linhas {
		lote.Queue(`
			INSERT INTO meta.municipio_ibge (codigo_siafi, codigo_ibge, nome, uf)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (codigo_siafi) DO UPDATE
			SET codigo_ibge = EXCLUDED.codigo_ibge,
			    nome        = EXCLUDED.nome,
			    uf          = EXCLUDED.uf`,
			m.CodigoSIAFI, m.CodigoIBGE, m.Nome, m.UF)
	}

	resultado := pool.SendBatch(ctx, lote)
	defer resultado.Close()

	for i := range linhas {
		if _, err := resultado.Exec(); err != nil {
			return fmt.Errorf("gravar municipio %d: %w", linhas[i].CodigoSIAFI, err)
		}
	}
	return nil
}

// IBGEPorSIAFI traduz o codigo da Receita para o codigo do IBGE.
func IBGEPorSIAFI(ctx context.Context, pool *pgxpool.Pool, siafi int32) (int32, error) {
	var ibge int32
	err := pool.QueryRow(ctx,
		`SELECT codigo_ibge FROM meta.municipio_ibge WHERE codigo_siafi = $1`,
		siafi).Scan(&ibge)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("SIAFI %d: %w", siafi, ErrMunicipioDesconhecido)
	}
	if err != nil {
		return 0, fmt.Errorf("traduzir SIAFI %d: %w", siafi, err)
	}
	return ibge, nil
}
