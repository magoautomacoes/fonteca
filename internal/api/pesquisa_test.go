package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/magoautomacoes/fonteca/internal/store"
)

// diasDesdeInicioDoAlvo calcula, a partir do relogio real, quantos dias
// cobrem 2026-07-01 — inicio do periodo da consulta-alvo do docs/arquitetura.md.
// Calculado em runtime (em vez de um numero fixo) para que o teste nao
// dependa da data em que foi escrito: um valor fixo pararia de cobrir
// 2026-07-01 assim que o relogio avancasse o suficiente.
func diasDesdeInicioDoAlvo(t *testing.T) int {
	t.Helper()
	inicio := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	dias := int(time.Since(inicio).Hours()/24) + 1 // +1 para arredondar pra cima com folga
	if dias < 1 {
		t.Fatalf("relogio do sistema esta antes de 2026-07-01: ajuste o teste")
	}
	return dias
}

func pesquisar(t *testing.T, h http.Handler, chave string, corpo any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(corpo)
	if err != nil {
		t.Fatalf("serializar: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/cnpj/pesquisa", bytes.NewReader(b))
	req.Header.Set("api-key", chave)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// A ancora do contrato: a consulta-alvo completa da exatamente 2 no seed.
func TestPesquisaDevolveOsDoisDoSeed(t *testing.T) {
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
}

// O nono digito entra na leitura. Se alguem "corrigir" a regra do celular
// para 9 digitos, este teste cai — e ele existe para isso.
func TestPesquisaMontaWhatsAppComONonoDigito(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	_, chave := criarConta(t, pool, "Conta Teste")
	h := Novo(pool, opcoesDeTeste())

	w := pesquisar(t, h, chave, map[string]any{
		"codigo_atividade_principal": []string{"2512800"},
		"uf":                         []string{"SP"},
		"mais_filtros":               map[string]any{"somente_celular": true},
	})

	var corpo RespostaPesquisa
	if err := json.Unmarshal(w.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if len(corpo.Resultados) == 0 {
		t.Fatal("nenhum resultado para conferir o whatsapp")
	}
	for _, r := range corpo.Resultados {
		// 55 + DDD(2) + 9 + telefone(8) = 13 digitos
		if len(r.WhatsApp) != 13 {
			t.Errorf("whatsapp %q tem %d digitos, esperava 13 (55+DDD+9+8)", r.WhatsApp, len(r.WhatsApp))
		}
		if r.WhatsApp[:2] != "55" {
			t.Errorf("whatsapp %q nao comeca com 55", r.WhatsApp)
		}
		if r.WhatsApp[4] != '9' {
			t.Errorf("whatsapp %q nao tem o nono digito na posicao certa", r.WhatsApp)
		}
	}
}

func TestPesquisaRecusaSituacaoDesconhecida(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	_, chave := criarConta(t, pool, "Conta Teste")
	h := Novo(pool, opcoesDeTeste())

	w := pesquisar(t, h, chave, map[string]any{
		"codigo_atividade_principal": []string{"2512800"},
		"situacao_cadastral":         "ATIVISSIMA",
	})

	if w.Code != http.StatusBadRequest {
		t.Errorf("codigo %d, esperava 400 para situacao desconhecida", w.Code)
	}
}

// Filtro invalido continua 400 com mensagem util — mesmo quando o erro vem
// de dentro de store.Buscar (via store.ErrFiltroInvalido), nao so da
// validacao superficial do handler.
func TestPesquisaRecusaCNAEMalformado(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	_, chave := criarConta(t, pool, "Conta Teste")
	h := Novo(pool, opcoesDeTeste())

	w := pesquisar(t, h, chave, map[string]any{
		"codigo_atividade_principal": []string{"nao-e-cnae"},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("codigo %d, esperava 400", w.Code)
	}
	var corpo ErroResposta
	if err := json.Unmarshal(w.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if corpo.Erro == "" {
		t.Error("corpo sem mensagem de erro")
	}
	if corpo.Correlacao != "" {
		t.Errorf("erro de filtro (dominio) nao deveria ter correlacao, veio %q", corpo.Correlacao)
	}
}

// Erro de infraestrutura na primeira janela — o LoteVigente que o PROPRIO
// handler chama antes de store.Buscar — nunca pode virar 400 com o texto cru
// do Postgres. Fechamos o pool antes da requisicao para forcar essa falha: a
// mensagem no corpo tem que ser generica e trazer so o ID de correlacao.
func TestPesquisaErroDeInfraestruturaNoLoteVigenteVira500SemVazar(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	_, chave := criarConta(t, pool, "Conta Teste")
	h := Novo(pool, opcoesDeTeste())

	pool.Close() // toda chamada ao banco a partir daqui falha

	w := pesquisar(t, h, chave, map[string]any{
		"codigo_atividade_principal": []string{"2512800"},
	})

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("codigo %d, esperava 500: %s", w.Code, w.Body.String())
	}
	afirmarSemVazamentoDeInfra(t, w)
}

// Erro de infraestrutura na SEGUNDA janela — dentro de store.Buscar, depois
// que o LoteVigente do handler ja teve sucesso — e o cenario exato que a
// revisao da Task 9 provou com "ler lote vigente: context canceled" indo
// para um 400. Para reproduzir essa janela de forma determinada (sem
// depender de timing), registramos em meta.lote um schema com nome valido
// mas que nunca foi criado (TrocarLoteVigente so valida o FORMATO do nome,
// nao que o schema exista fisicamente — CriarSchemaDeLote nao roda aqui de
// proposito). LoteVigente le so o metadado em meta.lote e sucede tanto no
// handler quanto dentro de Buscar; a falha real e do Postgres ("relation
// does not exist") quando Buscar tenta consultar as tabelas do schema
// inexistente — um erro de infraestrutura genuino, nao fabricado, e que nao
// carrega store.ErrFiltroInvalido porque o filtro em si (CNAE valido) esta
// correto.
func TestPesquisaErroDeInfraestruturaDentroDeBuscarVira500SemVazar(t *testing.T) {
	pool := bancoDeTeste(t)
	schema, err := store.NomeDoSchema(store.ReferenciaSeed)
	if err != nil {
		t.Fatalf("nome do schema: %v", err)
	}
	// Sem CriarSchemaDeLote nem SemearLoteDeTeste: o schema so existe em
	// meta.lote, nao no banco.
	if err := store.TrocarLoteVigente(t.Context(), pool, schema, store.ReferenciaSeed, 0); err != nil {
		t.Fatalf("trocar vigente: %v", err)
	}
	_, chave := criarConta(t, pool, "Conta Teste")
	h := Novo(pool, opcoesDeTeste())

	w := pesquisar(t, h, chave, map[string]any{
		"codigo_atividade_principal": []string{"2512800"},
	})

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("codigo %d, esperava 500: %s", w.Code, w.Body.String())
	}
	afirmarSemVazamentoDeInfra(t, w)
}

func afirmarSemVazamentoDeInfra(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	corpoTexto := w.Body.String()
	for _, proibido := range []string{"lote vigente", "context", "connection", "pool", "closed"} {
		if bodyContem(corpoTexto, proibido) {
			t.Errorf("corpo vazou detalhe de infraestrutura (%q): %s", proibido, corpoTexto)
		}
	}
	var corpo ErroResposta
	if err := json.Unmarshal(w.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if corpo.Correlacao == "" {
		t.Error("erro de infraestrutura sem correlacao")
	}
}

// O teto e do servidor: pedir 999999 devolve no maximo LimiteMaximo.
func TestPesquisaAplicaOTetoDoServidor(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	_, chave := criarConta(t, pool, "Conta Teste")
	h := Novo(pool, opcoesDeTeste())

	w := pesquisar(t, h, chave, map[string]any{
		"codigo_atividade_principal": []string{"2512800"},
		"limite":                     999999,
	})

	var corpo RespostaPesquisa
	if err := json.Unmarshal(w.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if corpo.LimiteAplicado != store.LimiteMaximo {
		t.Errorf("limite_aplicado = %d, esperava %d", corpo.LimiteAplicado, store.LimiteMaximo)
	}
}

// Sem lote importado, os endpoints autenticados respondem 503 — nao 500 nem 200.
func TestPesquisaSemLoteDa503(t *testing.T) {
	pool := bancoDeTeste(t) // sem semearLote
	_, chave := criarConta(t, pool, "Conta Teste")
	h := Novo(pool, opcoesDeTeste())

	w := pesquisar(t, h, chave, map[string]any{
		"codigo_atividade_principal": []string{"2512800"},
	})

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("codigo %d, esperava 503", w.Code)
	}
}

// A consulta precisa aparecer em meta.uso: e o registro que a LGPD exige e a
// base da cota.
func TestPesquisaRegistraUso(t *testing.T) {
	pool := bancoDeTeste(t)
	semearLote(t, pool)
	contaID, chave := criarConta(t, pool, "Conta Teste")
	h := Novo(pool, opcoesDeTeste())

	pesquisar(t, h, chave, map[string]any{
		"codigo_atividade_principal": []string{"2512800"},
		"uf":                         []string{"SP"},
		"data_abertura":              map[string]any{"ultimos_dias": diasDesdeInicioDoAlvo(t)},
		"mais_filtros":               map[string]any{"somente_celular": true},
		"mei":                        map[string]any{"excluir_optante": true},
	})

	var consultas, linhas int64
	if err := pool.QueryRow(t.Context(),
		`SELECT consultas, linhas_retornadas FROM meta.uso WHERE conta_id = $1`,
		contaID).Scan(&consultas, &linhas); err != nil {
		t.Fatalf("ler uso: %v", err)
	}
	if consultas != 1 {
		t.Errorf("consultas = %d, esperava 1", consultas)
	}
	if linhas != 2 {
		t.Errorf("linhas_retornadas = %d, esperava 2", linhas)
	}
}
