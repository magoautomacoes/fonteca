package ingest

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Indices dos campos que interessam no arquivo de Estabelecimentos (30 campos).
const (
	estCNPJBasico     = 0
	estCNPJOrdem      = 1
	estCNPJDV         = 2
	estMatrizFilial   = 3
	estNomeFantasia   = 4
	estSituacao       = 5
	estDataSituacao   = 6
	estDataInicio     = 10
	estCNAEPrincipal  = 11
	estCNAESecundario = 12
	estUF             = 19
	estMunicipio      = 20
	estDDD1           = 21
	estTelefone1      = 22
	estDDD2           = 23
	estTelefone2      = 24
	estEmail          = 27
)

// TabelaEstabelecimento e a maior: ~63 milhoes de linhas em dez arquivos.
// Traz quatro dos seis filtros da consulta-alvo — CNAE, situacao, telefone e UF.
// TabelaEstabelecimento e o caso sem filtro: carrega a base inteira.
var TabelaEstabelecimento = TabelaEstabelecimentoCom(nil)

// TabelaEstabelecimentoCom monta a tabela com um filtro de CNAE. Filtro nil
// carrega tudo.
func TabelaEstabelecimentoCom(filtro *FiltroCNAE) Tabela {
	return Tabela{
		Nome: "estabelecimento",
		Colunas: []string{
			"cnpj", "cnpj_basico", "matriz_filial", "nome_fantasia",
			"situacao", "data_situacao", "data_inicio",
			"cnae_principal", "cnae_secundarios",
			"uf", "municipio", "ddd_1", "telefone_1", "ddd_2", "telefone_2", "email",
		},
		Conv: func(campos []string) ([]any, error) {
			if len(campos) < 28 {
				return nil, ErrLinhaInvalida
			}
			basico := campos[estCNPJBasico]
			if len(basico) != 8 {
				return nil, ErrLinhaInvalida
			}

			// O filtro vem ANTES de qualquer conversao: nao adianta parsear datas
			// e montar CNPJ de uma linha que vai ser descartada. Sao 98% delas.
			if !filtro.Aceita(campos[estCNAEPrincipal], campos[estCNAESecundario]) {
				return nil, ErrForaDoFiltro
			}

			// O CNPJ completo e a concatenacao dos tres primeiros campos.
			ordem := campos[estCNPJOrdem]
			dv := campos[estCNPJDV]
			if len(ordem) != 4 || len(dv) != 2 {
				return nil, ErrLinhaInvalida
			}
			cnpj := basico + ordem + dv

			matriz, _ := strconv.Atoi(campos[estMatrizFilial])
			situacao, _ := strconv.Atoi(campos[estSituacao])

			dataSituacao, err := ParsearData(campos[estDataSituacao])
			if err != nil {
				dataSituacao = nil // data ilegivel nao descarta o registro
			}
			dataInicio, err := ParsearData(campos[estDataInicio])
			if err != nil {
				dataInicio = nil
			}

			// A base tem UF com lixo (achado 1.6 da analise): so aceita 2 letras.
			uf := campos[estUF]
			if len(uf) != 2 {
				uf = ""
			}
			municipio, _ := strconv.Atoi(campos[estMunicipio])

			return []any{
				cnpj, basico, int16(matriz), campos[estNomeFantasia],
				int16(situacao), dataSituacao, dataInicio,
				campos[estCNAEPrincipal], campos[estCNAESecundario],
				uf, int32(municipio),
				campos[estDDD1], campos[estTelefone1],
				campos[estDDD2], campos[estTelefone2],
				campos[estEmail],
			}, nil
		},
	}
}

// CarregarEstabelecimentos le um dos dez arquivos de Estabelecimentos.
func CarregarEstabelecimentos(ctx context.Context, pool *pgxpool.Pool, schema, caminhoZip string) (int64, error) {
	return CarregarEstabelecimentosCom(ctx, pool, schema, caminhoZip, nil)
}

// CarregarEstabelecimentosCom carrega aplicando um filtro de CNAE. Filtro nil
// carrega a base inteira.
func CarregarEstabelecimentosCom(ctx context.Context, pool *pgxpool.Pool, schema, caminhoZip string, filtro *FiltroCNAE) (int64, error) {
	return Carregar(ctx, pool, schema, TabelaEstabelecimentoCom(filtro), caminhoZip)
}
