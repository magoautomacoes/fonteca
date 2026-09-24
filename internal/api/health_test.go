package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthNaoExigeChave(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	h := Novo(pool, opcoesDeTeste())

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("codigo %d, esperava 200 sem chave", w.Code)
	}

	var corpo map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	lote, ok := corpo["lote"].(map[string]any)
	if !ok {
		t.Fatalf("campo lote ausente ou do tipo errado: %v", corpo["lote"])
	}
	if lote["referencia"] != "2026-09" {
		t.Errorf("referencia %v, esperava 2026-09", lote["referencia"])
	}
}

// Sem lote importado, /v1/health responde 200 com lote nulo — um monitor
// precisa distinguir processo caido de base ainda nao carregada.
func TestHealthSemLoteResponde200ComLoteNulo(t *testing.T) {
	pool := bancoDeTeste(t) // sem semearLote
	h := Novo(pool, opcoesDeTeste())

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("codigo %d, esperava 200", w.Code)
	}
	var corpo map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if corpo["lote"] != nil {
		t.Errorf("lote = %v, esperava null", corpo["lote"])
	}
}

// /v1/health nao tem chave nem limite: o lote vigente fica em cache, entao
// chamadas repetidas dentro da janela nao voltam ao banco. A prova e o lote
// importado no meio: dentro da janela o health ainda responde o valor antigo,
// passada a janela enxerga o novo.
func TestHealthGuardaLoteEmCache(t *testing.T) {
	pool := bancoDeTeste(t) // comeca sem lote
	opts := opcoesDeTeste()
	var avancar func(time.Duration)
	opts.relogio, avancar = relogioFixo()
	h := Novo(pool, opts)

	lote := func() any {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/health", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("codigo %d, esperava 200", w.Code)
		}
		var corpo map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &corpo); err != nil {
			t.Fatalf("decodificar: %v", err)
		}
		return corpo["lote"]
	}

	if l := lote(); l != nil {
		t.Fatalf("lote = %v antes de importar", l)
	}
	semearLote(t, pool)

	avancar(validadeDoHealth - time.Second)
	if l := lote(); l != nil {
		t.Errorf("dentro da janela o health voltou ao banco: lote = %v", l)
	}

	avancar(2 * time.Second)
	if l := lote(); l == nil {
		t.Error("passada a janela o health nao enxergou o lote importado")
	}
}

func TestRotaDesconhecidaDa404(t *testing.T) {
	pool := bancoDeTeste(t)
	h := Novo(pool, opcoesDeTeste())

	req := httptest.NewRequest(http.MethodGet, "/v1/naoexiste", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("codigo %d, esperava 404", w.Code)
	}
}
