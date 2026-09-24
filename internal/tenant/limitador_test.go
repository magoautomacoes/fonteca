package tenant

import (
	"strconv"
	"testing"
	"time"
)

func TestLimitadorPermiteAteOTeto(t *testing.T) {
	lim := NovoLimitador(3, 100)
	agora := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

	for i := 1; i <= 3; i++ {
		ok, _ := lim.Permitir("conta-a", agora)
		if !ok {
			t.Fatalf("requisicao %d foi barrada, devia passar", i)
		}
	}
	ok, espera := lim.Permitir("conta-a", agora)
	if ok {
		t.Fatal("a quarta requisicao passou, devia ser barrada")
	}
	if espera <= 0 {
		t.Error("Retry-After precisa ser positivo quando barra")
	}
}

func TestLimitadorLiberaNoMinutoSeguinte(t *testing.T) {
	lim := NovoLimitador(2, 100)
	agora := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

	lim.Permitir("conta-a", agora)
	lim.Permitir("conta-a", agora)
	if ok, _ := lim.Permitir("conta-a", agora); ok {
		t.Fatal("estourou o teto e passou")
	}

	if ok, _ := lim.Permitir("conta-a", agora.Add(61*time.Second)); !ok {
		t.Error("nao liberou no minuto seguinte")
	}
}

func TestLimitadorIsolaChaves(t *testing.T) {
	lim := NovoLimitador(1, 100)
	agora := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

	lim.Permitir("conta-a", agora)
	if ok, _ := lim.Permitir("conta-b", agora); !ok {
		t.Error("o consumo da conta-a barrou a conta-b")
	}
}

func TestLimitadorAplicaCotaDiaria(t *testing.T) {
	lim := NovoLimitador(1000, 5)
	agora := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

	for i := 1; i <= 5; i++ {
		if ok, _ := lim.Permitir("conta-a", agora); !ok {
			t.Fatalf("requisicao %d barrada dentro da cota diaria", i)
		}
	}
	if ok, _ := lim.Permitir("conta-a", agora.Add(time.Hour)); ok {
		t.Error("passou da cota diaria e nao foi barrada")
	}
	if ok, _ := lim.Permitir("conta-a", agora.Add(25*time.Hour)); !ok {
		t.Error("nao liberou no dia seguinte")
	}
}

// Sem varredura, um IP novo por requisicao vira vazamento de memoria lento.
func TestLimitadorDescartaEntradasVelhas(t *testing.T) {
	lim := NovoLimitador(10, 100)
	agora := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 500; i++ {
		lim.Permitir(string(rune('a'+i%26))+time.Duration(i).String(), agora)
	}
	depois := agora.Add(48 * time.Hour)
	lim.Permitir("gatilho", depois)

	if n := lim.Tamanho(); n > 50 {
		t.Errorf("%d entradas retidas apos 48h; a varredura nao rodou", n)
	}
}

// Bloqueado so consulta: nao pode gastar a cota de quem pergunta.
func TestLimitadorBloqueadoNaoConsome(t *testing.T) {
	lim := NovoLimitador(2, 0)
	agora := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 10; i++ {
		if bloq, _ := lim.Bloqueado("ip:a", agora); bloq {
			t.Fatalf("consulta %d: chave nunca usada aparece bloqueada", i)
		}
	}
	if ok, _ := lim.Permitir("ip:a", agora); !ok {
		t.Fatal("Bloqueado consumiu a cota: o primeiro Permitir foi barrado")
	}
	if ok, _ := lim.Permitir("ip:a", agora); !ok {
		t.Fatal("segundo Permitir barrado dentro do teto")
	}
	bloq, espera := lim.Bloqueado("ip:a", agora.Add(10*time.Second))
	if !bloq {
		t.Fatal("teto atingido e Bloqueado disse que nao")
	}
	if espera <= 0 || espera > time.Minute {
		t.Errorf("espera %v fora de (0, 1min]", espera)
	}
	if bloq, _ := lim.Bloqueado("ip:a", agora.Add(61*time.Second)); bloq {
		t.Error("continua bloqueado no minuto seguinte")
	}
}

// porDia <= 0 desliga a cota diaria: o limitador por IP so tem janela de
// minuto.
func TestLimitadorSemCotaDiaria(t *testing.T) {
	lim := NovoLimitador(5, 0)
	agora := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

	for m := 0; m < 30; m++ {
		for i := 0; i < 5; i++ {
			if ok, _ := lim.Permitir("ip:a", agora.Add(time.Duration(m)*time.Minute)); !ok {
				t.Fatalf("minuto %d, requisicao %d barrada sem cota diaria", m, i)
			}
		}
	}
}

// Sem cota diaria, a janela mais longa e o minuto: entrada parada ha mais de
// um minuto nao guarda estado util e tem de sair logo, nao em 24h.
func TestLimitadorSemCotaDiariaDescartaAposUmMinuto(t *testing.T) {
	lim := NovoLimitador(10, 0)
	agora := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 500; i++ {
		lim.Permitir("ip:"+strconv.Itoa(i), agora)
	}
	lim.Permitir("gatilho", agora.Add(2*time.Minute))

	if n := lim.Tamanho(); n != 1 {
		t.Errorf("%d entradas retidas 2min depois; esperava so o gatilho", n)
	}
}

// Com cota diaria a entrada precisa sobreviver ao dia: descartar antes
// zeraria a cota de quem ficou parado uma hora.
func TestLimitadorComCotaDiariaRetemEntradaNoDia(t *testing.T) {
	lim := NovoLimitador(1000, 3)
	agora := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		lim.Permitir("conta-a", agora)
	}
	if ok, _ := lim.Permitir("conta-a", agora.Add(2*time.Hour)); ok {
		t.Error("a varredura apagou a cota diaria de quem ficou parado 2h")
	}
}

// Tabela cheia nao pode virar porta aberta: chaves novas dividem uma unica
// janela de transbordo, que Bloqueado tambem enxerga — senao cada IP novo
// passaria direto pela consulta previa e iria ao banco sem ser contado.
func TestLimitadorTabelaCheiaCobraChaveNovaDoTransbordo(t *testing.T) {
	lim := NovoLimitador(2, 0).ComTeto(3)
	agora := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

	for _, k := range []string{"a", "b", "c"} {
		if ok, _ := lim.Permitir(k, agora); !ok {
			t.Fatalf("%s barrada antes de encher", k)
		}
	}

	// d e e sao chaves diferentes, mas dividem o transbordo (teto 2).
	for _, k := range []string{"d", "e"} {
		if ok, _ := lim.Permitir(k, agora); !ok {
			t.Fatalf("%s barrada com o transbordo ainda dentro da cota", k)
		}
	}
	ok, espera := lim.Permitir("f", agora)
	if ok {
		t.Fatal("transbordo esgotado aceitou chave nova: IPs novos escapariam do limite")
	}
	if espera <= 0 {
		t.Error("recusa do transbordo sem espera positiva")
	}
	if bloq, _ := lim.Bloqueado("g", agora); !bloq {
		t.Error("Bloqueado nao enxerga o transbordo: IP novo iria ao banco sem ser barrado")
	}

	if ok, _ := lim.Permitir("a", agora); !ok {
		t.Error("tabela cheia barrou chave ja rastreada dentro da cota")
	}
	if n := lim.Tamanho(); n > 3 {
		t.Errorf("tabela passou do teto: %d", n)
	}

	// Passada a janela, a varredura forcada abre espaco e a chave nova volta
	// a ter entrada propria.
	depois := agora.Add(2 * time.Minute)
	if ok, _ := lim.Permitir("d", depois); !ok {
		t.Error("depois da janela, a varredura nao abriu espaco para chave nova")
	}
	if bloq, _ := lim.Bloqueado("g", depois); bloq {
		t.Error("com espaco na tabela, chave desconhecida nao pode estar bloqueada")
	}
}
