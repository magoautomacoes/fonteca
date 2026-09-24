package store

import (
	"context"
	"errors"
	"testing"
)

// O mapa do radar acende com ContarPorUF e a lista vem de Buscar: os dois
// tem de concordar para qualquer filtro, senao o mapa promete empresa que a
// lista nao mostra.
func TestContarPorUFConcordaComBuscar(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)
	schema, _ := NomeDoSchema(ReferenciaSeed)
	if err := CriarSchemaDeLote(ctx, pool, schema); err != nil {
		t.Fatalf("criar schema: %v", err)
	}
	if err := SemearLoteDeTeste(ctx, pool, schema); err != nil {
		t.Fatalf("semear: %v", err)
	}
	if err := TrocarLoteVigente(ctx, pool, schema, ReferenciaSeed, 12); err != nil {
		t.Fatalf("trocar: %v", err)
	}

	filtros := map[string]Filtro{
		// Sem filtro de contato nem data: 2512800 ativa em SP (ALFA, DELTA,
		// GAMA, EPSILON, MU), RJ (KAPPA) e MG (LAMBDA). BETA e baixada.
		"so CNAE": {CNAEs: []string{"2512800"}},
		"consulta-alvo": {
			CNAEs: []string{"2512800"}, UFs: []string{"SP"}, UltimosDias: 365,
			SoCelular: true, ExcluirMEI: true,
		},
		"varios CNAEs": {CNAEs: []string{"2512800", "2511000", "1622602", "4744001"}, SoCelular: true},
	}
	esperado := map[string]map[string]int64{
		"so CNAE":       {"SP": 5, "RJ": 1, "MG": 1},
		"consulta-alvo": {"SP": 2},
	}

	for nome, f := range filtros {
		t.Run(nome, func(t *testing.T) {
			porUF, err := ContarPorUF(ctx, pool, f)
			if err != nil {
				t.Fatalf("ContarPorUF: %v", err)
			}

			rows, err := Buscar(ctx, pool, f)
			if err != nil {
				t.Fatalf("Buscar: %v", err)
			}
			daBusca := map[string]int64{}
			for rows.Next() {
				v, err := rows.Values()
				if err != nil {
					t.Fatalf("valores: %v", err)
				}
				daBusca[v[9].(string)]++ // coluna uf
			}
			rows.Close()

			if len(porUF) != len(daBusca) {
				t.Fatalf("contagem %v, busca %v", porUF, daBusca)
			}
			for uf, n := range daBusca {
				if porUF[uf] != n {
					t.Errorf("%s: contagem %d, busca %d", uf, porUF[uf], n)
				}
			}
			if quer, ok := esperado[nome]; ok {
				for uf, n := range quer {
					if porUF[uf] != n {
						t.Errorf("%s: %d, quer %d (%v)", uf, porUF[uf], n, porUF)
					}
				}
			}
		})
	}
}

func TestContarPorUFRecusaFiltroInvalido(t *testing.T) {
	pool := bancoDeTeste(t)
	_, err := ContarPorUF(context.Background(), pool, Filtro{CNAEs: []string{"abc"}})
	if !errors.Is(err, ErrFiltroInvalido) {
		t.Errorf("erro %v, quer ErrFiltroInvalido", err)
	}
}
