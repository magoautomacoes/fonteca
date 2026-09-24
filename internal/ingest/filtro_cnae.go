package ingest

import "strings"

// FiltroCNAE restringe a carga de estabelecimentos a um conjunto de CNAEs.
// Vazio significa carregar tudo, que e o comportamento historico.
//
// Por que isso existe: medido no lote 2026-09, quatro CNAEs de um mesmo ramo
// aparecem em 1,95% dos estabelecimentos — 1,23 milhao de linhas em vez de 63
// milhoes. Guardar os 98% restantes custa horas de carga e dezenas de GB para
// responder consultas que nunca os tocam.
type FiltroCNAE struct {
	codigos map[string]bool
	// Secundarios inclui quem tem o CNAE como atividade secundaria, nao so
	// principal. Muita empresa exerce a atividade procurada sem te-la como
	// principal: uma construtora que tambem fabrica o que instala, por
	// exemplo. Medido: dobra o alcance (0,93% para 1,95%) por um custo que
	// continua sendo 2% da base.
	Secundarios bool
}

// NovoFiltroCNAE monta o filtro. Lista vazia devolve nil, e um filtro nil
// aceita tudo — assim o chamador nao precisa tratar o caso especial.
func NovoFiltroCNAE(codigos []string, secundarios bool) *FiltroCNAE {
	if len(codigos) == 0 {
		return nil
	}
	m := make(map[string]bool, len(codigos))
	for _, c := range codigos {
		if c = strings.TrimSpace(c); c != "" {
			m[c] = true
		}
	}
	if len(m) == 0 {
		return nil
	}
	return &FiltroCNAE{codigos: m, Secundarios: secundarios}
}

// Aceita diz se o estabelecimento entra na carga. Um filtro nil aceita tudo.
func (f *FiltroCNAE) Aceita(principal, secundarios string) bool {
	if f == nil {
		return true
	}
	if f.codigos[strings.TrimSpace(principal)] {
		return true
	}
	if !f.Secundarios || secundarios == "" {
		return false
	}
	// O campo de secundarios e uma lista separada por virgula.
	for _, c := range strings.Split(secundarios, ",") {
		if f.codigos[strings.TrimSpace(c)] {
			return true
		}
	}
	return false
}
