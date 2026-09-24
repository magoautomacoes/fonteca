package ingest

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magoautomacoes/fonteca/internal/store"
)

// zipDeMunicipios monta um ZIP no formato exato da Receita, em Latin-1.
func zipDeMunicipios(t *testing.T) string {
	t.Helper()
	// 0xC1 = A-agudo, 0xC7 = C-cedilha em Latin-1.
	conteudo := []byte(
		"\"0001\";\"GUAJAR\xc1-MIRIM\"\n" +
			"\"0002\";\"ALTO ALEGRE DOS PARECIS\"\n" +
			"\"7107\";\"SAO PAULO\"\n" +
			"\n" + // linha em branco no meio, como acontece de verdade
			"\"0003\"\n" + // linha truncada: so o codigo, sem nome
			"\"6001\";\"RIO DE JANEIRO\"\n" +
			"\"9999\";\"CONCEI\xc7\xc3O\"\n")

	caminho := filepath.Join(t.TempDir(), "Municipios.zip")
	arq, _ := os.Create(caminho)
	defer arq.Close()
	w := zip.NewWriter(arq)
	f, _ := w.Create("F.K03200$Z.D60912.MUNICCSV")
	f.Write(conteudo)
	w.Close()
	return caminho
}

func TestCarregarMunicipios(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	schema, err := store.NomeDoSchema("2026-09")
	if err != nil {
		t.Fatalf("NomeDoSchema: %v", err)
	}
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}

	n, err := CarregarMunicipios(ctx, pool, schema, zipDeMunicipios(t))
	if err != nil {
		t.Fatalf("CarregarMunicipios: %v", err)
	}
	if n != 5 {
		t.Errorf("carregou %d; quer 5 (a linha em branco e a truncada nao contam)", n)
	}

	var total int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM lote_2026_09.municipio`).Scan(&total); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if total != 5 {
		t.Errorf("no banco = %d; quer 5", total)
	}
}

// O teste que prova o encoding ponta a ponta: do byte Latin-1 no ZIP ate a
// linha no Postgres, sem mojibake.
func TestCarregarMunicipiosPreservaAcento(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if _, err := CarregarMunicipios(ctx, pool, schema, zipDeMunicipios(t)); err != nil {
		t.Fatalf("carregar: %v", err)
	}

	var nome string
	if err := pool.QueryRow(ctx,
		`SELECT nome FROM lote_2026_09.municipio WHERE codigo = 1`).Scan(&nome); err != nil {
		t.Fatalf("ler: %v", err)
	}
	if nome != "GUAJARÁ-MIRIM" {
		t.Errorf("nome = %q; quer %q — mojibake na carga", nome, "GUAJARÁ-MIRIM")
	}

	if err := pool.QueryRow(ctx,
		`SELECT nome FROM lote_2026_09.municipio WHERE codigo = 9999`).Scan(&nome); err != nil {
		t.Fatalf("ler: %v", err)
	}
	if nome != "CONCEIÇÃO" {
		t.Errorf("nome = %q; quer %q", nome, "CONCEIÇÃO")
	}
}

// O circuito completo do design: carregar num schema novo enquanto o antigo
// serve, e trocar atomicamente no fim.
func TestCicloCompletoDeLote(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}

	n, err := CarregarMunicipios(ctx, pool, schema, zipDeMunicipios(t))
	if err != nil {
		t.Fatalf("carregar: %v", err)
	}

	// Antes da troca, nao ha lote vigente.
	if vig, err := store.LoteVigente(ctx, pool); err != nil || vig != nil {
		t.Fatalf("vigente antes da troca = %v (err %v); queria nil", vig, err)
	}

	if err := store.CriarIndices(ctx, pool, schema); err != nil {
		t.Fatalf("indices: %v", err)
	}
	if err := store.TrocarLoteVigente(ctx, pool, schema, "2026-09", n); err != nil {
		t.Fatalf("trocar: %v", err)
	}

	vig, err := store.LoteVigente(ctx, pool)
	if err != nil {
		t.Fatalf("LoteVigente: %v", err)
	}
	if vig == nil || vig.Schema != schema {
		t.Fatalf("vigente = %v; quer %s", vig, schema)
	}
	if vig.Linhas != n {
		t.Errorf("linhas = %d; quer %d", vig.Linhas, n)
	}
}

func TestCarregarMunicipiosRecusaSchemaInvalido(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	// Cria public.municipio com o mesmo formato, para que um COPY chegando ate
	// o banco FUNCIONASSE. Assim a unica razao possivel de erro e a validacao
	// do nome ter barrado antes de tocar no banco — sem isso o teste passaria
	// tambem com ValidarSchema removido, pela tabela simplesmente nao existir.
	_, err := pool.Exec(ctx, `CREATE TABLE public.municipio (
		codigo integer PRIMARY KEY, nome text NOT NULL)`)
	if err != nil {
		t.Fatalf("preparar public.municipio: %v", err)
	}

	_, err = CarregarMunicipios(ctx, pool, "public", zipDeMunicipios(t))
	if err == nil {
		t.Fatal("aceitou schema 'public'; devia recusar")
	}
	if !strings.Contains(err.Error(), "nome de schema invalido") {
		t.Errorf("erro = %v; queria a mensagem de ValidarSchema — outro erro "+
			"significa que a validacao nao foi quem barrou", err)
	}

	// E nada pode ter sido gravado.
	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.municipio`).Scan(&total); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if total != 0 {
		t.Errorf("gravou %d linhas em public.municipio; a validacao devia ter "+
			"barrado antes de qualquer escrita", total)
	}
}
