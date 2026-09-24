package ingest

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/magoautomacoes/fonteca/internal/store"
)

func zipDeEmpresas(t *testing.T) string {
	t.Helper()
	// 0xC7 = C-cedilha, 0xD5 = O-til em Latin-1: o acento tem que sobreviver.
	conteudo := []byte(
		"\"10000001\";\"METALURGICA ALFA LTDA\";\"2062\";\"49\";\"150000,00\";\"03\";\"\"\n" +
			"\"10000004\";\"CONSTRU\xc7\xd5ES DELTA LTDA\";\"2062\";\"49\";\"300000,50\";\"03\";\"\"\n" +
			"\"10000005\";\"METALURGICA EPSILON LTDA\";\"2135\";\"50\";\"0,00\";\"01\";\"\"\n" +
			"\n" +
			"\"10000010\";\"METALURGICA KAPPA LTDA\";\"2062\";\"49\";\"nao-e-numero\";\"01\";\"\"\n" +
			"\"10000099\";\"TRUNCADA LTDA\"\n" + // so 2 campos: nao alcanca o capital
			"\"10000009\";\"PADARIA IOTA LTDA\";\"2062\";\"49\";\"30000,00\";\"01\";\"\"\n")

	caminho := filepath.Join(t.TempDir(), "Empresas0.zip")
	arq, _ := os.Create(caminho)
	defer arq.Close()
	w := zip.NewWriter(arq)
	f, _ := w.Create("F.K03200$Z.D60912.EMPRECSV")
	f.Write(conteudo)
	w.Close()
	return caminho
}

func TestCarregarEmpresas(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}

	n, err := CarregarEmpresas(ctx, pool, schema, zipDeEmpresas(t))
	if err != nil {
		t.Fatalf("CarregarEmpresas: %v", err)
	}
	// 5 linhas de dados validas (a de capital ilegivel entra com capital
	// zero, nao e descartada) + a linha em branco e a truncada nao contam.
	if n != 5 {
		t.Errorf("carregou %d; quer 5", n)
	}

	// A truncada nao pode ter virado registro.
	var existe bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM lote_2026_09.empresa WHERE cnpj_basico = '10000099')`).Scan(&existe); err != nil {
		t.Fatalf("checar truncada: %v", err)
	}
	if existe {
		t.Error("a linha truncada virou registro")
	}
}

// Capital ilegivel nao pode descartar a empresa: o nome e o CNPJ continuam
// servindo para prospeccao. A linha entra com capital zero, nao some.
func TestCarregarEmpresasMantemEmpresaComCapitalIlegivel(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if _, err := CarregarEmpresas(ctx, pool, schema, zipDeEmpresas(t)); err != nil {
		t.Fatalf("carregar: %v", err)
	}

	var razao string
	var capital float64
	err := pool.QueryRow(ctx,
		`SELECT razao_social, capital_social FROM lote_2026_09.empresa
		 WHERE cnpj_basico = '10000010'`).Scan(&razao, &capital)
	if err != nil {
		t.Fatalf("a empresa com capital ilegivel sumiu do banco: %v", err)
	}
	if razao != "METALURGICA KAPPA LTDA" {
		t.Errorf("razao = %q; quer %q", razao, "METALURGICA KAPPA LTDA")
	}
	if capital != 0 {
		t.Errorf("capital = %v; quer 0 quando o campo e ilegivel", capital)
	}
}

// Capital social usa virgula decimal no arquivo e precisa virar numeric sem
// perder centavo — e dinheiro.
func TestCarregarEmpresasConverteCapital(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if _, err := CarregarEmpresas(ctx, pool, schema, zipDeEmpresas(t)); err != nil {
		t.Fatalf("carregar: %v", err)
	}

	// Lido como texto: a comparacao e exata, sem float no meio.
	var capital string
	err := pool.QueryRow(ctx,
		`SELECT capital_social::text FROM lote_2026_09.empresa WHERE cnpj_basico = '10000004'`).
		Scan(&capital)
	if err != nil {
		t.Fatalf("ler capital: %v", err)
	}
	if capital != "300000.50" {
		t.Errorf("capital = %s; quer 300000.50 (virgula decimal convertida)", capital)
	}
}

// A razao social e o que aparece na lista de leads: mojibake aqui e visivel
// para o vendedor.
func TestCarregarEmpresasPreservaAcento(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if _, err := CarregarEmpresas(ctx, pool, schema, zipDeEmpresas(t)); err != nil {
		t.Fatalf("carregar: %v", err)
	}

	var razao string
	err := pool.QueryRow(ctx,
		`SELECT razao_social FROM lote_2026_09.empresa WHERE cnpj_basico = '10000004'`).
		Scan(&razao)
	if err != nil {
		t.Fatalf("ler: %v", err)
	}
	if razao != "CONSTRUÇÕES DELTA LTDA" {
		t.Errorf("razao = %q; quer %q", razao, "CONSTRUÇÕES DELTA LTDA")
	}
}

// Capital de 16 digitos inteiros com centavos: via float64 viraria
// 1234567890123456.75. O valor tem de chegar ao banco exatamente como veio, e
// o capital grande demais para numeric(18,2) nao pode abortar a carga.
func TestCarregarEmpresasCapitalExatoEOverflowNaoAbortam(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}

	caminho := zipAuxiliar(t, "Empresas0.zip", []byte(
		"\"20000001\";\"GRANDE LTDA\";\"2062\";\"49\";\"1234567890123456,78\";\"05\";\"\"\n"+
			"\"20000002\";\"ESTOURO LTDA\";\"2062\";\"49\";\"99999999999999999,00\";\"05\";\"\"\n"))
	if _, err := CarregarEmpresas(ctx, pool, schema, caminho); err != nil {
		t.Fatalf("carregar: %v (capital fora de faixa abortou a carga?)", err)
	}

	capitais := map[string]string{}
	rows, err := pool.Query(ctx, `SELECT cnpj_basico, capital_social::text FROM lote_2026_09.empresa`)
	if err != nil {
		t.Fatalf("ler: %v", err)
	}
	for rows.Next() {
		var cnpj, capital string
		if err := rows.Scan(&cnpj, &capital); err != nil {
			t.Fatalf("scan: %v", err)
		}
		capitais[cnpj] = capital
	}
	if capitais["20000001"] != "1234567890123456.78" {
		t.Errorf("capital grande = %q; quer 1234567890123456.78 exato", capitais["20000001"])
	}
	if capitais["20000002"] != "0.00" {
		t.Errorf("capital fora de faixa = %q; quer 0.00 (empresa mantida, capital zerado)", capitais["20000002"])
	}
}
