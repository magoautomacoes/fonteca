package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSemChaveDa401(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	h := Novo(pool, opcoesDeTeste())

	req := httptest.NewRequest(http.MethodGet, "/v1/uso", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("codigo %d, esperava 401", w.Code)
	}
}

func TestChaveInvalidaDa401(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	h := Novo(pool, opcoesDeTeste())

	req := httptest.NewRequest(http.MethodGet, "/v1/uso", nil)
	req.Header.Set("api-key", "fnt_live_invalida")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("codigo %d, esperava 401", w.Code)
	}
}

func TestChaveValidaPassa(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	_, chave := criarConta(t, pool, "Conta Teste")
	h := Novo(pool, opcoesDeTeste())

	req := httptest.NewRequest(http.MethodGet, "/v1/uso", nil)
	req.Header.Set("api-key", chave)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("codigo %d, esperava 200", w.Code)
	}
}

func TestRateLimitPorContaDa429ComRetryAfter(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	_, chave := criarConta(t, pool, "Conta Teste")

	opts := opcoesDeTeste()
	opts.PorMinuto = 2
	h := Novo(pool, opts)

	chamar := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/uso", nil)
		req.Header.Set("api-key", chave)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}

	chamar()
	chamar()
	w := chamar()

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("codigo %d, esperava 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("429 sem Retry-After")
	}
}

// Panico num handler nao pode derrubar o processo nem vazar stack ao cliente.
func TestPanicoViraErro500(t *testing.T) {
	pool := bancoDeTeste(t)
	s := &servidor{pool: pool, log: opcoesDeTeste().Log}

	quebrado := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})
	h := s.recuperar(quebrado)

	req := httptest.NewRequest(http.MethodGet, "/qualquer", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req) // nao pode propagar o panico

	if w.Code != http.StatusInternalServerError {
		t.Errorf("codigo %d, esperava 500", w.Code)
	}
	if bodyContem(w.Body.String(), "boom") {
		t.Error("a mensagem do panico vazou na resposta")
	}
}

func bodyContem(corpo, agulha string) bool {
	return len(corpo) > 0 && len(agulha) > 0 &&
		(len(corpo) >= len(agulha)) && (indexOf(corpo, agulha) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// http.ErrAbortHandler e o jeito oficial de abortar a resposta: o net/http
// trata esse panico em silencio. Engolir ele aqui viraria um 500 com log de
// erro para algo que nao e erro.
func TestRecuperarRepassaErrAbortHandler(t *testing.T) {
	s := &servidor{log: opcoesDeTeste().Log}
	h := s.recuperar(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		if p := recover(); p != http.ErrAbortHandler {
			t.Errorf("panico repassado = %v, esperava http.ErrAbortHandler", p)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

// Panico depois do status enviado nao pode virar 500 colado no corpo ja
// escrito: aborta a conexao, e o corpo parcial fica como estava.
func TestPanicoDepoisDaRespostaIniciadaAborta(t *testing.T) {
	s := &servidor{log: opcoesDeTeste().Log}
	h := s.recuperar(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"parcial":`))
		panic("boom")
	}))

	w := httptest.NewRecorder()
	func() {
		defer func() {
			if p := recover(); p != http.ErrAbortHandler {
				t.Errorf("panico = %v, esperava http.ErrAbortHandler", p)
			}
		}()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	}()

	if w.Code != http.StatusOK {
		t.Errorf("codigo %d: o status ja enviado foi reescrito", w.Code)
	}
	if corpo := w.Body.String(); corpo != `{"parcial":` {
		t.Errorf("corpo %q: algo foi colado depois do panico", corpo)
	}
}
