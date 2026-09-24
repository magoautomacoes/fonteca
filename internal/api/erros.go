// Package api expoe a consulta-alvo do Fonteca como HTTP. Conhece o dominio
// de CNPJ e o protocolo; nao conhece Argon2id nem token bucket (isso e do
// pacote tenant).
package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
)

// ErroResposta e o corpo de toda resposta de erro.
type ErroResposta struct {
	Erro       string `json:"erro"`
	Correlacao string `json:"correlacao,omitempty"`
}

// responderJSON serializa ANTES de escrever o cabecalho. A ordem importa:
// escrever o status primeiro e serializar depois entrega, quando o encode
// falha no meio, um corpo truncado sob um status que promete sucesso — e
// status ja enviado nao se corrige. Todo handler passa por aqui, entao o
// defeito se propagaria pelo pacote inteiro.
func responderJSON(w http.ResponseWriter, codigo int, corpo any) {
	bruto, err := json.Marshal(corpo)
	if err != nil {
		slog.Default().Error("serializar resposta", "erro", err)
		http.Error(w, `{"erro":"erro interno"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(codigo)
	if _, err := w.Write(bruto); err != nil {
		// Cliente desistiu no meio da escrita; nao ha o que responder.
		slog.Default().Error("escrever resposta", "erro", err)
	}
}

// responderErro devolve uma mensagem de dominio ao cliente.
//
// `mensagem` tem de ser texto NOSSO — literal ou construido por nos. Nunca
// passe `err.Error()` de uma dependencia (banco, rede, driver): a mensagem do
// Postgres descreve a estrutura do schema, que e justamente o que a API nao
// conta a quem ataca. Para erro vindo de dependencia, use responderErroInterno,
// que manda o detalhe ao log e devolve so o ID de correlacao.
func responderErro(w http.ResponseWriter, codigo int, mensagem string) {
	responderJSON(w, codigo, ErroResposta{Erro: mensagem})
}

// responderErroInterno devolve so um identificador ao cliente e manda o
// detalhe para o log: texto de erro do Postgres e mapa da estrutura para
// quem ataca.
func responderErroInterno(w http.ResponseWriter, log *slog.Logger, err error) {
	id := correlacao()
	log.Error("erro interno", "correlacao", id, "erro", err)
	responderJSON(w, http.StatusInternalServerError, ErroResposta{
		Erro:       "erro interno",
		Correlacao: id,
	})
}

func correlacao() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "sem-id"
	}
	return hex.EncodeToString(b)
}
