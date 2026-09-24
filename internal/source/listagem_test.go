package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// xmlDeLote imita a resposta 207 do Nextcloud para uma pasta de lote.
const xmlDeLote = `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/public.php/webdav/Dados/Cadastros/CNPJ/2026-09/</d:href>
    <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop>
    <d:status>HTTP/1.1 200 OK</d:status></d:propstat>
  </d:response>
  <d:response>
    <d:href>/public.php/webdav/Dados/Cadastros/CNPJ/2026-09/Municipios.zip</d:href>
    <d:propstat><d:prop>
      <d:getcontentlength>28672</d:getcontentlength>
      <d:getlastmodified>Sun, 13 Sep 2026 16:02:02 GMT</d:getlastmodified>
      <d:resourcetype/>
    </d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>
  </d:response>
  <d:response>
    <d:href>/public.php/webdav/Dados/Cadastros/CNPJ/2026-09/Estabelecimentos0.zip</d:href>
    <d:propstat><d:prop>
      <d:getcontentlength>2243366912</d:getcontentlength>
      <d:getlastmodified>Mon, 14 Sep 2026 15:02:59 GMT</d:getlastmodified>
      <d:resourcetype/>
    </d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>
  </d:response>
</d:multistatus>`

const xmlDePastas = `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:">
  <d:response><d:href>/public.php/webdav/Dados/Cadastros/CNPJ/</d:href>
    <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
  <d:response><d:href>/public.php/webdav/Dados/Cadastros/CNPJ/2026-08/</d:href>
    <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
  <d:response><d:href>/public.php/webdav/Dados/Cadastros/CNPJ/2026-09/</d:href>
    <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
  <d:response><d:href>/public.php/webdav/Dados/Cadastros/CNPJ/cnpj.tar.gz</d:href>
    <d:propstat><d:prop><d:resourcetype/></d:prop></d:propstat></d:response>
</d:multistatus>`

// xmlMultiPropstat imita o Nextcloud real: um bloco 404 para as propriedades
// que o servidor nao soube fornecer, e um 200 com as que soube. O 404 vem
// primeiro de proposito — se propriedades() pegasse o primeiro bloco, o
// tamanho viria zerado e a verificacao de download seria pulada em silencio.
const xmlMultiPropstat = `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/public.php/webdav/Dados/Cadastros/CNPJ/2026-09/Estabelecimentos0.zip</d:href>
    <d:propstat>
      <d:prop><d:getcontenttype/><d:quota-used-bytes/></d:prop>
      <d:status>HTTP/1.1 404 Not Found</d:status>
    </d:propstat>
    <d:propstat>
      <d:prop>
        <d:getcontentlength>2243366912</d:getcontentlength>
        <d:getlastmodified>Mon, 14 Sep 2026 15:02:59 GMT</d:getlastmodified>
        <d:resourcetype/>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`

func servidorWebDAV(t *testing.T, corpo string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" {
			t.Errorf("metodo = %s; quer PROPFIND", r.Method)
		}
		if r.Header.Get("Depth") != "1" {
			t.Errorf("Depth = %q; quer \"1\"", r.Header.Get("Depth"))
		}
		usuario, senha, ok := r.BasicAuth()
		if !ok || usuario == "" || senha != "" {
			t.Errorf("auth = (%q, %q, %v); quer hash como usuario e senha vazia", usuario, senha, ok)
		}
		w.WriteHeader(http.StatusMultiStatus)
		w.Write([]byte(corpo))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestListarLote(t *testing.T) {
	srv := servidorWebDAV(t, xmlDeLote)
	f := &Fonte{BaseURL: srv.URL, Hash: "ABC123", HTTP: srv.Client()}

	arquivos, err := f.ListarLote(context.Background(), "2026-09")
	if err != nil {
		t.Fatalf("ListarLote: %v", err)
	}
	// A pasta em si nao entra: so arquivos.
	if len(arquivos) != 2 {
		t.Fatalf("len = %d; quer 2 (a pasta nao conta)", len(arquivos))
	}

	porNome := map[string]Arquivo{}
	for _, a := range arquivos {
		porNome[a.Nome] = a
	}

	m, ok := porNome["Municipios.zip"]
	if !ok {
		t.Fatal("Municipios.zip nao foi listado")
	}
	if m.Tamanho != 28672 {
		t.Errorf("Tamanho = %d; quer 28672", m.Tamanho)
	}
	if m.Modificado.IsZero() {
		t.Error("Modificado ficou zerado; a data do WebDAV nao foi parseada")
	}

	e := porNome["Estabelecimentos0.zip"]
	if e.Tamanho != 2243366912 {
		t.Errorf("Tamanho = %d; quer 2243366912 (int64, nao int32)", e.Tamanho)
	}
}

func TestListarLotes(t *testing.T) {
	srv := servidorWebDAV(t, xmlDePastas)
	f := &Fonte{BaseURL: srv.URL, Hash: "ABC123", HTTP: srv.Client()}

	lotes, err := f.ListarLotes(context.Background())
	if err != nil {
		t.Fatalf("ListarLotes: %v", err)
	}
	// So pastas AAAA-MM; cnpj.tar.gz e a propria pasta CNPJ/ ficam de fora.
	if len(lotes) != 2 {
		t.Fatalf("lotes = %v; quer 2 entradas", lotes)
	}
	if lotes[len(lotes)-1] != "2026-09" {
		t.Errorf("ultimo = %q; quer \"2026-09\" (mais recente por ultimo)", lotes[len(lotes)-1])
	}
}

func TestListarLoteComMultiplosPropstat(t *testing.T) {
	srv := servidorWebDAV(t, xmlMultiPropstat)
	f := &Fonte{BaseURL: srv.URL, Hash: "ABC123", HTTP: srv.Client()}

	arquivos, err := f.ListarLote(context.Background(), "2026-09")
	if err != nil {
		t.Fatalf("ListarLote: %v", err)
	}
	if len(arquivos) != 1 {
		t.Fatalf("len = %d; quer 1", len(arquivos))
	}
	a := arquivos[0]
	if a.Tamanho != 2243366912 {
		t.Errorf("Tamanho = %d; quer 2243366912 — o bloco 404 veio primeiro e "+
			"nao pode ser o escolhido", a.Tamanho)
	}
	if a.Modificado.IsZero() {
		t.Error("Modificado zerado; a data estava no bloco 200 e devia ter sido lida")
	}
}

func TestListarLoteErroHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	f := &Fonte{BaseURL: srv.URL, Hash: "ABC123", HTTP: srv.Client()}

	if _, err := f.ListarLote(context.Background(), "2099-01"); err == nil {
		t.Fatal("devia falhar com 404")
	}
}
