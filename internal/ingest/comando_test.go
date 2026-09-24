package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/magoautomacoes/fonteca/internal/source"
	"github.com/magoautomacoes/fonteca/internal/store"
)

// xmlDeLoteMunicipios imita a resposta 207 do Nextcloud para uma pasta de
// lote que so tem Municipios.zip, no mesmo formato que internal/source usa
// em seus proprios testes (ver internal/source/listagem_test.go).
const xmlDeLoteMunicipios = `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/public.php/webdav/Dados/Cadastros/CNPJ/2026-09/</d:href>
    <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop>
    <d:status>HTTP/1.1 200 OK</d:status></d:propstat>
  </d:response>
  <d:response>
    <d:href>/public.php/webdav/Dados/Cadastros/CNPJ/2026-09/Municipios.zip</d:href>
    <d:propstat><d:prop>
      <d:getcontentlength>{{tamanho}}</d:getcontentlength>
      <d:getlastmodified>Sun, 13 Sep 2026 16:02:02 GMT</d:getlastmodified>
      <d:resourcetype/>
    </d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>
  </d:response>
</d:multistatus>`

// servidorDeLoteMunicipios sobe um servidor local que responde tanto ao
// PROPFIND (listagem WebDAV) quanto ao GET (download) do Municipios.zip,
// para que os testes de Executar nunca precisem da rede real.
func servidorDeLoteMunicipios(t *testing.T, conteudoZip []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			corpo := strings.ReplaceAll(xmlDeLoteMunicipios, "{{tamanho}}",
				strconv.Itoa(len(conteudoZip)))
			w.WriteHeader(http.StatusMultiStatus)
			w.Write([]byte(corpo))
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			w.Write(conteudoZip)
		default:
			t.Errorf("metodo inesperado: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func lerArquivo(t *testing.T, caminho string) []byte {
	t.Helper()
	dados, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatalf("ler %s: %v", caminho, err)
	}
	return dados
}

// TestOpcoesValidaReferencia nao chega a usar a rede: com Referencia
// preenchida (mesmo que invalida), Executar valida o formato via
// store.NomeDoSchema antes de listar o lote. A fonte aponta para um
// servidor que falha o teste se for tocado, para provar isso.
func TestOpcoesValidaReferencia(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Executar foi a rede antes de validar a referencia")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	f := &source.Fonte{BaseURL: srv.URL, Hash: "TESTE123", HTTP: srv.Client()}

	err := Executar(ctx, pool, Opcoes{Referencia: "2026-9", DryRun: true, Fonte: f})
	if err == nil {
		t.Fatal("aceitou referencia malformada")
	}
}

func TestDryRunNaoTocaNoBanco(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	zip := lerArquivo(t, zipDeMunicipios(t))
	srv := servidorDeLoteMunicipios(t, zip)
	f := &source.Fonte{BaseURL: srv.URL, Hash: "TESTE123", HTTP: srv.Client()}

	if err := Executar(ctx, pool, Opcoes{Referencia: "2026-09", DryRun: true, Fonte: f}); err != nil {
		t.Fatalf("Executar (dry-run): %v", err)
	}

	var schemas int
	err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.schemata
		WHERE schema_name LIKE 'lote_%'`).Scan(&schemas)
	if err != nil {
		t.Fatalf("contar schemas: %v", err)
	}
	if schemas != 0 {
		t.Errorf("dry-run criou %d schema(s); nao devia criar nenhum", schemas)
	}
}

// Com um lote ja vigente e igual ao que seria importado, Executar deve sair
// imediatamente. E o que torna seguro o cron rodar todo dia: a Receita nao tem
// dia fixo de publicacao, entao a maioria das execucoes nao tem nada a fazer.
func TestExecutarSaiSeLoteJaImportado(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	schema, err := store.NomeDoSchema("2026-09")
	if err != nil {
		t.Fatalf("NomeDoSchema: %v", err)
	}
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if err := store.TrocarLoteVigente(ctx, pool, schema, "2026-09", 5572); err != nil {
		t.Fatalf("trocar: %v", err)
	}

	// Fonte que FALHA se for usada: se Executar nao sair cedo, o teste quebra.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Executar foi a rede apesar do lote ja estar importado")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	f := &source.Fonte{BaseURL: srv.URL, Hash: "TESTE123", HTTP: srv.Client()}

	if err := Executar(ctx, pool, Opcoes{Referencia: "2026-09", Fonte: f}); err != nil {
		t.Fatalf("Executar devia sair sem erro: %v", err)
	}

	// E o lote vigente continua o mesmo, com as linhas originais.
	vig, err := store.LoteVigente(ctx, pool)
	if err != nil {
		t.Fatalf("LoteVigente: %v", err)
	}
	if vig == nil || vig.Schema != schema || vig.Linhas != 5572 {
		t.Errorf("vigente = %v; o lote existente foi alterado", vig)
	}
}

// Prova o caminho feliz: um ingest completo troca o lote vigente para o
// novo, com as linhas certas, e o lote antigo acaba descartado. A garantia
// de ORDEM (descartar so DEPOIS que a troca teve sucesso) fica coberta por
// TestExecutarFalhaNaTrocaMantemLoteAntigoIntacto, que forca a troca a
// falhar e verifica que o antigo sobrevive — aqui, no caminho sem falha,
// so importa o resultado final.
func TestExecutarTrocaLoteVigenteQuandoHaLoteAntigo(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	// Semeia um lote antigo (2026-08) vigente.
	schemaAntigo, err := store.NomeDoSchema("2026-08")
	if err != nil {
		t.Fatalf("NomeDoSchema antigo: %v", err)
	}
	if err := store.CriarSchemaDeLote(ctx, pool, schemaAntigo); err != nil {
		t.Fatalf("criar schema antigo: %v", err)
	}
	if err := store.TrocarLoteVigente(ctx, pool, schemaAntigo, "2026-08", 100); err != nil {
		t.Fatalf("trocar para o antigo: %v", err)
	}

	zip := lerArquivo(t, zipDeMunicipios(t))
	srv := servidorDeLoteMunicipios(t, zip)
	f := &source.Fonte{BaseURL: srv.URL, Hash: "TESTE123", HTTP: srv.Client()}

	dir := t.TempDir()
	if err := Executar(ctx, pool, Opcoes{Referencia: "2026-09", Diretorio: dir, Fonte: f}); err != nil {
		t.Fatalf("Executar: %v", err)
	}

	schemaNovo, err := store.NomeDoSchema("2026-09")
	if err != nil {
		t.Fatalf("NomeDoSchema novo: %v", err)
	}

	vig, err := store.LoteVigente(ctx, pool)
	if err != nil {
		t.Fatalf("LoteVigente: %v", err)
	}
	if vig == nil || vig.Schema != schemaNovo {
		t.Fatalf("vigente = %v; quer o schema novo %s", vig, schemaNovo)
	}
	if vig.Linhas != 5 {
		t.Errorf("linhas = %d; quer 5 (o conteudo de zipDeMunicipios)", vig.Linhas)
	}

	// O schema antigo foi descartado apos a troca.
	var existeAntigo bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)`,
		schemaAntigo).Scan(&existeAntigo)
	if err != nil {
		t.Fatalf("checar schema antigo: %v", err)
	}
	if existeAntigo {
		t.Errorf("schema antigo %s ainda existe; devia ter sido descartado apos a troca", schemaAntigo)
	}
}

// Os arquivos vem em dez partes (Empresas0..9, Estabelecimentos0..9). O
// despacho precisa reconhecer todas, nao so a primeira.
func TestCarregarReconheceTodasAsPartes(t *testing.T) {
	nomes := []string{
		"Municipios.zip",
		"Simples.zip",
		"Empresas0.zip", "Empresas5.zip", "Empresas9.zip",
		"Estabelecimentos0.zip", "Estabelecimentos7.zip", "Estabelecimentos9.zip",
	}
	for _, n := range nomes {
		if !suportado(n) {
			t.Errorf("%s devia ser suportado", n)
		}
	}

	// Auxiliares que traduzem codigo em texto.
	for _, n := range []string{"Cnaes.zip", "Naturezas.zip"} {
		if !suportado(n) {
			t.Errorf("%s devia ser suportado", n)
		}
	}

	// Socios fica de fora por decisao (docs/docs/arquitetura.md); os demais
	// auxiliares nao tem quem use o codigo.
	for _, n := range []string{"Socios0.zip", "Motivos.zip", "Qualificacoes.zip", "Paises.zip"} {
		if suportado(n) {
			t.Errorf("%s nao devia ser suportado", n)
		}
	}

	// Nao basta o comprimento bater: o caractere da parte precisa ser digito.
	// A Receita publica nomes exatos — aceitar variantes de caixa ou letra no
	// lugar do digito so esconderia uma mudanca real no portal.
	for _, n := range []string{
		"EmpresasX.zip", "EstabelecimentosZ.zip",
		"Empresas.zip", "Empresas10.zip",
		"Empresas0.ZIP", "empresas0.zip",
	} {
		if suportado(n) {
			t.Errorf("%s nao devia ser suportado", n)
		}
	}
}

// Estabelecimentos e de longe o maior arquivo do lote (5,1 GB comprimidos,
// ~63 milhoes de linhas): se a carga falhar no meio, falhar depois que o
// resto ja foi carregado deixa menos trabalho para a retomada. Por isso a
// ordem de carga tem que colocar Estabelecimentos por ultimo, nao importa a
// ordem em que a listagem WebDAV devolveu os arquivos.
func TestOrdenarPorPrioridadeDeCargaPoeEstabelecimentosPorUltimo(t *testing.T) {
	entrada := []source.Arquivo{
		{Nome: "Estabelecimentos3.zip"},
		{Nome: "Estabelecimentos0.zip"},
		{Nome: "Empresas1.zip"},
		{Nome: "Simples.zip"},
		{Nome: "Municipios.zip"},
		{Nome: "Empresas0.zip"},
	}
	ordenarPorPrioridadeDeCarga(entrada)

	var nomes []string
	for _, a := range entrada {
		nomes = append(nomes, a.Nome)
	}

	if nomes[0] != "Municipios.zip" {
		t.Errorf("primeiro = %s; queria Municipios.zip", nomes[0])
	}
	if nomes[1] != "Simples.zip" {
		t.Errorf("segundo = %s; queria Simples.zip", nomes[1])
	}
	ultimos := nomes[len(nomes)-2:]
	for _, n := range ultimos {
		if !strings.HasPrefix(n, "Estabelecimentos") {
			t.Errorf("os dois ultimos deviam ser Estabelecimentos*, veio %v", ultimos)
		}
	}

	// Nenhum Empresas* pode vir depois de um Estabelecimentos*.
	primeiraEstab := -1
	for i, n := range nomes {
		if strings.HasPrefix(n, "Estabelecimentos") {
			primeiraEstab = i
			break
		}
	}
	for i, n := range nomes {
		if strings.HasPrefix(n, "Empresas") && i > primeiraEstab {
			t.Errorf("%s (indice %d) veio depois do primeiro Estabelecimentos (indice %d)", n, i, primeiraEstab)
		}
	}
}

// TestCarregarDespachaParaOCarregadorCerto prova que cada nome de arquivo
// cai no carregador certo. Sem este teste, trocar CarregarEmpresas por
// CarregarSimples (por exemplo) no switch de carregar() nao quebraria nada:
// TestCarregarEmpresas chama CarregarEmpresas diretamente, nunca passa pelo
// switch, entao um bug de despacho passaria em silencio — os dados de
// Empresas entrariam pelo conversor de Simples, com colunas erradas.
//
// Cada fixture usada aqui (zipDeMunicipios, zipDeSimples, zipDeEmpresas,
// zipDeEstabelecimentos) so e valida para o layout da SUA propria tabela:
// os outros tres conversores rejeitam essas linhas (poucos campos demais,
// ou o campo obrigatorio no lugar errado) e carregam zero linhas. Isso e o
// que torna o teste capaz de distinguir um despacho trocado: se
// "Empresas0.zip" fosse mandado para CarregarSimples, o conversor de Simples
// exige 5+ campos (o de Empresas tem 7, entao passaria o corte de tamanho)
// mas gravaria os campos nas colunas de Simples (opcao_simples, opcao_mei) —
// por isso a asserção verifica tanto "a tabela certa tem linhas" quanto "as
// outras tres continuam vazias".
func TestCarregarDespachaParaOCarregadorCerto(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, err := store.NomeDoSchema("2026-09")
	if err != nil {
		t.Fatalf("NomeDoSchema: %v", err)
	}
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}

	tabelas := []string{"municipio", "simples", "empresa", "estabelecimento"}
	casos := []struct {
		arquivo string
		caminho string
		tabela  string
	}{
		{"Municipios.zip", zipDeMunicipios(t), "municipio"},
		{"Simples.zip", zipDeSimples(t), "simples"},
		{"Empresas0.zip", zipDeEmpresas(t), "empresa"},
		{"Estabelecimentos0.zip", zipDeEstabelecimentos(t), "estabelecimento"},
	}

	for _, c := range casos {
		n, err := carregar(ctx, pool, schema, c.arquivo, c.caminho, nil)
		if err != nil {
			t.Fatalf("carregar(%s): %v", c.arquivo, err)
		}
		if n == 0 {
			t.Fatalf("carregar(%s) carregou 0 linhas; a fixture tem linhas validas para %s",
				c.arquivo, c.tabela)
		}

		for _, tab := range tabelas {
			var contagem int
			q := "SELECT count(*) FROM " + schema + "." + tab
			if err := pool.QueryRow(ctx, q).Scan(&contagem); err != nil {
				t.Fatalf("contar %s: %v", tab, err)
			}
			if tab == c.tabela {
				if contagem == 0 {
					t.Errorf("%s: tabela %s ficou vazia; devia ter recebido as linhas", c.arquivo, tab)
				}
			} else {
				if contagem != 0 {
					t.Errorf("%s: tabela %s tem %d linha(s); devia estar vazia (despacho foi para o lugar errado)",
						c.arquivo, tab, contagem)
				}
			}
		}

		// Limpa antes do proximo caso, para que a checagem de "as outras
		// tabelas ficam vazias" nao acuse sobra do caso anterior.
		for _, tab := range tabelas {
			if _, err := pool.Exec(ctx, "DELETE FROM "+schema+"."+tab); err != nil {
				t.Fatalf("limpar %s: %v", tab, err)
			}
		}
	}
}

// xmlDeLoteComArquivoNaoSuportado lista um lote com um arquivo que a ingestao
// nao carrega (Socios0.zip fica de fora por decisao, ver docs/docs/arquitetura.md). Isso forca carregar() a
// falhar DEPOIS que o schema novo ja foi criado mas ANTES de qualquer troca
// ou descarte — o ponto exato onde uma falha no meio do caminho tem que
// deixar o lote antigo intocado e vigente.
const xmlDeLoteComArquivoNaoSuportado = `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/public.php/webdav/Dados/Cadastros/CNPJ/2026-09/</d:href>
    <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop>
    <d:status>HTTP/1.1 200 OK</d:status></d:propstat>
  </d:response>
  <d:response>
    <d:href>/public.php/webdav/Dados/Cadastros/CNPJ/2026-09/Socios0.zip</d:href>
    <d:propstat><d:prop>
      <d:getcontentlength>10</d:getcontentlength>
      <d:getlastmodified>Sun, 13 Sep 2026 16:02:02 GMT</d:getlastmodified>
      <d:resourcetype/>
    </d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>
  </d:response>
</d:multistatus>`

// TestExecutarFalhaNaTrocaMantemLoteAntigoIntacto e a peca que faltava para
// fechar a ordem exigida pelo contrato: descartar o lote antigo so DEPOIS que
// TrocarLoteVigente tiver sucesso. Para provar isso sem sorte de timing,
// corrompe deliberadamente meta.lote (remove uma coluna que o INSERT usa) so
// DEPOIS de semear o lote antigo — a carga inteira roda normalmente (arquivo
// suportado, ZIP valido, indices criados), e so a troca em si falha. Se o
// codigo (por engano futuro) descartasse o lote antigo antes de tentar a
// troca, este teste pegaria: o lote antigo teria sumido apesar da falha.
func TestExecutarFalhaNaTrocaMantemLoteAntigoIntacto(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	schemaAntigo, err := store.NomeDoSchema("2026-08")
	if err != nil {
		t.Fatalf("NomeDoSchema antigo: %v", err)
	}
	if err := store.CriarSchemaDeLote(ctx, pool, schemaAntigo); err != nil {
		t.Fatalf("criar schema antigo: %v", err)
	}
	if err := store.TrocarLoteVigente(ctx, pool, schemaAntigo, "2026-08", 100); err != nil {
		t.Fatalf("trocar para o antigo: %v", err)
	}

	// Trigger que faz APENAS a escrita (INSERT/UPDATE) em meta.lote falhar
	// para o schema novo, sem afetar leituras (LoteVigente so faz SELECT).
	// Assim a carga inteira roda normal e so TrocarLoteVigente falha.
	_, err = pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION teste_bloquear_troca() RETURNS trigger AS $$
		BEGIN
			IF NEW.schema = 'lote_2026_09' THEN
				RAISE EXCEPTION 'bloqueado pelo teste: troca deve falhar';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER bloquear_troca
			BEFORE INSERT OR UPDATE ON meta.lote
			FOR EACH ROW EXECUTE FUNCTION teste_bloquear_troca();
	`)
	if err != nil {
		t.Fatalf("instalar trigger de bloqueio: %v", err)
	}

	zip := lerArquivo(t, zipDeMunicipios(t))
	srv := servidorDeLoteMunicipios(t, zip)
	f := &source.Fonte{BaseURL: srv.URL, Hash: "TESTE123", HTTP: srv.Client()}

	dir := t.TempDir()
	err = Executar(ctx, pool, Opcoes{Referencia: "2026-09", Diretorio: dir, Fonte: f})
	if err == nil {
		t.Fatal("Executar devia falhar: o trigger de teste bloqueia a troca para lote_2026_09")
	}

	// O schema antigo tem que continuar existindo: se o codigo tivesse
	// descartado ANTES de tentar a troca, esta consulta acusaria.
	var existeAntigo bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)`,
		schemaAntigo).Scan(&existeAntigo)
	if err != nil {
		t.Fatalf("checar schema antigo: %v", err)
	}
	if !existeAntigo {
		t.Fatalf("schema antigo %s foi descartado apesar da troca ter falhado; "+
			"a ordem exigida (descartar so DEPOIS da troca) foi violada", schemaAntigo)
	}
}

// TestExecutarFalhaNoMeioMantemLoteAntigoVigente e a garantia blue/green: se
// a carga falhar depois do schema novo criado (aqui, por topar com um
// arquivo que o A2 ainda nao sabe carregar), o lote antigo continua vigente,
// intacto, e o schema novo fica abandonado — nunca ha um instante em que
// TrocarLoteVigente ou o descarte do lote antigo rodam antes da falha.
func TestExecutarFalhaNoMeioMantemLoteAntigoVigente(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	schemaAntigo, err := store.NomeDoSchema("2026-08")
	if err != nil {
		t.Fatalf("NomeDoSchema antigo: %v", err)
	}
	if err := store.CriarSchemaDeLote(ctx, pool, schemaAntigo); err != nil {
		t.Fatalf("criar schema antigo: %v", err)
	}
	if err := store.TrocarLoteVigente(ctx, pool, schemaAntigo, "2026-08", 100); err != nil {
		t.Fatalf("trocar para o antigo: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			w.Write([]byte(xmlDeLoteComArquivoNaoSuportado))
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("0123456789")) // conteudo irrelevante: nunca sera carregado
		default:
			t.Errorf("metodo inesperado: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)
	f := &source.Fonte{BaseURL: srv.URL, Hash: "TESTE123", HTTP: srv.Client()}

	dir := t.TempDir()
	err = Executar(ctx, pool, Opcoes{
		Referencia: "2026-09",
		Diretorio:  dir,
		Fonte:      f,
		Apenas:     []string{"Socios0.zip"},
	})
	if err == nil {
		t.Fatal("Executar devia falhar: Socios0.zip nao e suportado")
	}
	if !strings.Contains(err.Error(), "nao suportado") {
		t.Errorf("erro = %v; queria a mensagem de 'nao suportado'", err)
	}

	// O lote antigo continua vigente, com as linhas originais intocadas.
	vig, err := store.LoteVigente(ctx, pool)
	if err != nil {
		t.Fatalf("LoteVigente: %v", err)
	}
	if vig == nil || vig.Schema != schemaAntigo || vig.Linhas != 100 {
		t.Fatalf("vigente = %v; a falha no meio do caminho nao devia ter alterado o lote antigo", vig)
	}

	// E o schema antigo nao foi descartado por engano.
	var existeAntigo bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)`,
		schemaAntigo).Scan(&existeAntigo)
	if err != nil {
		t.Fatalf("checar schema antigo: %v", err)
	}
	if !existeAntigo {
		t.Errorf("schema antigo %s foi descartado apesar da falha; a garantia blue/green foi violada", schemaAntigo)
	}
}
