package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestCursorIdaEVolta(t *testing.T) {
	data := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	c := codificarCursor(&data, "12345678000199")

	voltaData, voltaCNPJ, err := decodificarCursor(c)
	if err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if voltaCNPJ != "12345678000199" {
		t.Errorf("cnpj %q", voltaCNPJ)
	}
	if voltaData == nil || !voltaData.Equal(data) {
		t.Errorf("data %v, esperava %v", voltaData, data)
	}
}

func TestCursorMalformadoDaErro(t *testing.T) {
	for _, ruim := range []string{"nao-e-base64!!", "", "YWJj"} {
		if _, _, err := decodificarCursor(ruim); err == nil {
			t.Errorf("cursor %q foi aceito", ruim)
		}
	}
}

// Paginar com limite 1 precisa percorrer os dois resultados do seed sem
// repetir nem pular — e sem OFFSET.
func TestPesquisaPaginaComCursor(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	_, chave := criarConta(t, pool, "Conta Teste")
	h := Novo(pool, opcoesDeTeste())

	filtro := map[string]any{
		"codigo_atividade_principal": []string{"2512800"},
		"situacao_cadastral":         "ATIVA",
		"uf":                         []string{"SP"},
		"data_abertura":              map[string]any{"ultimos_dias": diasDesdeInicioDoAlvo(t)},
		"mei":                        map[string]any{"excluir_optante": true},
		"mais_filtros":               map[string]any{"somente_celular": true},
		"limite":                     1,
	}

	w := pesquisar(t, h, chave, filtro)
	if w.Code != http.StatusOK {
		t.Fatalf("primeira pagina: %d: %s", w.Code, w.Body.String())
	}
	var pagina1 RespostaPesquisa
	if err := json.Unmarshal(w.Body.Bytes(), &pagina1); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if len(pagina1.Resultados) != 1 {
		t.Fatalf("pagina 1 trouxe %d, esperava 1", len(pagina1.Resultados))
	}
	if pagina1.ProximoCursor == "" {
		t.Fatal("pagina 1 sem proximo_cursor, mas ha mais resultados")
	}

	filtro["cursor"] = pagina1.ProximoCursor
	w = pesquisar(t, h, chave, filtro)
	if w.Code != http.StatusOK {
		t.Fatalf("segunda pagina: %d: %s", w.Code, w.Body.String())
	}
	var pagina2 RespostaPesquisa
	if err := json.Unmarshal(w.Body.Bytes(), &pagina2); err != nil {
		t.Fatalf("decodificar pagina 2: %v", err)
	}
	if len(pagina2.Resultados) != 1 {
		t.Fatalf("pagina 2 trouxe %d, esperava 1", len(pagina2.Resultados))
	}
	if pagina2.Resultados[0].CNPJ == pagina1.Resultados[0].CNPJ {
		t.Error("pagina 2 repetiu o resultado da pagina 1")
	}

	filtro["cursor"] = pagina2.ProximoCursor
	w = pesquisar(t, h, chave, filtro)
	if w.Code != http.StatusOK {
		t.Fatalf("terceira pagina: %d: %s", w.Code, w.Body.String())
	}
	var pagina3 RespostaPesquisa
	if err := json.Unmarshal(w.Body.Bytes(), &pagina3); err != nil {
		t.Fatalf("decodificar pagina 3: %v", err)
	}
	if len(pagina3.Resultados) != 0 {
		t.Errorf("pagina 3 trouxe %d, esperava 0 (fim)", len(pagina3.Resultados))
	}
	if pagina3.ProximoCursor != "" {
		t.Errorf("pagina 3 (ultima) veio com proximo_cursor %q, esperava vazio", pagina3.ProximoCursor)
	}
}

// Uma pagina parcial (menos resultados que o limite pedido, mas maior que
// zero) e o unico jeito de pegar um "sempre devolve proximo_cursor" que uma
// pagina vazia (0 resultados) nao expoe: aqui o limite (5) e maior que o
// total de linhas-alvo do seed (2), entao a unica pagina ja vem incompleta e
// nao pode trazer proximo_cursor.
func TestPesquisaPaginaParcialNaoTemProximoCursor(t *testing.T) {
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
		"limite":                     5,
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
	if corpo.ProximoCursor != "" {
		t.Errorf("pagina parcial (2 de limite 5) veio com proximo_cursor %q, esperava vazio", corpo.ProximoCursor)
	}
}

func TestCursorInvalidoNaRequisicaoDa400(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	_, chave := criarConta(t, pool, "Conta Teste")
	h := Novo(pool, opcoesDeTeste())

	w := pesquisar(t, h, chave, map[string]any{
		"codigo_atividade_principal": []string{"2512800"},
		"cursor":                     "lixo-nao-decodifica",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("codigo %d, esperava 400", w.Code)
	}
}
