package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A pesquisa traduz CNAE e natureza juridica pelas tabelas auxiliares do lote.
func TestPesquisaTrazDescricoesDasAuxiliares(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	_, chave := criarConta(t, pool, "Conta Teste")
	h := Novo(pool, opcoesDeTeste())

	w := pesquisar(t, h, chave, map[string]any{
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
	var corpo RespostaPesquisa
	if err := json.Unmarshal(w.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if len(corpo.Resultados) != 2 {
		t.Fatalf("%d resultados, esperava 2 (ancora do docs/arquitetura.md)", len(corpo.Resultados))
	}
	for _, l := range corpo.Resultados {
		if l.CNAEDescricao != "Fabricação de esquadrias de metal" {
			t.Errorf("%s: cnae_descricao = %q", l.CNPJ, l.CNAEDescricao)
		}
		if l.Natureza != "Sociedade Empresária Limitada" {
			t.Errorf("%s: natureza_juridica = %q", l.CNPJ, l.Natureza)
		}
	}
}

// CNAE que a auxiliar nao descreve nao pode sumir com a linha: o join e LEFT.
// No seed, 1091102 (IOTA) nao tem descricao de proposito.
func TestPesquisaSemDescricaoNaoPerdeALinha(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	_, chave := criarConta(t, pool, "Conta Teste")
	h := Novo(pool, opcoesDeTeste())

	w := pesquisar(t, h, chave, map[string]any{
		"codigo_atividade_principal": []string{"1091102"},
		"situacao_cadastral":         "ATIVA",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("codigo %d: %s", w.Code, w.Body.String())
	}
	var corpo RespostaPesquisa
	if err := json.Unmarshal(w.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if len(corpo.Resultados) != 1 {
		t.Fatalf("%d resultados, esperava 1: o LEFT JOIN virou INNER?", len(corpo.Resultados))
	}
	if d := corpo.Resultados[0].CNAEDescricao; d != "" {
		t.Errorf("cnae_descricao = %q, esperava vazio", d)
	}
}

func TestDetalheTrazDescricoesDasAuxiliares(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	_, chave := criarConta(t, pool, "Conta Teste")
	h := Novo(pool, opcoesDeTeste())

	req := httptest.NewRequest(http.MethodGet, "/v1/cnpj/10000001000101", nil)
	req.Header.Set("api-key", chave)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("codigo %d: %s", w.Code, w.Body.String())
	}
	var d Detalhe
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if d.CNAEDescricao != "Fabricação de esquadrias de metal" {
		t.Errorf("cnae_descricao = %q", d.CNAEDescricao)
	}
	if d.Natureza != "Sociedade Empresária Limitada" {
		t.Errorf("natureza_juridica = %q", d.Natureza)
	}
}
