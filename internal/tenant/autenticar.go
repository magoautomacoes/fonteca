package tenant

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/magoautomacoes/fonteca/internal/store"
)

// ErrChaveInvalida cobre todos os motivos de recusa — chave desconhecida,
// revogada ou conta inativa. O cliente recebe 401 sem saber qual deles: dizer
// "chave existe mas esta revogada" entrega informacao a quem sonda.
var ErrChaveInvalida = errors.New("chave invalida")

type entradaCache struct {
	conta    store.Conta
	expiraEm time.Time
}

// Cache guarda chave valida -> conta por pouco tempo, para que o Argon2id
// (~50ms de proposito) nao rode em toda requisicao e vire vetor de exaustao
// de CPU. TTL curto porque uma chave revogada precisa parar de funcionar.
type Cache struct {
	mu       sync.Mutex
	entradas map[string]entradaCache
	ttl      time.Duration
	max      int
}

func NovoCache(ttl time.Duration, max int) *Cache {
	return &Cache{entradas: make(map[string]entradaCache), ttl: ttl, max: max}
}

func (c *Cache) ler(lookup string, agora time.Time) (store.Conta, bool) {
	if c == nil {
		return store.Conta{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entradas[lookup]
	if !ok || agora.After(e.expiraEm) {
		return store.Conta{}, false
	}
	return e.conta, true
}

func (c *Cache) gravar(lookup string, conta store.Conta, agora time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	// Varre expirados antes de crescer: sem isso o mapa so aumenta.
	if len(c.entradas) >= c.max {
		for k, e := range c.entradas {
			if agora.After(e.expiraEm) {
				delete(c.entradas, k)
			}
		}
		// Ainda cheio depois da varredura: descarta tudo. Simples e limitado;
		// o custo e alguns Argon2id a mais, nunca memoria sem teto.
		if len(c.entradas) >= c.max {
			c.entradas = make(map[string]entradaCache)
		}
	}
	c.entradas[lookup] = entradaCache{conta: conta, expiraEm: agora.Add(c.ttl)}
}

// Autenticar resolve a chave para uma conta. A busca usa o lookup
// deterministico; o Argon2id confirma. Chave sem o prefixo nem toca o banco.
func Autenticar(ctx context.Context, pool *pgxpool.Pool, cache *Cache, chave string) (*store.Conta, error) {
	if !strings.HasPrefix(chave, PrefixoChave) {
		return nil, ErrChaveInvalida
	}

	lookup := HashLookup(chave)
	agora := time.Now()

	if conta, ok := cache.ler(lookup, agora); ok {
		return &conta, nil
	}

	var argon string
	var conta store.Conta
	err := pool.QueryRow(ctx, `
		SELECT k.hash, c.id, c.nome, c.plano, c.criada_em, c.ativa
		FROM meta.api_key k
		JOIN meta.conta c ON c.id = k.conta_id
		WHERE k.hash_lookup = $1 AND NOT k.revogada AND c.ativa`, lookup).
		Scan(&argon, &conta.ID, &conta.Nome, &conta.Plano, &conta.CriadaEm, &conta.Ativa)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrChaveInvalida
	}
	if err != nil {
		return nil, fmt.Errorf("consultar chave: %w", err)
	}

	if !VerificarArgon(chave, argon) {
		return nil, ErrChaveInvalida
	}

	// ultimo_uso e telemetria: a chave ja passou pelo Argon2id com sucesso, e
	// recusar uma autenticacao valida por causa de um UPDATE de telemetria e
	// pior do que perder o registro. Alem disso este UPDATE esta no caminho
	// quente de toda autenticacao sem cache; se ele propagasse o erro, o
	// modo de falha pioraria justamente sob carga/contencao no banco, que e
	// o pior momento para recusar clientes legitimos. Por isso so logamos
	// (sem chave nem hash_lookup — hash_lookup deriva direto do segredo) e
	// seguimos.
	if err := registrarUltimoUso(ctx, pool, lookup); err != nil {
		slog.Default().Error("registrar ultimo uso da chave", "erro", err, "conta_id", conta.ID)
	}

	cache.gravar(lookup, conta, agora)
	return &conta, nil
}

// registrarUltimoUso e uma variavel de pacote (nao uma funcao livre) para que
// os testes possam substitui-la e provocar a falha do UPDATE sem depender de
// infraestrutura de permissoes de banco que ainda nao existe neste pacote.
//
// Nao ha sincronizacao nesta variavel: Autenticar a LE de varias goroutines de
// HTTP, e um teste que a ESCREVA em paralelo criaria data race de verdade. Por
// isso nenhum teste deste pacote usa t.Parallel(). Se algum dia usar, troque
// este seam por injecao explicita (campo de struct ou parametro) em vez de
// adicionar mutex aqui — o estado global e que e o problema.
var registrarUltimoUso = func(ctx context.Context, pool *pgxpool.Pool, lookup string) error {
	_, err := pool.Exec(ctx,
		`UPDATE meta.api_key SET ultimo_uso = now() WHERE hash_lookup = $1`, lookup)
	if err != nil {
		return fmt.Errorf("registrar ultimo uso: %w", err)
	}
	return nil
}
