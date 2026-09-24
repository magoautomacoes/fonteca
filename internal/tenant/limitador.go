package tenant

import (
	"sync"
	"time"
)

type janela struct {
	minuto       time.Time
	noMinuto     int
	dia          time.Time
	noDia        int
	ultimoAcesso time.Time
}

// TetoPadraoDeEntradas e quantas chaves o limitador rastreia por padrao. Com
// ~100 bytes por entrada, 10 mil chaves custam ~1 MiB — o mesmo teto do
// Cache de autenticacao.
const TetoPadraoDeEntradas = 10000

// Limitador conta requisicoes por chave (conta ou IP) em duas janelas fixas:
// minuto e dia. Vive em memoria, por processo — zera no restart e nao coordena
// entre replicas. E aceitavel porque o limite que realmente protege a base
// (teto de 1.000 linhas por resposta) nao depende disto.
//
// porDia <= 0 desliga a cota diaria (o limitador por IP so tem janela de
// minuto). maxEntradas <= 0 desliga o teto de chaves rastreadas.
type Limitador struct {
	mu              sync.Mutex
	janelas         map[string]*janela
	porMinuto       int
	porDia          int
	maxEntradas     int
	ultimaVarredura time.Time
	ultimaForcada   time.Time
	// transbordo e a janela COMPARTILHADA por toda chave nova que chega com a
	// tabela cheia. Sem ela, chave nova nao era contada em lugar nenhum: nao
	// havia o que Bloqueado consultar, e cada tentativa ia ao banco.
	transbordo janela
}

// NovoLimitador cria um limitador sem teto de chaves. Use ComTeto quando a
// chave vem de fora (IP), para que a memoria nao cresca sem limite.
func NovoLimitador(porMinuto, porDia int) *Limitador {
	return &Limitador{
		janelas:   make(map[string]*janela),
		porMinuto: porMinuto,
		porDia:    porDia,
	}
}

// ComTeto limita quantas chaves o limitador rastreia ao mesmo tempo. Com a
// tabela cheia, primeiro roda uma varredura (no maximo uma por segundo); se
// continuar cheia, toda chave NOVA passa a dividir uma unica janela de
// transbordo, com o mesmo limite de uma chave. Descartar a entrada mais velha
// deixaria um atacante com muitos IPs apagar o proprio historico e recomecar
// a cota; o transbordo mantem todos contados. O preco e que, sob um ataque
// desses, um IP legitimo novo divide a cota do atacante — mas so no caminho
// sem autenticacao (ver api.autenticado).
func (l *Limitador) ComTeto(maxEntradas int) *Limitador {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maxEntradas = maxEntradas
	return l
}

// Permitir consome uma requisicao da chave. Devolve false e quanto falta para
// liberar. O relogio vem por parametro para que o teste nao dependa de tempo
// de parede.
func (l *Limitador) Permitir(chave string, agora time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.varrer(agora, false)

	// Janela FIXA, nao deslizante: quem manda o teto no fim de um minuto e o
	// teto no inicio do seguinte passa 2x o limite em poucos segundos reais. E
	// troca consciente — este limitador protege de abuso, nao de colapso; o
	// teto que impede varrer a base e o de linhas por resposta, no store.
	j, ok := l.janelas[chave]
	if !ok {
		if l.cheio() {
			l.varrerForcado(agora)
		}
		if l.cheio() {
			// Tabela cheia de entradas vivas: a chave nova e cobrada do
			// transbordo, entao continua contada (e bloqueavel) sem ocupar
			// memoria nova.
			return l.consumir(&l.transbordo, agora)
		}
		j = &janela{}
		l.janelas[chave] = j
	}
	return l.consumir(j, agora)
}

// consumir cobra uma requisicao da janela. Chamada ja com o mutex.
func (l *Limitador) consumir(j *janela, agora time.Time) (bool, time.Duration) {
	minutoAtual := agora.Truncate(time.Minute)
	// Truncate corta sobre o tempo absoluto desde a epoca (UTC), nao sobre o
	// dia civil local: no Brasil (UTC-3) a cota diaria vira as 21h, nao a
	// meia-noite. Aceitavel porque "dia" aqui e so uma janela de 24h que
	// reseta; quem for depurar a cota precisa saber disso.
	diaAtual := agora.Truncate(24 * time.Hour)

	j.ultimoAcesso = agora
	if !j.minuto.Equal(minutoAtual) {
		j.minuto, j.noMinuto = minutoAtual, 0
	}
	if !j.dia.Equal(diaAtual) {
		j.dia, j.noDia = diaAtual, 0
	}

	if l.porDia > 0 && j.noDia >= l.porDia {
		return false, diaAtual.Add(24 * time.Hour).Sub(agora)
	}
	if j.noMinuto >= l.porMinuto {
		return false, minutoAtual.Add(time.Minute).Sub(agora)
	}

	j.noMinuto++
	j.noDia++
	return true, 0
}

// Bloqueado diz se a chave ja esgotou a cota, SEM consumir nada e sem criar
// entrada. Chave desconhecida com a tabela cheia responde pelo transbordo —
// e a mesma janela que Permitir cobraria dela.
func (l *Limitador) Bloqueado(chave string, agora time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	j, ok := l.janelas[chave]
	if !ok {
		if !l.cheio() {
			return false, 0
		}
		j = &l.transbordo
	}
	return l.esgotada(j, agora)
}

// esgotada diz se a janela esta no teto. Chamada ja com o mutex.
func (l *Limitador) esgotada(j *janela, agora time.Time) (bool, time.Duration) {
	minutoAtual := agora.Truncate(time.Minute)
	diaAtual := agora.Truncate(24 * time.Hour)

	if l.porDia > 0 && j.dia.Equal(diaAtual) && j.noDia >= l.porDia {
		return true, diaAtual.Add(24 * time.Hour).Sub(agora)
	}
	if j.minuto.Equal(minutoAtual) && j.noMinuto >= l.porMinuto {
		return true, minutoAtual.Add(time.Minute).Sub(agora)
	}
	return false, 0
}

// Tamanho e o numero de chaves rastreadas. Existe para o teste provar que a
// varredura roda.
func (l *Limitador) Tamanho() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.janelas)
}

func (l *Limitador) cheio() bool {
	return l.maxEntradas > 0 && len(l.janelas) >= l.maxEntradas
}

// janelaMaisLonga e por quanto tempo uma entrada parada ainda guarda estado
// util. Sem cota diaria, passado um minuto a contagem ja zeraria; com cota
// diaria, a entrada precisa viver o dia, senao a cota reseta ao descartar.
func (l *Limitador) janelaMaisLonga() time.Duration {
	if l.porDia > 0 {
		return 24 * time.Hour
	}
	return time.Minute
}

// varrerForcado e a varredura pedida pela tabela cheia, limitada a uma por
// segundo: sob ataque, toda requisicao nova acharia a tabela cheia, e varrer
// O(n) em cada uma seguraria o mutex que as requisicoes autenticadas tambem
// usam. Um segundo de atraso na liberacao de espaco nao muda nada.
func (l *Limitador) varrerForcado(agora time.Time) {
	if agora.Sub(l.ultimaForcada) < time.Second {
		return
	}
	l.ultimaForcada = agora
	l.varrer(agora, true)
}

// varrer descarta chaves paradas ha mais que a janela mais longa. Roda no
// maximo uma vez por janela (e no maximo por hora), salvo quando forcada pela
// tabela cheia (ver varrerForcado). Chamada ja com o mutex.
func (l *Limitador) varrer(agora time.Time, forcar bool) {
	janela := l.janelaMaisLonga()
	intervalo := min(janela, time.Hour)
	if !forcar && agora.Sub(l.ultimaVarredura) < intervalo {
		return
	}
	l.ultimaVarredura = agora
	for k, j := range l.janelas {
		if agora.Sub(j.ultimoAcesso) > janela {
			delete(l.janelas, k)
		}
	}
}
