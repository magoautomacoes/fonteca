package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func contar(t *testing.T, h http.Handler, chave string, corpo any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(corpo)
	if err != nil {
		t.Fatalf("serializar: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/cnpj/contagem", bytes.NewReader(b))
	if chave != "" {
		req.Header.Set("api-key", chave)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// A contagem da consulta-alvo tem de bater com a ancora da pesquisa: 2, em SP.
func TestContagemDaConsultaAlvo(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	_, chave := criarConta(t, pool, "Conta Teste")
	h := Novo(pool, opcoesDeTeste())

	w := contar(t, h, chave, map[string]any{
		"codigo_atividade_principal": []string{"2512800"},
		"situacao_cadastral":         "ATIVA",
		"uf":                         []string{"SP"},
		"data_abertura":              map[string]any{"ultimos_dias": diasDesdeInicioDoAlvo(t)},
		"mei":                        map[string]any{"excluir_optante": true},
		"mais_filtros":               map[string]any{"somente_celular": true},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("codigo %d: %s", w.Code, w.Body.String())
	}
	var r RespostaContagem
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if r.Total != 2 || r.PorUF["SP"] != 2 || len(r.PorUF) != 1 {
		t.Errorf("contagem %+v, quer total 2, SP 2", r)
	}
	if r.Lote != "2026-09" {
		t.Errorf("lote %q", r.Lote)
	}
}

func TestContagemFiltroInvalidoDa400(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	_, chave := criarConta(t, pool, "Conta Teste")
	h := Novo(pool, opcoesDeTeste())

	w := contar(t, h, chave, map[string]any{"codigo_atividade_principal": []string{"12"}})
	if w.Code != http.StatusBadRequest {
		t.Errorf("codigo %d, quer 400: %s", w.Code, w.Body.String())
	}
}

func TestContagemSemChaveDa401(t *testing.T) {
	pool := bancoDeTeste(t)
	h := Novo(pool, opcoesDeTeste())
	w := contar(t, h, "", map[string]any{"codigo_atividade_principal": []string{"2512800"}})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("codigo %d, quer 401", w.Code)
	}
}
