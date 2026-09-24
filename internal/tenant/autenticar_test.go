package tenant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAutenticarAceitaChaveValida(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	contaID, chave := criarConta(t, pool, "Conta Teste")

	conta, err := Autenticar(ctx, pool, NovoCache(time.Minute, 100), chave)
	if err != nil {
		t.Fatalf("autenticar: %v", err)
	}
	if conta.ID != contaID {
		t.Errorf("conta %q, esperava %q", conta.ID, contaID)
	}
	if conta.Nome != "Conta Teste" {
		t.Errorf("nome %q", conta.Nome)
	}
}

func TestAutenticarRecusaChaveDesconhecida(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	criarConta(t, pool, "Conta Teste")

	_, err := Autenticar(ctx, pool, NovoCache(time.Minute, 100), "fnt_live_naoexiste")
	if !errors.Is(err, ErrChaveInvalida) {
		t.Fatalf("erro %v, esperava ErrChaveInvalida", err)
	}
}

func TestAutenticarRecusaChaveRevogada(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	_, chave := criarConta(t, pool, "Conta Teste")

	if _, err := pool.Exec(ctx,
		`UPDATE meta.api_key SET revogada = true`); err != nil {
		t.Fatalf("revogar: %v", err)
	}

	if _, err := Autenticar(ctx, pool, NovoCache(time.Minute, 100), chave); !errors.Is(err, ErrChaveInvalida) {
		t.Fatalf("chave revogada foi aceita (erro=%v)", err)
	}
}

// Lacuna encontrada no teste de mutacao do brief: trocar VerificarArgon por
// "true" nao deixava nenhum teste vermelho, porque nenhum caso cobria uma
// linha cujo lookup casa mas cujo Argon nao bate com a chave apresentada.
// Sem este teste, um bug que pulasse a verificacao do Argon passaria batido.
func TestAutenticarRecusaArgonQueNaoBate(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	_, chave := criarConta(t, pool, "Conta Teste")

	// Corrompe o hash armazenado com o Argon de uma chave diferente, mas
	// mantendo o mesmo hash_lookup — a linha e encontrada, so o Argon nao bate.
	_, _, argonDeOutra, err := GerarChave()
	if err != nil {
		t.Fatalf("gerar outra chave: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE meta.api_key SET hash = $1`, argonDeOutra); err != nil {
		t.Fatalf("corromper hash: %v", err)
	}

	if _, err := Autenticar(ctx, pool, NovoCache(time.Minute, 100), chave); !errors.Is(err, ErrChaveInvalida) {
		t.Fatalf("chave com argon divergente foi aceita (erro=%v)", err)
	}
}

func TestAutenticarRecusaContaInativa(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	contaID, chave := criarConta(t, pool, "Conta Teste")

	if _, err := pool.Exec(ctx,
		`UPDATE meta.conta SET ativa = false WHERE id = $1`, contaID); err != nil {
		t.Fatalf("desativar: %v", err)
	}

	if _, err := Autenticar(ctx, pool, NovoCache(time.Minute, 100), chave); !errors.Is(err, ErrChaveInvalida) {
		t.Fatalf("conta inativa foi aceita (erro=%v)", err)
	}
}

func TestAutenticarRegistraUltimoUso(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	_, chave := criarConta(t, pool, "Conta Teste")

	if _, err := Autenticar(ctx, pool, NovoCache(time.Minute, 100), chave); err != nil {
		t.Fatalf("autenticar: %v", err)
	}

	var ultimo *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT ultimo_uso FROM meta.api_key`).Scan(&ultimo); err != nil {
		t.Fatalf("ler ultimo_uso: %v", err)
	}
	if ultimo == nil {
		t.Fatal("ultimo_uso continua nulo apos autenticar")
	}
}

// ultimo_uso e telemetria: se o UPDATE falhar, a autenticacao (ja aprovada
// pelo Argon2id) precisa seguir valendo mesmo assim. Substitui a variavel de
// pacote registrarUltimoUso por uma que sempre falha, para provocar o erro
// sem depender de infraestrutura de permissoes de banco que ainda nao existe
// neste pacote (isso fica para quando houver usuario restrito, na Task 5).
func TestAutenticarSeguePassandoQuandoRegistrarUltimoUsoFalha(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	contaID, chave := criarConta(t, pool, "Conta Teste")

	original := registrarUltimoUso
	registrarUltimoUso = func(ctx context.Context, pool *pgxpool.Pool, lookup string) error {
		return errors.New("falha simulada de telemetria")
	}
	defer func() { registrarUltimoUso = original }()

	conta, err := Autenticar(ctx, pool, NovoCache(time.Minute, 100), chave)
	if err != nil {
		t.Fatalf("autenticar deveria passar mesmo com falha no registro de uso: %v", err)
	}
	if conta.ID != contaID {
		t.Errorf("conta %q, esperava %q", conta.ID, contaID)
	}
}

// O cache existe para nao rodar Argon2id (~50ms) em toda requisicao. Se ele
// nao estiver funcionando, a segunda chamada custa o mesmo que a primeira.
func TestCacheEvitaOArgonNaSegundaChamada(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	_, chave := criarConta(t, pool, "Conta Teste")
	cache := NovoCache(time.Minute, 100)

	inicio := time.Now()
	if _, err := Autenticar(ctx, pool, cache, chave); err != nil {
		t.Fatalf("primeira: %v", err)
	}
	primeira := time.Since(inicio)

	inicio = time.Now()
	if _, err := Autenticar(ctx, pool, cache, chave); err != nil {
		t.Fatalf("segunda: %v", err)
	}
	segunda := time.Since(inicio)

	if segunda > primeira/2 {
		t.Errorf("segunda chamada (%v) nao foi muito mais rapida que a primeira (%v): o cache nao esta pegando", segunda, primeira)
	}
}

// Chave revogada depois de ter entrado no cache precisa parar de funcionar
// quando o TTL expira — cache com TTL longo vira chave irrevogavel.
func TestCacheExpiraEReconsultaOBanco(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	_, chave := criarConta(t, pool, "Conta Teste")
	cache := NovoCache(10*time.Millisecond, 100)

	if _, err := Autenticar(ctx, pool, cache, chave); err != nil {
		t.Fatalf("primeira: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE meta.api_key SET revogada = true`); err != nil {
		t.Fatalf("revogar: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	if _, err := Autenticar(ctx, pool, cache, chave); !errors.Is(err, ErrChaveInvalida) {
		t.Fatal("chave revogada continuou valendo depois do TTL expirar")
	}
}
