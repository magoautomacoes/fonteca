package ingest

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/magoautomacoes/fonteca/internal/store"
)

// linhaEstab monta uma linha de 30 campos preenchendo so os que importam.
func linhaEstab(vals map[int]string) string {
	campos := make([]string, 30)
	for i := range campos {
		campos[i] = "\"\""
	}
	for i, v := range vals {
		campos[i] = "\"" + v + "\""
	}
	return strings.Join(campos, ";")
}

func zipDeEstabelecimentos(t *testing.T) string {
	t.Helper()
	linhas := []string{
		// ALVO: CNAE 2512800, ativa, celular de 9 digitos, SP, jul/2026.
		// Preenche TODOS os 16 campos guardados, incluindo cnae_secundarios,
		// ddd_2 e telefone_2, para que TestCarregarEstabelecimentosMapeiaTodasAsColunas
		// consiga travar cada indice/posicao — inclusive uma troca entre
		// ddd_2 e telefone_2, que ficaria invisivel se um dos dois for vazio.
		linhaEstab(map[int]string{
			0: "10000001", 1: "0001", 2: "01", 3: "1", 4: "ALFA",
			5: "02", 6: "20260710", 10: "20260710", 11: "2512800",
			12: "2512800,4321000", 19: "SP", 20: "7107",
			21: "11", 22: "98888777", 23: "21", 24: "22221111",
			27: "alfa@ex.com",
		}),
		// FIXO LEGADO de 8 digitos comecando em 9: NAO pode virar celular
		linhaEstab(map[int]string{
			0: "10000002", 1: "0001", 2: "02", 3: "1", 4: "BETA",
			5: "02", 6: "20260711", 10: "20260711", 11: "2512800",
			19: "SP", 20: "7107", 21: "11", 22: "34445555",
		}),
		// BAIXADA
		linhaEstab(map[int]string{
			0: "10000003", 1: "0001", 2: "03", 3: "1", 4: "GAMA",
			5: "08", 6: "20260801", 10: "20260705", 11: "2512800",
			19: "SP", 20: "7107", 21: "11", 22: "96666555",
		}),
		"",
		// Outra UF
		linhaEstab(map[int]string{
			0: "10000010", 1: "0001", 2: "10", 3: "1", 4: "KAPPA",
			5: "02", 6: "20260718", 10: "20260718", 11: "2512800",
			19: "RJ", 20: "6001", 21: "21", 22: "93333222",
		}),
		// TRUNCADA: poucos campos reais, nao alcanca nem o cnpj_ordem.
		"\"10000099\";\"0001\"",
		// Data ilegivel (30/02) NAO pode descartar o estabelecimento.
		linhaEstab(map[int]string{
			0: "10000020", 1: "0001", 2: "20", 3: "1", 4: "OMEGA",
			5: "02", 6: "20260230", 10: "20260230", 11: "2512800",
			19: "SP", 20: "7107", 21: "11", 22: "97777666",
		}),
		// UF com lixo, que a analise de fase 2 documenta na base real. Sem o
		// guarda de comprimento isso aborta o COPY inteiro com "value too long
		// for type character(2)" — uma linha ruim mataria horas de carga.
		linhaEstab(map[int]string{
			0: "10000021", 1: "0001", 2: "21", 3: "1", 4: "UF SUJA",
			5: "02", 6: "20260720", 10: "20260720", 11: "2512800",
			19: "EXTERIOR", 20: "9999", 21: "11", 22: "95555444",
		}),
	}

	caminho := filepath.Join(t.TempDir(), "Estabelecimentos0.zip")
	arq, _ := os.Create(caminho)
	defer arq.Close()
	w := zip.NewWriter(arq)
	f, _ := w.Create("F.K03200$Z.D60912.ESTABELE")
	f.Write([]byte(strings.Join(linhas, "\n") + "\n"))
	w.Close()
	return caminho
}

func TestCarregarEstabelecimentos(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}

	n, err := CarregarEstabelecimentos(ctx, pool, schema, zipDeEstabelecimentos(t))
	if err != nil {
		t.Fatalf("CarregarEstabelecimentos: %v", err)
	}
	// 6 linhas validas (ALFA, BETA, GAMA, KAPPA, OMEGA, UF SUJA) — a data
	// ilegivel nao descarta OMEGA e a UF suja nao descarta UF SUJA. A linha
	// em branco e a truncada nao contam.
	if n != 6 {
		t.Errorf("carregou %d; quer 6", n)
	}

	// A truncada nao pode ter virado registro.
	var existe bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM lote_2026_09.estabelecimento WHERE cnpj_basico = '10000099')`).Scan(&existe); err != nil {
		t.Fatalf("checar truncada: %v", err)
	}
	if existe {
		t.Error("a linha truncada virou registro")
	}
}

// O CNPJ completo e a concatenacao de tres campos separados no arquivo.
func TestCarregarEstabelecimentosMontaCNPJ(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if _, err := CarregarEstabelecimentos(ctx, pool, schema, zipDeEstabelecimentos(t)); err != nil {
		t.Fatalf("carregar: %v", err)
	}

	var cnpj string
	err := pool.QueryRow(ctx,
		`SELECT cnpj FROM lote_2026_09.estabelecimento WHERE cnpj_basico = '10000001'`).
		Scan(&cnpj)
	if err != nil {
		t.Fatalf("ler: %v", err)
	}
	if cnpj != "10000001000101" {
		t.Errorf("cnpj = %q; quer %q (basico+ordem+dv)", cnpj, "10000001000101")
	}
}

// A coluna gerada tem que classificar certo o que vem do arquivo real:
// 9 digitos comecando em 9 e celular; 8 digitos comecando em 9 nao e.
func TestCarregarEstabelecimentosClassificaCelular(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if _, err := CarregarEstabelecimentos(ctx, pool, schema, zipDeEstabelecimentos(t)); err != nil {
		t.Fatalf("carregar: %v", err)
	}

	casos := []struct {
		basico string
		quer   bool
		porque string
	}{
		{"10000001", true, "98888777: 8 digitos comecando em 9, como a Receita publica"},
		{"10000002", false, "34445555: fixo, nao comeca em 9"},
	}
	for _, c := range casos {
		var celular bool
		err := pool.QueryRow(ctx,
			`SELECT celular FROM lote_2026_09.estabelecimento WHERE cnpj_basico = $1`,
			c.basico).Scan(&celular)
		if err != nil {
			t.Fatalf("ler %s: %v", c.basico, err)
		}
		if celular != c.quer {
			t.Errorf("%s: celular = %v; quer %v (%s)", c.basico, celular, c.quer, c.porque)
		}
	}
}

// Situacao vem como texto de dois digitos e precisa virar o smallint que os
// indices parciais usam.
func TestCarregarEstabelecimentosConverteSituacao(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if _, err := CarregarEstabelecimentos(ctx, pool, schema, zipDeEstabelecimentos(t)); err != nil {
		t.Fatalf("carregar: %v", err)
	}

	var ativas int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM lote_2026_09.estabelecimento WHERE situacao = 2`).Scan(&ativas); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if ativas != 5 {
		t.Errorf("ativas = %d; quer 5 (uma esta baixada)", ativas)
	}
}

// Data ilegivel (30 de fevereiro) nao pode descartar o estabelecimento: o
// CNPJ e o telefone continuam servindo para prospeccao mesmo sem a data.
func TestCarregarEstabelecimentosDataIlegivelNaoDescartaLinha(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if _, err := CarregarEstabelecimentos(ctx, pool, schema, zipDeEstabelecimentos(t)); err != nil {
		t.Fatalf("carregar: %v", err)
	}

	var nomeFantasia string
	var dataInicio *string
	err := pool.QueryRow(ctx,
		`SELECT nome_fantasia, data_inicio::text FROM lote_2026_09.estabelecimento WHERE cnpj_basico = '10000020'`).
		Scan(&nomeFantasia, &dataInicio)
	if err != nil {
		t.Fatalf("a linha com data ilegivel sumiu do banco: %v", err)
	}
	if nomeFantasia != "OMEGA" {
		t.Errorf("nome_fantasia = %q; quer %q", nomeFantasia, "OMEGA")
	}
	if dataInicio != nil {
		t.Errorf("data_inicio = %v; quer nil quando a data e ilegivel", *dataInicio)
	}
}

// UF com lixo vira string vazia em vez de abortar o COPY. A coluna e char(2):
// um valor maior derruba a carga inteira da parte, nao so a linha.
func TestCarregarEstabelecimentosUFSujaNaoAbortaCarga(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if _, err := CarregarEstabelecimentos(ctx, pool, schema, zipDeEstabelecimentos(t)); err != nil {
		t.Fatalf("a UF suja abortou a carga: %v", err)
	}

	var uf string
	err := pool.QueryRow(ctx,
		`SELECT uf FROM lote_2026_09.estabelecimento WHERE cnpj_basico = '10000021'`).
		Scan(&uf)
	if err != nil {
		t.Fatalf("a linha com UF suja sumiu: %v", err)
	}
	if strings.TrimSpace(uf) != "" {
		t.Errorf("uf = %q; quer vazia — o guarda de comprimento nao funcionou", uf)
	}
}

// Trava o mapa de indices inteiro numa linha so. Um deslize de indice (ou uma
// troca na ordem do slice de valores) corromperia uma coluna em 63 milhoes de
// linhas sem quebrar nenhum outro teste.
func TestCarregarEstabelecimentosMapeiaTodasAsColunas(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if _, err := CarregarEstabelecimentos(ctx, pool, schema, zipDeEstabelecimentos(t)); err != nil {
		t.Fatalf("carregar: %v", err)
	}

	var (
		cnpj, basico, fantasia, cnae, cnaeSec, uf string
		ddd1, tel1, ddd2, tel2, email             string
		matriz, situacao                          int16
		municipio                                 int32
		dataSit, dataIni                          *time.Time
	)
	err := pool.QueryRow(ctx, `
		SELECT cnpj, cnpj_basico, matriz_filial, nome_fantasia, situacao,
		       data_situacao, data_inicio, cnae_principal, cnae_secundarios,
		       uf, municipio, ddd_1, telefone_1, ddd_2, telefone_2, email
		FROM lote_2026_09.estabelecimento WHERE cnpj_basico = '10000001'`).
		Scan(&cnpj, &basico, &matriz, &fantasia, &situacao,
			&dataSit, &dataIni, &cnae, &cnaeSec,
			&uf, &municipio, &ddd1, &tel1, &ddd2, &tel2, &email)
	if err != nil {
		t.Fatalf("ler ALFA: %v", err)
	}

	confere := func(nome, got, quer string) {
		t.Helper()
		if strings.TrimSpace(got) != quer {
			t.Errorf("%s = %q; quer %q", nome, got, quer)
		}
	}
	confere("cnpj", cnpj, "10000001000101")
	confere("cnpj_basico", basico, "10000001")
	confere("nome_fantasia", fantasia, "ALFA")
	confere("cnae_principal", cnae, "2512800")
	confere("cnae_secundarios", cnaeSec, "2512800,4321000")
	confere("uf", uf, "SP")
	confere("ddd_1", ddd1, "11")
	confere("telefone_1", tel1, "98888777")
	confere("ddd_2", ddd2, "21")
	confere("telefone_2", tel2, "22221111")
	confere("email", email, "alfa@ex.com")

	if matriz != 1 {
		t.Errorf("matriz_filial = %d; quer 1", matriz)
	}
	if situacao != 2 {
		t.Errorf("situacao = %d; quer 2", situacao)
	}
	if municipio != 7107 {
		t.Errorf("municipio = %d; quer 7107", municipio)
	}
	if dataIni == nil || dataIni.Format("20060102") != "20260710" {
		t.Errorf("data_inicio = %v; quer 2026-07-10", dataIni)
	}
	if dataSit == nil || dataSit.Format("20060102") != "20260710" {
		t.Errorf("data_situacao = %v; quer 2026-07-10", dataSit)
	}
}

// O filtro por CNAE e o que torna a carga viavel para o caso de uso da
// Medido no lote 2026-09, quatro CNAEs de um mesmo ramo aparecem em
// 1,95% dos estabelecimentos. Guardar os outros 98% custa horas de carga e
// dezenas de GB para responder consultas que nunca os tocam.
func TestCarregarEstabelecimentosComFiltroDeCNAE(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}

	// So o CNAE 2512800, e sem contar secundarios.
	filtro := NovoFiltroCNAE([]string{"2512800"}, false)

	n, err := CarregarEstabelecimentosCom(ctx, pool, schema, zipDeEstabelecimentos(t), filtro)
	if err != nil {
		t.Fatalf("CarregarEstabelecimentosCom: %v", err)
	}

	// Todas as linhas da fixture usam 2512800, entao o filtro nao descarta
	// nenhuma — o que prova que ele nao rejeita o que devia aceitar.
	var comCNAE int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM lote_2026_09.estabelecimento
		 WHERE cnae_principal = '2512800'`).Scan(&comCNAE); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if int64(comCNAE) != n {
		t.Errorf("carregou %d mas so %d tem o CNAE do filtro", n, comCNAE)
	}
	if n == 0 {
		t.Error("o filtro descartou tudo; devia aceitar as linhas com 2512800")
	}
}

// O outro lado: um CNAE que nao existe na fixture descarta tudo, sem erro.
// Descartar nao e falhar — a carga termina limpa com zero linhas.
func TestCarregarEstabelecimentosFiltroDescartaTudo(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := store.NomeDoSchema("2026-09")
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}

	filtro := NovoFiltroCNAE([]string{"9999999"}, false) // ninguem tem

	n, err := CarregarEstabelecimentosCom(ctx, pool, schema, zipDeEstabelecimentos(t), filtro)
	if err != nil {
		t.Fatalf("descartar tudo nao pode virar erro: %v", err)
	}
	if n != 0 {
		t.Errorf("carregou %d; quer 0 (nenhuma linha tem esse CNAE)", n)
	}
}
