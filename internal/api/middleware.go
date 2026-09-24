package api

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/magoautomacoes/fonteca/internal/store"
	"github.com/magoautomacoes/fonteca/internal/tenant"
)

type handlerAutenticado func(http.ResponseWriter, *http.Request, *store.Conta)

// autenticado resolve a chave, aplica os limites e entrega a conta ao handler.
//
// O limite por IP e o limite "sem auth" da spec: so FALHA de autenticacao
// (401, chave ausente ou invalida) consome a cota do IP. Requisicao
// autenticada nunca toca nela — responde so ao limite da conta. Isso importa
// atras de proxy: se toda requisicao cobrasse do IP, clientes diferentes que
// chegam pelo mesmo IP (NAT, proxy sem -proxy-confiavel) dividiriam 10/min.
//
// Antes de autenticar, o IP so e CONSULTADO (Bloqueado, sem consumir): IP que
// ja estourou recebe 429 sem chegar ao banco. Consequencia aceita: enquanto a
// janela nao passa, ate chave valida vinda de um IP bloqueado recebe 429 —
// quem divide IP com um atacante espera ate um minuto.
//
// Cobrar so no 401 nao deixa forca bruta barata: chave errada nunca chega ao
// Argon2id, porque a busca pelo SHA-256 (hash_lookup) falha antes. Cada
// tentativa custa uma consulta indexada e e cobrada no 401.
func (s *servidor) autenticado(fn handlerAutenticado) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agora := s.agora()

		ip := "ip:" + ipDoCliente(r, s.proxy)
		if bloq, espera := s.porIP.Bloqueado(ip, agora); bloq {
			responderLimite(w, espera, "limite por IP excedido")
			return
		}

		chave := r.Header.Get("api-key")
		if chave == "" {
			s.falhaDeAutenticacao(w, ip, agora, "chave ausente")
			return
		}

		conta, err := tenant.Autenticar(r.Context(), s.pool, s.cache, chave)
		if errors.Is(err, tenant.ErrChaveInvalida) {
			s.falhaDeAutenticacao(w, ip, agora, "chave invalida")
			return
		}
		if err != nil {
			responderErroInterno(w, s.log, err)
			return
		}

		if ok, espera := s.porConta.Permitir("conta:"+conta.ID, agora); !ok {
			responderLimite(w, espera, "limite da conta excedido")
			return
		}

		fn(w, r, conta)
	})
}

// falhaDeAutenticacao cobra a tentativa do IP e responde 401. Se a cobranca
// for recusada (corrida entre requisicoes simultaneas do mesmo IP, ou
// transbordo do limitador esgotado), responde 429 no lugar.
//
// Como a consulta (Bloqueado) vem antes e a cobranca so depois da busca da
// chave, uma rajada simultanea do mesmo IP passa inteira pela consulta antes
// da primeira cobranca: o excesso e limitado a concorrencia x latencia de uma
// busca indexada. Aceito — o custo de cada tentativa e so essa busca.
func (s *servidor) falhaDeAutenticacao(w http.ResponseWriter, ip string, agora time.Time, mensagem string) {
	if ok, espera := s.porIP.Permitir(ip, agora); !ok {
		responderLimite(w, espera, "limite por IP excedido")
		return
	}
	responderErro(w, http.StatusUnauthorized, mensagem)
}

func responderLimite(w http.ResponseWriter, espera time.Duration, mensagem string) {
	w.Header().Set("Retry-After", strconv.Itoa(int(espera.Seconds())+1))
	responderErro(w, http.StatusTooManyRequests, mensagem)
}

// recuperar impede que um panico derrube o processo e garante que a stack
// fique no log, nunca na resposta.
func (s *servidor) recuperar(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &respostaVigiada{ResponseWriter: w}
		defer func() {
			if p := recover(); p != nil {
				// ErrAbortHandler e o aborto deliberado de resposta: o
				// net/http o trata em silencio, entao repassamos.
				if p == http.ErrAbortHandler {
					panic(p)
				}
				if rw.escrito {
					// O status ja saiu: um 500 agora seria um segundo
					// WriteHeader (ignorado, com aviso) e colaria JSON no fim
					// de um corpo pela metade. Registra e aborta a conexao,
					// para o cliente ver resposta truncada, nao uma inteira.
					s.log.Error("panico depois da resposta iniciada", "erro", errPanico(p))
					panic(http.ErrAbortHandler)
				}
				responderErroInterno(w, s.log, errPanico(p))
			}
		}()
		next.ServeHTTP(rw, r)
	})
}

// respostaVigiada so registra se o cabecalho ja foi enviado.
type respostaVigiada struct {
	http.ResponseWriter
	escrito bool
}

func (r *respostaVigiada) WriteHeader(codigo int) {
	r.escrito = true
	r.ResponseWriter.WriteHeader(codigo)
}

func (r *respostaVigiada) Write(b []byte) (int, error) {
	r.escrito = true
	return r.ResponseWriter.Write(b)
}

// Unwrap deixa http.ResponseController alcancar o writer original.
func (r *respostaVigiada) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func errPanico(p any) error {
	if err, ok := p.(error); ok {
		return err
	}
	return errors.New("panico no handler")
}

// ipDoCliente devolve a identidade de rede usada pelo limite por IP.
//
// X-Forwarded-For so e lido quando a conexao vem do proxy configurado em
// -proxy-confiavel; de qualquer outro par o cabecalho e ignorado, senao cada
// tentativa de forca bruta mandaria um IP forjado e nunca seria contada. Do
// proxy, vale a entrada MAIS A DIREITA: e a que o proprio proxy escreveu com
// o par que ele viu. As da esquerda vem do cliente e podem ser inventadas.
//
// IPv6 e agrupado por /64: um unico assinante costuma receber a /64 inteira,
// e contar por endereco deixaria ele trocar de IP a cada tentativa.
func ipDoCliente(r *http.Request, proxy string) string {
	par, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		par = r.RemoteAddr
	}

	escolhido := par
	if proxy != "" && mesmoIP(par, proxy) {
		if valores := r.Header.Values("X-Forwarded-For"); len(valores) > 0 {
			ultimo := valores[len(valores)-1]
			if i := strings.LastIndexByte(ultimo, ','); i >= 0 {
				ultimo = ultimo[i+1:]
			}
			if ip := net.ParseIP(strings.TrimSpace(ultimo)); ip != nil {
				escolhido = ip.String()
			}
		}
	}

	ip := net.ParseIP(escolhido)
	if ip == nil {
		return escolhido
	}
	if ip.To4() == nil {
		return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
	}
	return ip.String()
}

func mesmoIP(a, b string) bool {
	ia, ib := net.ParseIP(a), net.ParseIP(b)
	if ia == nil || ib == nil {
		return a == b
	}
	return ia.Equal(ib)
}
