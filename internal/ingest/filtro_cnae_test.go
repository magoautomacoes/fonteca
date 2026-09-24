package ingest

import "testing"

func TestFiltroCNAENilAceitaTudo(t *testing.T) {
	var f *FiltroCNAE // nil: sem filtro
	if !f.Aceita("9999999", "") {
		t.Error("filtro nil devia aceitar qualquer CNAE")
	}
}

func TestFiltroCNAEVazioViraNil(t *testing.T) {
	if NovoFiltroCNAE(nil, false) != nil {
		t.Error("lista nil devia virar filtro nil")
	}
	if NovoFiltroCNAE([]string{}, false) != nil {
		t.Error("lista vazia devia virar filtro nil")
	}
	if NovoFiltroCNAE([]string{"", "  "}, false) != nil {
		t.Error("lista so com vazios devia virar filtro nil")
	}
}

func TestFiltroCNAEPrincipal(t *testing.T) {
	f := NovoFiltroCNAE([]string{"2512800", "2511000"}, false)

	casos := []struct {
		principal   string
		secundarios string
		quer        bool
		porque      string
	}{
		{"2512800", "", true, "CNAE pedido como principal"},
		{"2511000", "", true, "estruturas metalicas, principal"},
		{"1091102", "", false, "padaria: fora do recorte"},
		{" 2512800 ", "", true, "espacos em volta nao podem atrapalhar"},
		{"1091102", "2512800", false, "secundario NAO conta quando Secundarios e false"},
	}
	for _, c := range casos {
		if got := f.Aceita(c.principal, c.secundarios); got != c.quer {
			t.Errorf("Aceita(%q, %q) = %v; quer %v (%s)",
				c.principal, c.secundarios, got, c.quer, c.porque)
		}
	}
}

// A opcao que importa: muita empresa exerce a atividade procurada sem te-la
// como principal (uma construtora que tambem fabrica o que instala). Medido no lote real: dobra o alcance (0,93% para
// 1,95% da base) sem mudar a ordem de grandeza do custo.
func TestFiltroCNAESecundario(t *testing.T) {
	f := NovoFiltroCNAE([]string{"2512800"}, true)

	casos := []struct {
		principal   string
		secundarios string
		quer        bool
		porque      string
	}{
		{"2512800", "", true, "principal ainda vale"},
		{"4399103", "2512800", true, "obra de alvenaria QUE TAMBEM tem o CNAE pedido"},
		{"4399103", "1091102,2512800,4744001", true, "no meio de uma lista"},
		{"4399103", "1091102,4744001", false, "lista sem o codigo procurado"},
		{"4399103", "", false, "sem secundario nenhum"},
		{"4399103", " 2512800 , 1091102 ", true, "espacos na lista"},
		{"4399103", "25128001", false, "prefixo parecido nao pode casar"},
	}
	for _, c := range casos {
		if got := f.Aceita(c.principal, c.secundarios); got != c.quer {
			t.Errorf("Aceita(%q, %q) = %v; quer %v (%s)",
				c.principal, c.secundarios, got, c.quer, c.porque)
		}
	}
}
