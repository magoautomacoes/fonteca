package tenant

import (
	"context"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/magoautomacoes/fonteca/internal/store"
)

// poolRestrito devolve um pool conectado como um usuario que NAO e dono de
// meta.uso. Sem isso a RLS fica inerte e o teste de isolamento nao prova nada.
func poolRestrito(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	for _, cmd := range []string{
		`DROP ROLE IF EXISTS fonteca_api_teste`,
		`CREATE ROLE fonteca_api_teste LOGIN PASSWORD 'teste'`,
		`GRANT USAGE ON SCHEMA meta TO fonteca_api_teste`,
		`GRANT SELECT, INSERT, UPDATE ON meta.uso TO fonteca_api_teste`,
		`GRANT SELECT ON meta.conta, meta.api_key TO fonteca_api_teste`,
	} {
		if _, err := pool.Exec(ctx, cmd); err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
	}

	cfg := pool.Config().ConnConfig
	dsn := "postgres://fonteca_api_teste:teste@" + cfg.Host + ":" +
		strconv.Itoa(int(cfg.Port)) + "/" + cfg.Database + "?sslmode=disable"

	restrito, err := store.Conectar(ctx, dsn)
	if err != nil {
		t.Fatalf("conectar restrito: %v", err)
	}
	t.Cleanup(restrito.Close)
	return restrito
}

func TestRegistrarUsoAcumulaNoMesmoDia(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	contaID, _ := criarConta(t, pool, "Conta Teste")

	if err := RegistrarUso(ctx, pool, contaID, 10); err != nil {
		t.Fatalf("primeiro registro: %v", err)
	}
	if err := RegistrarUso(ctx, pool, contaID, 5); err != nil {
		t.Fatalf("segundo registro: %v", err)
	}

	var consultas, linhas int64
	if err := pool.QueryRow(ctx,
		`SELECT consultas, linhas_retornadas FROM meta.uso WHERE conta_id = $1`,
		contaID).Scan(&consultas, &linhas); err != nil {
		t.Fatalf("ler uso: %v", err)
	}
	if consultas != 2 {
		t.Errorf("consultas = %d, esperava 2", consultas)
	}
	if linhas != 15 {
		t.Errorf("linhas = %d, esperava 15", linhas)
	}
}

// TestIdentidadeMorreNoFimDaTransacao prova a diferenca entre SET LOCAL e SET
// de sessao diretamente: a mesma conexao fisica, apos RegistrarUso ter feito
// commit, nao pode mais enxergar a conta que acabou de usar. Se comIdentidade
// usasse SET de sessao (is_local=false), o valor sobreviveria ao commit e
// ficaria residual na conexao devolvida ao pool -- a proxima requisicao que
// pegar essa conexao herdaria a identidade da conta anterior, mesmo sem
// chamar RegistrarUso/LerUso primeiro. Esse e o mesmo tipo de bug que fez o
// projeto recusar `search_path` de sessao no docs/arquitetura.md.
//
// TestRLSIsolaContasNaMesmaConexao nao pega essa regressao porque toda chamada
// publica (RegistrarUso, LerUso) chama set_config de novo antes de qualquer
// leitura -- o residuo, se houver, e sempre sobrescrito antes de ser lido.
// Aqui, em vez disso, exercitamos RegistrarUso de verdade (sem mudar sua
// assinatura publica) e depois olhamos o estado da MESMA conexao fisica sem
// passar por outra chamada de comIdentidade no meio. Para garantir que e a
// mesma conexao, o pool restrito e aberto com pool_max_conns=1 no DSN: com
// uma unica conexao possivel, RegistrarUso e a checagem seguinte via Acquire
// sao forcosamente a mesma conexao fisica.
func TestIdentidadeMorreNoFimDaTransacao(t *testing.T) {
	dono := bancoDeTeste(t)
	ctx := context.Background()
	contaID, _ := criarConta(t, dono, "Conta Residuo")

	restrito := poolRestritoUmaConexao(t, dono)

	if err := RegistrarUso(ctx, restrito, contaID, 1); err != nil {
		t.Fatalf("registrar uso: %v", err)
	}

	// Adquire a conexao do pool: como o pool tem no maximo uma conexao e
	// RegistrarUso ja devolveu a dela, esta e forcosamente a mesma conexao
	// fisica que rodou o commit acima.
	conn, err := restrito.Acquire(ctx)
	if err != nil {
		t.Fatalf("adquirir conexao: %v", err)
	}
	defer conn.Release()

	var residuo string
	if err := conn.QueryRow(ctx,
		`SELECT current_setting('fonteca.conta_id', true)`).Scan(&residuo); err != nil {
		t.Fatalf("ler current_setting: %v", err)
	}
	if residuo != "" {
		t.Errorf("current_setting('fonteca.conta_id') = %q apos o commit, esperava vazio -- "+
			"a identidade sobreviveu na conexao e vazaria para a proxima requisicao", residuo)
	}
}

// poolRestritoUmaConexao e como poolRestrito, mas forca o pool a ter no
// maximo uma conexao fisica -- necessario para TestIdentidadeMorreNoFimDaTransacao
// provar, via Acquire, que esta olhando exatamente a conexao que RegistrarUso
// acabou de usar.
func poolRestritoUmaConexao(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	for _, cmd := range []string{
		`DROP ROLE IF EXISTS fonteca_api_teste_1conn`,
		`CREATE ROLE fonteca_api_teste_1conn LOGIN PASSWORD 'teste'`,
		`GRANT USAGE ON SCHEMA meta TO fonteca_api_teste_1conn`,
		`GRANT SELECT, INSERT, UPDATE ON meta.uso TO fonteca_api_teste_1conn`,
		`GRANT SELECT ON meta.conta, meta.api_key TO fonteca_api_teste_1conn`,
	} {
		if _, err := pool.Exec(ctx, cmd); err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
	}

	cfg := pool.Config().ConnConfig
	dsn := "postgres://fonteca_api_teste_1conn:teste@" + cfg.Host + ":" +
		strconv.Itoa(int(cfg.Port)) + "/" + cfg.Database +
		"?sslmode=disable&pool_max_conns=1"

	restrito, err := store.Conectar(ctx, dsn)
	if err != nil {
		t.Fatalf("conectar restrito: %v", err)
	}
	t.Cleanup(restrito.Close)
	return restrito
}

// O TESTE QUE IMPORTA: duas contas na mesma conexao do pool, em sequencia.
// Se o SET LOCAL sumir ou virar SET, a segunda conta enxerga o uso da primeira.
func TestRLSIsolaContasNaMesmaConexao(t *testing.T) {
	dono := bancoDeTeste(t)
	ctx := context.Background()
	contaA, _ := criarConta(t, dono, "Conta A")
	contaB, _ := criarConta(t, dono, "Conta B")

	if err := RegistrarUso(ctx, dono, contaA, 100); err != nil {
		t.Fatalf("uso da A: %v", err)
	}
	if err := RegistrarUso(ctx, dono, contaB, 7); err != nil {
		t.Fatalf("uso da B: %v", err)
	}

	restrito := poolRestrito(t, dono)

	linhasA, err := LerUso(ctx, restrito, contaA)
	if err != nil {
		t.Fatalf("ler uso da A: %v", err)
	}
	if len(linhasA) != 1 || linhasA[0].LinhasRetornadas != 100 {
		t.Fatalf("conta A viu %+v, esperava 1 linha com 100", linhasA)
	}

	// Mesma conexao, logo em seguida: a B nao pode ver nada da A.
	linhasB, err := LerUso(ctx, restrito, contaB)
	if err != nil {
		t.Fatalf("ler uso da B: %v", err)
	}
	if len(linhasB) != 1 {
		t.Fatalf("conta B viu %d linhas, esperava 1", len(linhasB))
	}
	if linhasB[0].LinhasRetornadas != 7 {
		t.Errorf("conta B viu %d linhas retornadas, esperava 7 — vazou o uso da conta A",
			linhasB[0].LinhasRetornadas)
	}
}

// Defesa em profundidade: mesmo sob o DONO de meta.uso (onde a RLS fica
// inerte), LerUso so devolve a propria conta, porque filtra conta_id na
// query.
func TestLerUsoFiltraContaMesmoSemRLS(t *testing.T) {
	dono := bancoDeTeste(t)
	ctx := context.Background()
	contaA, _ := criarConta(t, dono, "Conta A")
	contaB, _ := criarConta(t, dono, "Conta B")

	if err := RegistrarUso(ctx, dono, contaA, 1); err != nil {
		t.Fatalf("registrar A: %v", err)
	}
	if err := RegistrarUso(ctx, dono, contaB, 1); err != nil {
		t.Fatalf("registrar B: %v", err)
	}

	usos, err := LerUso(ctx, dono, contaA)
	if err != nil {
		t.Fatalf("ler uso: %v", err)
	}
	if len(usos) != 1 {
		t.Fatalf("LerUso(A) sob o dono devolveu %d linhas, esperava 1", len(usos))
	}
	for _, u := range usos {
		if u.ContaID != contaA {
			t.Errorf("LerUso(A) devolveu linha da conta %s", u.ContaID)
		}
	}
}

// Com o filtro conta_id na query de LerUso, TestRLSIsolaContasNaMesmaConexao
// passaria mesmo sem RLS. Este teste prova a outra camada sozinha: uma query
// SEM filtro, sob o usuario restrito e a identidade declarada, so enxerga a
// propria conta.
func TestRLSSozinhaIsolaSemFiltroNaQuery(t *testing.T) {
	dono := bancoDeTeste(t)
	ctx := context.Background()
	contaA, _ := criarConta(t, dono, "Conta A")
	contaB, _ := criarConta(t, dono, "Conta B")
	for _, c := range []string{contaA, contaB} {
		if err := RegistrarUso(ctx, dono, c, 1); err != nil {
			t.Fatalf("registrar uso: %v", err)
		}
	}

	restrito := poolRestrito(t, dono)
	var visiveis int
	var vista string
	err := comIdentidade(ctx, restrito, contaA, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*), min(conta_id::text) FROM meta.uso`).Scan(&visiveis, &vista)
	})
	if err != nil {
		t.Fatalf("consultar sem filtro: %v", err)
	}
	if visiveis != 1 || vista != contaA {
		t.Errorf("sem filtro na query, a RLS deixou ver %d linha(s) (min conta %s); esperava so a conta A", visiveis, vista)
	}
}
