package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponderErroUsaJSON(t *testing.T) {
	w := httptest.NewRecorder()
	responderErro(w, http.StatusBadRequest, "CNAE invalido")

	if w.Code != http.StatusBadRequest {
		t.Errorf("codigo %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type %q", ct)
	}
	var corpo ErroResposta
	if err := json.Unmarshal(w.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if corpo.Erro != "CNAE invalido" {
		t.Errorf("erro %q", corpo.Erro)
	}
}

// Mensagem de banco e mapa da estrutura para quem ataca: nunca vai ao cliente.
func TestErroInternoNaoVazaDetalhe(t *testing.T) {
	w := httptest.NewRecorder()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	responderErroInterno(w, log, errors.New(`relation "meta.api_key" does not exist`))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("codigo %d", w.Code)
	}
	corpoTexto := w.Body.String()
	if strings.Contains(corpoTexto, "meta.api_key") {
		t.Error("a mensagem do banco vazou na resposta")
	}

	var corpo ErroResposta
	if err := json.Unmarshal(w.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if corpo.Correlacao == "" {
		t.Error("erro 500 sem ID de correlacao: nao da para achar no log")
	}
}

func TestCorrelacaoEUnicaPorErro(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	vistos := map[string]bool{}

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		responderErroInterno(w, log, errors.New("falha"))
		var corpo ErroResposta
		if err := json.Unmarshal(w.Body.Bytes(), &corpo); err != nil {
			t.Fatalf("decodificar: %v", err)
		}
		if vistos[corpo.Correlacao] {
			t.Fatalf("ID de correlacao repetido: %s", corpo.Correlacao)
		}
		vistos[corpo.Correlacao] = true
	}
}

// Corpo que nao serializa: canal nao tem representacao JSON. O cliente nao
// pode receber 200 com corpo truncado — o status tem de contar a verdade.
func TestResponderJSONNaoEnviaStatusEnganosoQuandoOEncodeFalha(t *testing.T) {
	w := httptest.NewRecorder()

	responderJSON(w, http.StatusOK, map[string]any{"ruim": make(chan int)})

	if w.Code == http.StatusOK {
		t.Error("status 200 com corpo que nao serializa: o cliente recebe promessa de sucesso e corpo quebrado")
	}
	if w.Code != http.StatusInternalServerError {
		t.Errorf("codigo %d, esperava 500", w.Code)
	}
	if strings.Contains(w.Body.String(), "chan") {
		t.Errorf("detalhe do erro de serializacao vazou: %s", w.Body.String())
	}
}
