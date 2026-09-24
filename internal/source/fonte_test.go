package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// servidorComRedirect imita a raiz da Receita: responde 302 para /index.php/s/<hash>.
func servidorComRedirect(t *testing.T, hash string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/index.php/s/"+hash, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/index.php/s/"+hash, http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestNovaFonteDescobreOHash(t *testing.T) {
	srv := servidorComRedirect(t, "gn672Ad4CF8N6TK")

	f, err := novaFonteEm(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("descobrir hash: %v", err)
	}
	if f.Hash != "gn672Ad4CF8N6TK" {
		t.Errorf("Hash = %q; quer %q", f.Hash, "gn672Ad4CF8N6TK")
	}
}

func TestNovaFonteFalhaSemRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // 200 direto, sem redirect: nao ha hash
	}))
	t.Cleanup(srv.Close)

	_, err := novaFonteEm(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatal("devia falhar quando a raiz nao redireciona")
	}
	if !strings.Contains(err.Error(), "hash") {
		t.Errorf("erro = %v; devia mencionar o hash para o operador entender", err)
	}
}

func TestURLDoLote(t *testing.T) {
	f := &Fonte{BaseURL: "https://exemplo.test", Hash: "ABC123"}

	got, err := f.URLDoLote("2026-09")
	if err != nil {
		t.Fatalf("URLDoLote: %v", err)
	}
	quer := "https://exemplo.test/public.php/webdav/Dados/Cadastros/CNPJ/2026-09/"
	if got != quer {
		t.Errorf("URLDoLote = %q; quer %q", got, quer)
	}
}

func TestURLDoLoteRecusaReferenciaInvalida(t *testing.T) {
	f := &Fonte{BaseURL: "https://exemplo.test", Hash: "ABC123"}

	for _, ruim := range []string{"2026-9", "26-09", "", "2026-09/../../etc"} {
		if _, err := f.URLDoLote(ruim); err == nil {
			t.Errorf("URLDoLote(%q) devia falhar", ruim)
		}
	}
}

func TestURLDoArquivo(t *testing.T) {
	f := &Fonte{BaseURL: "https://exemplo.test", Hash: "ABC123"}

	got, err := f.URLDoArquivo("2026-09", "Municipios.zip")
	if err != nil {
		t.Fatalf("URLDoArquivo: %v", err)
	}
	quer := "https://exemplo.test/public.php/webdav/Dados/Cadastros/CNPJ/2026-09/Municipios.zip"
	if got != quer {
		t.Errorf("URLDoArquivo = %q; quer %q", got, quer)
	}
}

func TestURLDoArquivoRecusaNomeComCaminho(t *testing.T) {
	f := &Fonte{BaseURL: "https://exemplo.test", Hash: "ABC123"}

	for _, ruim := range []string{"../segredo", "a/b.zip", "", "."} {
		if _, err := f.URLDoArquivo("2026-09", ruim); err == nil {
			t.Errorf("URLDoArquivo(%q) devia falhar", ruim)
		}
	}
}

// O cliente da descoberta NAO pode virar o cliente do download. Ele tem
// timeout de 30s, adequado a uma requisicao curta; um download de 293 MB a
// 5 MB/s leva mais que isso e morreria com "context deadline exceeded".
// Foi exatamente o que aconteceu na primeira carga real do Simples.zip —
// nenhum teste pegou porque nenhum httptest serve centenas de megabytes.
func TestNovaFonteNaoVazaClienteComTimeout(t *testing.T) {
	srv := servidorComRedirect(t, "gn672Ad4CF8N6TK")

	// Producao: o chamador nao passa cliente.
	f, err := novaFonteEm(context.Background(), nil, srv.URL)
	if err != nil {
		t.Fatalf("novaFonteEm: %v", err)
	}
	if f.HTTP != nil {
		t.Errorf("HTTP = %v; quer nil quando o chamador nao passou cliente — "+
			"senao o Baixar herda o timeout de 30s da descoberta", f.HTTP)
	}

	// Teste: o chamador passa o seu, e a escolha dele e respeitada.
	meu := srv.Client()
	f2, err := novaFonteEm(context.Background(), meu, srv.URL)
	if err != nil {
		t.Fatalf("novaFonteEm com cliente: %v", err)
	}
	if f2.HTTP != meu {
		t.Errorf("HTTP = %v; quer o cliente que o chamador passou", f2.HTTP)
	}
}
