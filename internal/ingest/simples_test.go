package ingest

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/magoautomacoes/fonteca/internal/store"
)

func zipDeSimples(t *testing.T) string {
	t.Helper()
	// Layout real, medido em 17/09/2026: 7 campos.
	conteudo := []byte(
		"\"10000001\";\"S\";\"20200101\";\"00000000\";\"N\";\"00000000\";\"00000000\"\n" +
			"\"10000005\";\"S\";\"20210301\";\"00000000\";\"S\";\"20210301\";\"00000000\"\n" +
			"\"10000011\";\"N\";\"00000000\";\"00000000\";\"S\";\"20190601\";\"00000000\"\n" +
			"\n" +
			"\"10000099\";\"S\"\n" + // truncada: so 2 campos, nao alcanca o MEI
			"\"10000004\";\"N\";\"00000000\";\"00000000\";\"N\";\"00000000\";\"00000000\"\n")

	caminho := filepath.Join(t.TempDir(), "Simples.zip")
	arq, _ := os.Create(caminho)
	defer arq.Close()
	w := zip.NewWriter(arq)
	// Nome interno segue o padrao real deste arquivo, diferente do Municipios.
	f, _ := w.Create("F.K03200$W.SIMPLES.CSV.D60912")
	f.Write(conteudo)
	w.Close()
	return caminho
}

func TestCarregarSimples(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}

	n, err := CarregarSimples(ctx, pool, schema, zipDeSimples(t))
	if err != nil {
		t.Fatalf("CarregarSimples: %v", err)
	}
	if n != 4 {
		t.Errorf("carregou %d; quer 4 (a linha em branco e a truncada nao contam)", n)
	}

	// A linha truncada nao pode ter virado registro: sem o guarda de
	// comprimento, o acesso a campos[4] entraria em panic e derrubaria a
	// carga inteira em vez de pular uma linha.
	var existe bool
	err = pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM lote_2026_09.simples WHERE cnpj_basico = '10000099')`).Scan(&existe)
	if err != nil {
		t.Fatalf("checar truncada: %v", err)
	}
	if existe {
		t.Error("a linha truncada virou registro; o guarda de comprimento nao funcionou")
	}
}

// "S" e "N" precisam virar boolean de verdade: e sobre esse campo que o
// filtro "excluindo MEI" da consulta-alvo opera.
func TestCarregarSimplesConverteFlags(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if _, err := CarregarSimples(ctx, pool, schema, zipDeSimples(t)); err != nil {
		t.Fatalf("carregar: %v", err)
	}

	casos := []struct {
		cnpj    string
		simples bool
		mei     bool
	}{
		{"10000001", true, false},
		{"10000005", true, true},
		{"10000011", false, true},
		{"10000004", false, false},
	}
	for _, c := range casos {
		var s, m bool
		err := pool.QueryRow(ctx,
			`SELECT opcao_simples, opcao_mei FROM lote_2026_09.simples WHERE cnpj_basico = $1`,
			c.cnpj).Scan(&s, &m)
		if err != nil {
			t.Fatalf("ler %s: %v", c.cnpj, err)
		}
		if s != c.simples || m != c.mei {
			t.Errorf("%s: simples=%v mei=%v; quer %v/%v", c.cnpj, s, m, c.simples, c.mei)
		}
	}

	// O filtro da consulta-alvo: quantos NAO sao MEI.
	var naoMEI int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM lote_2026_09.simples WHERE NOT opcao_mei`).Scan(&naoMEI); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if naoMEI != 2 {
		t.Errorf("nao-MEI = %d; quer 2", naoMEI)
	}
}
