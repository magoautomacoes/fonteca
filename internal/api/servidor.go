package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/magoautomacoes/fonteca/internal/tenant"
)

// Opcoes configura o servidor. Tudo por injecao, para que o teste use limites
// altos e log silencioso sem tocar em variavel global.
type Opcoes struct {
	Log       *slog.Logger
	TTLCache  time.Duration
	PorMinuto int
	PorDia    int
	PorIP     int // falhas de autenticacao por minuto, por IP
	// ProxyConfiavel e o IP do proxy reverso (Caddy). So quando a conexao vem
	// dele o X-Forwarded-For e lido. Vazio = nunca confiar no cabecalho.
	ProxyConfiavel string

	// relogio existe para o teste controlar o tempo; nil = time.Now.
	relogio func() time.Time
}

func (o *Opcoes) comPadroes() {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.TTLCache <= 0 {
		o.TTLCache = 5 * time.Minute
	}
	if o.PorMinuto <= 0 {
		o.PorMinuto = 60
	}
	if o.PorDia <= 0 {
		o.PorDia = 10000
	}
	if o.relogio == nil {
		o.relogio = time.Now
	}
	if o.PorIP <= 0 {
		o.PorIP = 10
	}
}

type servidor struct {
	pool     *pgxpool.Pool
	log      *slog.Logger
	cache    *tenant.Cache
	porConta *tenant.Limitador
	porIP    *tenant.Limitador
	proxy    string
	agora    func() time.Time
	lote     cacheDoLote
}

// Novo monta o handler com rotas e middleware. Devolve http.Handler (em vez de
// subir servidor) para que o teste use httptest sem abrir porta.
func Novo(pool *pgxpool.Pool, opts Opcoes) http.Handler {
	opts.comPadroes()

	s := &servidor{
		pool:     pool,
		log:      opts.Log,
		cache:    tenant.NovoCache(opts.TTLCache, 10000),
		porConta: tenant.NovoLimitador(opts.PorMinuto, opts.PorDia),
		// Limite por IP: so janela de minuto, sem cota diaria (porDia 0). O teto
		// de chaves segura a memoria contra muitos IPs distintos (IPv6).
		porIP: tenant.NovoLimitador(opts.PorIP, 0).ComTeto(tenant.TetoPadraoDeEntradas),
		proxy: opts.ProxyConfiavel,
		agora: opts.relogio,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.health)
	mux.Handle("POST /v1/cnpj/pesquisa", s.autenticado(s.pesquisa))
	mux.Handle("POST /v1/cnpj/contagem", s.autenticado(s.contagem))
	mux.Handle("GET /v1/cnpj/{cnpj}", s.autenticado(s.detalhe))
	mux.Handle("GET /v1/uso", s.autenticado(s.uso))

	return s.recuperar(mux)
}
