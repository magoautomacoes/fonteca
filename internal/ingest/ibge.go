package ingest

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/magoautomacoes/fonteca/internal/store"
)

// URLTabMun e a tabela de municipios do SIAFI publicada pelo Tesouro
// Transparente: a fonte primaria do de-para SIAFI -> IBGE (schema_ibge.sql).
const URLTabMun = "https://www.tesourotransparente.gov.br/ckan/dataset/abb968cb-3710-4f85-89cf-875c91b9c7f6/resource/eebb3bc6-9eea-4496-8bcf-304f33155282/download/TABMUN.csv"

// minimoDeMunicipios e a trava contra gravar um arquivo que nao e a tabela:
// o Brasil tem 5.570 municipios, e o Tesouro publica ~5.590 linhas. Uma
// pagina de erro ou um arquivo cortado ficam muito abaixo disso.
const minimoDeMunicipios = 5000

// LerTabMun le o TABMUN.csv: sem cabecalho, separado por ";", nome com
// espacos a direita. Layout: SIAFI ; CNPJ da prefeitura ; nome ; UF ; IBGE.
// Linha que nao tem os cinco campos com codigos numericos e ignorada.
func LerTabMun(r io.Reader) ([]store.MunicipioIBGE, error) {
	var linhas []store.MunicipioIBGE
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		campos := strings.Split(strings.TrimRight(sc.Text(), "\r"), ";")
		if len(campos) != 5 {
			continue
		}
		siafi, err1 := strconv.Atoi(strings.TrimSpace(campos[0]))
		ibge, err2 := strconv.Atoi(strings.TrimSpace(campos[4]))
		nome := strings.TrimSpace(campos[2])
		uf := strings.TrimSpace(campos[3])
		if err1 != nil || err2 != nil || nome == "" || len(uf) != 2 {
			continue
		}
		linhas = append(linhas, store.MunicipioIBGE{
			CodigoSIAFI: int32(siafi),
			CodigoIBGE:  int32(ibge),
			Nome:        nome,
			UF:          uf,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("ler tabela de municipios: %w", err)
	}
	return linhas, nil
}

// CarregarIBGE le a tabela e grava o de-para em meta.municipio_ibge. Com
// menos linhas que minimoDeMunicipios recusa sem gravar nada.
func CarregarIBGE(ctx context.Context, pool *pgxpool.Pool, r io.Reader) (int, error) {
	linhas, err := LerTabMun(r)
	if err != nil {
		return 0, err
	}
	if len(linhas) < minimoDeMunicipios {
		return 0, fmt.Errorf("tabela de municipios com %d linhas validas; esperava ao menos %d — arquivo errado ou cortado?",
			len(linhas), minimoDeMunicipios)
	}
	if err := store.CarregarDeParaIBGE(ctx, pool, linhas); err != nil {
		return 0, err
	}
	return len(linhas), nil
}

// BaixarTabMun busca a tabela no Tesouro Transparente.
func BaixarTabMun(ctx context.Context, url string) (io.ReadCloser, error) {
	ctx, cancelar := context.WithTimeout(ctx, 2*time.Minute)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancelar()
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancelar()
		return nil, fmt.Errorf("baixar tabela de municipios: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancelar()
		return nil, fmt.Errorf("baixar tabela de municipios: HTTP %d", resp.StatusCode)
	}
	return corpoComCancelamento{resp.Body, cancelar}, nil
}

type corpoComCancelamento struct {
	io.ReadCloser
	cancelar context.CancelFunc
}

func (c corpoComCancelamento) Close() error {
	defer c.cancelar()
	return c.ReadCloser.Close()
}
