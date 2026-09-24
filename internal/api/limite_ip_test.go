package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// relogioFixo devolve um relogio controlado pelo teste: avancar(d) move o
// tempo sem esperar de verdade.
func relogioFixo() (agora func() time.Time, avancar func(time.Duration)) {
	t := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	return func() time.Time { return t }, func(d time.Duration) { t = t.Add(d) }
}

func chamarUso(h http.Handler, remoto, chave, xff string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/uso", nil)
	req.RemoteAddr = remoto
	if chave != "" {
		req.Header.Set("api-key", chave)
	}
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// O limite por IP e "sem auth": cliente autenticado so responde ao limite da
// conta. Atras do Caddy todo mundo tem o mesmo IP, entao gastar a cota do IP
// em requisicao autenticada estrangularia a API inteira em PorIP/min.
func TestAutenticadoNaoGastaCotaDoIP(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	_, chave := criarConta(t, pool, "Conta Teste")

	opts := opcoesDeTeste()
	opts.PorIP = 2
	opts.PorMinuto = 20
	opts.relogio, _ = relogioFixo()
	h := Novo(pool, opts)

	for i := 1; i <= 20; i++ {
		if w := chamarUso(h, "192.0.2.1:1234", chave, ""); w.Code != http.StatusOK {
			t.Fatalf("requisicao autenticada %d: codigo %d, esperava 200 (limite da conta e 20)", i, w.Code)
		}
	}
	if w := chamarUso(h, "192.0.2.1:1234", chave, ""); w.Code != http.StatusTooManyRequests {
		t.Errorf("requisicao 21: codigo %d, esperava 429 do limite da conta", w.Code)
	}
}

// Forca bruta: cada 401 (chave ausente ou invalida) cobra do IP; passado o
// teto, 429 com Retry-After.
func TestFalhaDeAutenticacaoCobraDoIP(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)

	opts := opcoesDeTeste()
	opts.PorIP = 3
	opts.relogio, _ = relogioFixo()
	h := Novo(pool, opts)

	chaves := []string{"fnt_live_errada1", "", "fnt_live_errada2"}
	for i, c := range chaves {
		if w := chamarUso(h, "192.0.2.1:1234", c, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("tentativa %d: codigo %d, esperava 401", i+1, w.Code)
		}
	}
	w := chamarUso(h, "192.0.2.1:1234", "fnt_live_errada3", "")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("tentativa 4: codigo %d, esperava 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("429 por IP sem Retry-After")
	}
	// Outro IP nao herda o bloqueio.
	if w := chamarUso(h, "192.0.2.2:1234", "fnt_live_errada", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("IP diferente: codigo %d, esperava 401", w.Code)
	}
}

// Decisao documentada em autenticado: IP bloqueado e barrado ANTES de
// autenticar, entao ate chave valida vinda dele recebe 429 ate a janela
// passar. E o que impede o atacante bloqueado de continuar gerando consulta
// ao banco; o custo (quem divide IP com o atacante espera ate 1 minuto) e
// aceito.
func TestIPBloqueadoBarraAteChaveValidaAteAJanelaPassar(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	_, chave := criarConta(t, pool, "Conta Teste")

	opts := opcoesDeTeste()
	opts.PorIP = 2
	var avancar func(time.Duration)
	opts.relogio, avancar = relogioFixo()
	h := Novo(pool, opts)

	chamarUso(h, "192.0.2.1:1234", "fnt_live_errada", "")
	chamarUso(h, "192.0.2.1:1234", "fnt_live_errada", "")

	if w := chamarUso(h, "192.0.2.1:1234", chave, ""); w.Code != http.StatusTooManyRequests {
		t.Fatalf("chave valida de IP bloqueado: codigo %d, esperava 429", w.Code)
	}
	avancar(61 * time.Second)
	if w := chamarUso(h, "192.0.2.1:1234", chave, ""); w.Code != http.StatusOK {
		t.Errorf("janela passou e a chave valida segue barrada: codigo %d", w.Code)
	}
}

// X-Forwarded-For so vale quando a conexao vem do proxy configurado. De
// qualquer outro par, o cabecalho e ignorado — senao cada tentativa de forca
// bruta mandaria um IP forjado diferente e nunca seria contada.
func TestXFFSoValeDoProxyConfiavel(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)

	opts := opcoesDeTeste()
	opts.PorIP = 1
	opts.ProxyConfiavel = "10.0.0.1"
	opts.relogio, _ = relogioFixo()
	h := Novo(pool, opts)

	// Pelo proxy: cada cliente real (entrada mais a direita) tem a propria cota.
	if w := chamarUso(h, "10.0.0.1:5000", "fnt_live_x", "203.0.113.9, 198.51.100.1"); w.Code != http.StatusUnauthorized {
		t.Fatalf("cliente A pelo proxy: %d, esperava 401", w.Code)
	}
	if w := chamarUso(h, "10.0.0.1:5000", "fnt_live_x", "198.51.100.1"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("cliente A de novo pelo proxy: %d, esperava 429", w.Code)
	}
	if w := chamarUso(h, "10.0.0.1:5000", "fnt_live_x", "198.51.100.2"); w.Code != http.StatusUnauthorized {
		t.Fatalf("cliente B pelo proxy: %d, esperava 401 (cota propria)", w.Code)
	}

	// Fora do proxy: o XFF forjado nao muda nada, o IP e o do par.
	if w := chamarUso(h, "192.0.2.50:4000", "fnt_live_x", "198.51.100.3"); w.Code != http.StatusUnauthorized {
		t.Fatalf("primeira tentativa direta: %d, esperava 401", w.Code)
	}
	if w := chamarUso(h, "192.0.2.50:4000", "fnt_live_x", "198.51.100.4"); w.Code != http.StatusTooManyRequests {
		t.Errorf("XFF forjado de quem nao e o proxy trocou o IP contado: codigo %d, esperava 429", w.Code)
	}
}

func TestIPDoCliente(t *testing.T) {
	casos := []struct {
		nome, proxy, remoto, xff, quer string
	}{
		{"sem proxy usa o par", "", "192.0.2.1:1234", "", "192.0.2.1"},
		{"sem proxy ignora XFF", "", "192.0.2.1:1234", "198.51.100.1", "192.0.2.1"},
		{"par nao e o proxy", "10.0.0.1", "192.0.2.1:1234", "198.51.100.1", "192.0.2.1"},
		{"proxy: entrada mais a direita", "10.0.0.1", "10.0.0.1:80", "1.1.1.1, 198.51.100.7 ", "198.51.100.7"},
		{"proxy: XFF vazio cai no par", "10.0.0.1", "10.0.0.1:80", "", "10.0.0.1"},
		{"proxy: XFF lixo cai no par", "10.0.0.1", "10.0.0.1:80", "nao-e-ip", "10.0.0.1"},
		{"IPv6 agrupa por /64", "", "[2001:db8:1:2::1]:443", "", "2001:db8:1:2::/64"},
		{"IPv6 mesma /64, outro host", "", "[2001:db8:1:2:ffff::9]:443", "", "2001:db8:1:2::/64"},
		{"IPv6 pelo proxy", "::1", "[::1]:80", "2001:db8:1:3::5", "2001:db8:1:3::/64"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = c.remoto
			if c.xff != "" {
				req.Header.Set("X-Forwarded-For", c.xff)
			}
			if got := ipDoCliente(req, c.proxy); got != c.quer {
				t.Errorf("ipDoCliente = %q, quer %q", got, c.quer)
			}
		})
	}
}
