package ingest

import (
	"context"
	"testing"

	"github.com/magoautomacoes/fonteca/internal/store"
)

func TestCarregarGenerico(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	schema, err := store.NomeDoSchema("2026-09")
	if err != nil {
		t.Fatalf("NomeDoSchema: %v", err)
	}
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}

	n, err := Carregar(ctx, pool, schema, TabelaMunicipio, zipDeMunicipios(t))
	if err != nil {
		t.Fatalf("Carregar: %v", err)
	}
	if n != 5 {
		t.Errorf("carregou %d; quer 5 (branco e truncada nao contam)", n)
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

func TestCarregarRecusaSchemaInvalido(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	// Cria public.municipio para que um COPY que CHEGASSE ao banco funcionasse;
	// assim a unica causa possivel de erro e a validacao ter barrado antes.
	if _, err := pool.Exec(ctx, `CREATE TABLE public.municipio (
		codigo integer PRIMARY KEY, nome text NOT NULL)`); err != nil {
		t.Fatalf("preparar public.municipio: %v", err)
	}

	_, err := Carregar(ctx, pool, "public", TabelaMunicipio, zipDeMunicipios(t))
	if err == nil {
		t.Fatal("aceitou schema 'public'")
	}

	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.municipio`).Scan(&total); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if total != 0 {
		t.Errorf("gravou %d linhas; a validacao devia barrar antes de escrever", total)
	}
}

// A base da Receita tem CNPJ repetido de verdade: no lote 2026-09 o
// Empresas2.zip traz o basico 08314885 duas vezes — uma linha real
// ("FLAVIO PAVAO DE SOUZA") e uma linha-fantasma vazia, natureza "0000".
// Com COPY direto na tabela real, essa UMA linha em 4,5 milhoes abortava a
// carga inteira. A primeira ocorrencia vence e a carga sobrevive.
func TestCarregarSobreviveAChaveDuplicada(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	schema, err := store.NomeDoSchema("2026-09")
	if err != nil {
		t.Fatalf("NomeDoSchema: %v", err)
	}
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}

	// Duas linhas com o mesmo codigo, como no arquivo real.
	conteudo := []byte(
		"\"1\";\"PRIMEIRA\"\n" +
			"\"2\";\"SEGUNDA\"\n" +
			"\"2\";\"\"\n" + // a fantasma, mesmo codigo e nome vazio
			"\"3\";\"TERCEIRA\"\n")

	n, err := Carregar(ctx, pool, schema, TabelaMunicipio, zipDeTeste(t, conteudo))
	if err != nil {
		t.Fatalf("a chave duplicada abortou a carga: %v", err)
	}
	if n != 3 {
		t.Errorf("carregou %d; quer 3 (a duplicata e descartada, nao aborta)", n)
	}

	// A PRIMEIRA ocorrencia vence: o nome preenchido, nao o vazio.
	var nome string
	err = pool.QueryRow(ctx,
		`SELECT nome FROM lote_2026_09.municipio WHERE codigo = 2`).Scan(&nome)
	if err != nil {
		t.Fatalf("ler o codigo 2: %v", err)
	}
	if nome != "SEGUNDA" {
		t.Errorf("nome = %q; quer SEGUNDA (a primeira ocorrencia vence)", nome)
	}

	var total int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM lote_2026_09.municipio`).Scan(&total); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if total != 3 {
		t.Errorf("no banco = %d; quer 3", total)
	}
}
