package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/magoautomacoes/fonteca/internal/store"
	"github.com/magoautomacoes/fonteca/internal/tenant"
)

type loteResposta struct {
	Referencia  string    `json:"referencia"`
	Linhas      int64     `json:"linhas"`
	ImportadoEm time.Time `json:"importado_em"`
}

// health e o unico endpoint sem autenticacao: e o que o Caddy e os monitores
// consultam, e exigir chave transformaria o healthcheck em consumidor de cota.
// Nao expoe dado de empresa.
//
// Tambem e o unico que responde 200 com lote nulo — um monitor precisa
// distinguir "processo caiu" de "processo vivo, base ainda nao carregada".
//
// Sem chave nao ha limite por conta, entao o lote vigente fica em cache por
// validadeDoHealth: qualquer volume de chamadas custa no maximo uma consulta
// ao banco por janela. O lote so muda numa ingestao, entao segundos de atraso
// nao enganam ninguem.
func (s *servidor) health(w http.ResponseWriter, r *http.Request) {
	vigente, err := s.lote.obter(r.Context(), s.pool, s.agora())
	if err != nil {
		responderErroInterno(w, s.log, err)
		return
	}

	corpo := map[string]any{"status": "ok", "lote": nil}
	if vigente != nil {
		corpo["lote"] = loteResposta{
			Referencia:  vigente.Referencia,
			Linhas:      vigente.Linhas,
			ImportadoEm: vigente.ImportadoEm,
		}
	}
	responderJSON(w, http.StatusOK, corpo)
}

const validadeDoHealth = 5 * time.Second

// cacheDoLote guarda o ultimo LoteVigente lido com sucesso. Erro nao entra no
// cache: o proximo health tenta o banco de novo.
type cacheDoLote struct {
	mu     sync.Mutex
	lote   *store.Lote
	lidoEm time.Time
}

func (c *cacheDoLote) obter(ctx context.Context, pool *pgxpool.Pool, agora time.Time) (*store.Lote, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.lidoEm.IsZero() && agora.Sub(c.lidoEm) < validadeDoHealth {
		return c.lote, nil
	}
	lote, err := store.LoteVigente(ctx, pool)
	if err != nil {
		return nil, err
	}
	c.lote, c.lidoEm = lote, agora
	return lote, nil
}

// uso devolve o consumo da propria conta no mes. A leitura passa por
// tenant.LerUso, que declara a identidade na transacao — sob o usuario
// fonteca_api a RLS garante que uma conta nao enxerga o uso de outra.
func (s *servidor) uso(w http.ResponseWriter, r *http.Request, conta *store.Conta) {
	linhas, err := tenant.LerUso(r.Context(), s.pool, conta.ID)
	if err != nil {
		responderErroInterno(w, s.log, err)
		return
	}
	if linhas == nil {
		linhas = []store.Uso{}
	}
	responderJSON(w, http.StatusOK, map[string]any{
		"conta": conta.Nome,
		"dias":  linhas,
	})
}
