package ingest

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/magoautomacoes/fonteca/internal/store"
)

// zipAuxiliar monta um ZIP de tabela auxiliar no formato da Receita
// (codigo;descricao, entre aspas, Latin-1).
func zipAuxiliar(t *testing.T, nome string, conteudo []byte) string {
	t.Helper()
	caminho := filepath.Join(t.TempDir(), nome)
	arq, err := os.Create(caminho)
	if err != nil {
		t.Fatalf("criar zip: %v", err)
	}
	defer arq.Close()
	w := zip.NewWriter(arq)
	f, _ := w.Create("F.K03200$Z.D60912.AUXCSV")
	f.Write(conteudo)
	w.Close()
	return caminho
}

func TestCarregarCnaes(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}

	// 0xE7 = c-cedilha, 0xE3 = a-til em Latin-1.
	caminho := zipAuxiliar(t, "Cnaes.zip", []byte(
		"\"2512800\";\"Fabrica\xe7\xe3o de esquadrias de metal\"\n"+
			"\"4744001\";\"Com\xe9rcio varejista de ferragens e ferramentas\"\n"+
			"\"251280\";\"codigo curto: nunca casaria no join\"\n"+
			"\"25128OO\";\"codigo com letra\"\n"+
			"\"1091102\"\n")) // truncada: sem descricao

	n, err := CarregarCnaes(ctx, pool, schema, caminho)
	if err != nil {
		t.Fatalf("CarregarCnaes: %v", err)
	}
	if n != 2 {
		t.Errorf("carregou %d; quer 2 (codigo curto, com letra e linha truncada nao contam)", n)
	}

	var descricao string
	if err := pool.QueryRow(ctx,
		`SELECT descricao FROM lote_2026_09.cnae WHERE codigo = '2512800'`).
		Scan(&descricao); err != nil {
		t.Fatalf("ler cnae: %v", err)
	}
	if descricao != "Fabricação de esquadrias de metal" {
		t.Errorf("descricao = %q: encoding ou aspas nao foram tratados", descricao)
	}
}

func TestCarregarNaturezas(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}

	caminho := zipAuxiliar(t, "Naturezas.zip", []byte(
		"\"2062\";\"Sociedade Empres\xe1ria Limitada\"\n"+
			"\"2135\";\"Empres\xe1rio (Individual)\"\n"+
			"\"2062\";\"duplicada: a primeira vence\"\n"))

	n, err := CarregarNaturezas(ctx, pool, schema, caminho)
	if err != nil {
		t.Fatalf("CarregarNaturezas: %v", err)
	}
	if n != 2 {
		t.Errorf("carregou %d; quer 2 (a duplicada nao entra)", n)
	}

	var descricao string
	if err := pool.QueryRow(ctx,
		`SELECT descricao FROM lote_2026_09.natureza WHERE codigo = '2062'`).
		Scan(&descricao); err != nil {
		t.Fatalf("ler natureza: %v", err)
	}
	if descricao != "Sociedade Empresária Limitada" {
		t.Errorf("descricao = %q", descricao)
	}
}
