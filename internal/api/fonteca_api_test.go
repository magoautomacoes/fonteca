package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/magoautomacoes/fonteca/internal/store"
)

// Os outros testes do pacote conectam como o superusuario do testcontainer,
// que ignora RLS e GRANT. Este roda a API inteira como fonteca_api, com os
// papeis reais de deploy/papeis.sql: pega GRANT faltando (a API responderia
// 500 em producao) e prova que /v1/uso de uma conta nao traz a outra.
func TestAPIComoFontecaAPI(t *testing.T) {
	dono := bancoDeTeste(t)
	ctx := context.Background()

	papeis, err := os.ReadFile("../../deploy/papeis.sql")
	if err != nil {
		t.Fatalf("ler deploy/papeis.sql: %v", err)
	}
	if _, err := dono.Exec(ctx, string(papeis)); err != nil {
		t.Fatalf("aplicar papeis: %v", err)
	}
	if _, err := dono.Exec(ctx, `ALTER ROLE fonteca_api PASSWORD 'senha-de-teste'`); err != nil {
		t.Fatalf("senha de teste: %v", err)
	}

	// Lote criado DEPOIS dos papeis, como faz a ingestao: CriarSchemaDeLote
	// concede o USAGE do schema novo.
	semearLote(t, dono)
	contaA, chaveA := criarConta(t, dono, "Conta A")
	contaB, chaveB := criarConta(t, dono, "Conta B")

	cfg := dono.Config().ConnConfig
	poolAPI, err := store.Conectar(ctx, fmt.Sprintf(
		"postgres://fonteca_api:senha-de-teste@%s:%d/%s?sslmode=disable",
		cfg.Host, cfg.Port, cfg.Database))
	if err != nil {
		t.Fatalf("conectar como fonteca_api: %v", err)
	}
	t.Cleanup(poolAPI.Close)

	if err := store.VerificarPapelSeguro(ctx, poolAPI); err != nil {
		t.Fatalf("fonteca_api recusado pela verificacao do serve: %v", err)
	}

	h := Novo(poolAPI, opcoesDeTeste())

	contas := []struct{ nome, id, chave string }{
		{"A", contaA, chaveA},
		{"B", contaB, chaveB},
	}

	// Primeira passada: as duas contas consultam, entao as duas tem uso
	// registrado antes de qualquer leitura de /v1/uso. Se ler logo depois de
	// consultar, a conta A leria com B ainda vazia e nao provaria nada.
	for _, c := range contas {
		w := pesquisar(t, h, c.chave, map[string]any{
			"codigo_atividade_principal": []string{"2512800"},
			"situacao_cadastral":         "ATIVA",
			"uf":                         []string{"SP"},
			"data_abertura":              map[string]any{"ultimos_dias": diasDesdeInicioDoAlvo(t)},
			"mei":                        map[string]any{"excluir_optante": true},
			"mais_filtros":               map[string]any{"somente_celular": true},
		})
		if w.Code != http.StatusOK {
			t.Fatalf("conta %s, pesquisa: %d: %s", c.nome, w.Code, w.Body.String())
		}
		var pesquisa RespostaPesquisa
		if err := json.Unmarshal(w.Body.Bytes(), &pesquisa); err != nil {
			t.Fatalf("decodificar pesquisa: %v", err)
		}
		if len(pesquisa.Resultados) != 2 {
			t.Fatalf("conta %s, pesquisa trouxe %d, esperava 2", c.nome, len(pesquisa.Resultados))
		}

		req := httptest.NewRequest(http.MethodGet, "/v1/cnpj/"+pesquisa.Resultados[0].CNPJ, nil)
		req.Header.Set("api-key", c.chave)
		w = httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("conta %s, detalhe: %d: %s", c.nome, w.Code, w.Body.String())
		}
	}

	// Segunda passada: cada conta so ve a propria linha, nos dois sentidos.
	for _, c := range contas {
		req := httptest.NewRequest(http.MethodGet, "/v1/uso", nil)
		req.Header.Set("api-key", c.chave)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("conta %s, uso: %d: %s", c.nome, w.Code, w.Body.String())
		}
		var uso struct {
			Dias []store.Uso `json:"dias"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &uso); err != nil {
			t.Fatalf("decodificar uso: %v", err)
		}
		if len(uso.Dias) == 0 {
			t.Fatalf("conta %s, /v1/uso vazio depois de consultar", c.nome)
		}
		for _, d := range uso.Dias {
			if d.ContaID != c.id {
				t.Errorf("conta %s, /v1/uso trouxe linha da conta %s", c.nome, d.ContaID)
			}
		}
	}
}
