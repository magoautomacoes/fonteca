package tenant

import (
	"context"
	"testing"
	"time"
)

func TestCriarContaGeraChaveUsavel(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()

	contaID, chave, err := CriarConta(ctx, pool, "Conta Teste")
	if err != nil {
		t.Fatalf("criar conta: %v", err)
	}
	if contaID == "" || chave == "" {
		t.Fatal("conta ou chave vazia")
	}

	conta, err := Autenticar(ctx, pool, NovoCache(time.Minute, 10), chave)
	if err != nil {
		t.Fatalf("a chave recem-criada nao autentica: %v", err)
	}
	if conta.ID != contaID {
		t.Errorf("conta %q, esperava %q", conta.ID, contaID)
	}
}

// A chave em claro nunca pode estar no banco.
func TestCriarContaNaoArmazenaAChave(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()

	_, chave, err := CriarConta(ctx, pool, "Conta Teste")
	if err != nil {
		t.Fatalf("criar conta: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM meta.api_key WHERE hash = $1 OR hash_lookup = $1`,
		chave).Scan(&n); err != nil {
		t.Fatalf("consultar: %v", err)
	}
	if n != 0 {
		t.Error("a chave em claro esta no banco")
	}
}
