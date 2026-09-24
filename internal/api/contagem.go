package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/magoautomacoes/fonteca/internal/store"
	"github.com/magoautomacoes/fonteca/internal/tenant"
)

// RespostaContagem diz quantas empresas o filtro acha em cada estado. E o que
// acende o mapa do radar antes (ou em vez) de paginar a lista inteira.
type RespostaContagem struct {
	Total int64            `json:"total"`
	PorUF map[string]int64 `json:"por_uf"`
	Lote  string           `json:"lote"`
}

// contagem recebe o mesmo corpo da pesquisa; limite e cursor sao ignorados.
func (s *servidor) contagem(w http.ResponseWriter, r *http.Request, conta *store.Conta) {
	var pedido PedidoPesquisa
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, corpoMax))
	if err := dec.Decode(&pedido); err != nil {
		responderErro(w, http.StatusBadRequest, "corpo invalido: "+err.Error())
		return
	}
	filtro, ok := filtroDoPedido(w, pedido)
	if !ok {
		return
	}

	vigente, err := store.LoteVigente(r.Context(), s.pool)
	if err != nil {
		responderErroInterno(w, s.log, err)
		return
	}
	if vigente == nil {
		responderErro(w, http.StatusServiceUnavailable, "nenhum lote importado ainda")
		return
	}

	// Mesmo tratamento de erro de pesquisa: so erro de filtro vira 400 com
	// texto nosso; falha de banco vira 500 com ID de correlacao.
	porUF, err := store.ContarPorUF(r.Context(), s.pool, filtro)
	if errors.Is(err, store.ErrFiltroInvalido) {
		responderErro(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		responderErroInterno(w, s.log, err)
		return
	}

	var total int64
	for _, n := range porUF {
		total += n
	}

	// Conta como consulta, sem linhas: nenhuma empresa sai daqui.
	if err := tenant.RegistrarUso(r.Context(), s.pool, conta.ID, 0); err != nil {
		s.log.Error("registrar uso", "conta_id", conta.ID, "erro", err)
	}

	responderJSON(w, http.StatusOK, RespostaContagem{Total: total, PorUF: porUF, Lote: vigente.Referencia})
}
