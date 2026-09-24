package store

import "testing"

func TestSituacaoPorNome(t *testing.T) {
	casos := []struct {
		nome   string
		quer   int16
		querOK bool
	}{
		{"ATIVA", 2, true},
		{"ativa", 2, true},
		{"  Ativa  ", 2, true},
		{"BAIXADA", 8, true},
		{"SUSPENSA", 3, true},
		{"INAPTA", 4, true},
		{"NULA", 1, true},
		{"INEXISTENTE", 0, false},
		{"", 0, false},
	}
	for _, c := range casos {
		got, ok := SituacaoPorNome(c.nome)
		if got != c.quer || ok != c.querOK {
			t.Errorf("SituacaoPorNome(%q) = (%d, %v); quer (%d, %v)",
				c.nome, got, ok, c.quer, c.querOK)
		}
	}
}

func TestNomeDaSituacao(t *testing.T) {
	if got := NomeDaSituacao(SituacaoAtiva); got != "ATIVA" {
		t.Errorf("NomeDaSituacao(2) = %q; quer \"ATIVA\"", got)
	}
	if got := NomeDaSituacao(99); got != "" {
		t.Errorf("NomeDaSituacao(99) = %q; quer \"\"", got)
	}
}

func TestSituacaoAtivaEhDois(t *testing.T) {
	// A spec e os indices parciais dependem deste valor.
	if SituacaoAtiva != 2 {
		t.Fatalf("SituacaoAtiva = %d; quer 2", SituacaoAtiva)
	}
}
