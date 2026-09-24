// Package source descobre, lista e baixa os arquivos publicados pela Receita
// Federal. Nao sabe o que e um CNPJ: entrega arquivos.
package source

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// RaizReceita e a URL que redireciona para o compartilhamento atual.
const RaizReceita = "https://arquivos.receitafederal.gov.br"

// caminhoCNPJ e estavel desde 2023-05 (verificado em 16/09/2026).
const caminhoCNPJ = "/public.php/webdav/Dados/Cadastros/CNPJ"

// ErrHashNaoEncontrado indica que a raiz nao redirecionou para um share.
// Provavelmente o portal mudou de estrutura e o source precisa de ajuste.
var ErrHashNaoEncontrado = errors.New("nao foi possivel descobrir o hash do compartilhamento")

var (
	padraoHash       = regexp.MustCompile(`/s/([A-Za-z0-9]+)`)
	padraoReferencia = regexp.MustCompile(`^\d{4}-\d{2}$`)
	padraoNome       = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// Fonte guarda o endereco base e o hash do compartilhamento vigente.
type Fonte struct {
	BaseURL string
	Hash    string
	HTTP    *http.Client
}

// NovaFonte descobre o hash atual seguindo o redirect da raiz da Receita.
// Uma requisicao, sem parsear HTML: o hash vem na URL final.
func NovaFonte(ctx context.Context, cliente *http.Client) (*Fonte, error) {
	return novaFonteEm(ctx, cliente, RaizReceita)
}

// novaFonteEm existe para os testes apontarem a um servidor local.
func novaFonteEm(ctx context.Context, cliente *http.Client, raiz string) (*Fonte, error) {
	// O cliente da DESCOBERTA e o do DOWNLOAD sao coisas diferentes. Descobrir
	// o hash e uma requisicao curta, que merece timeout de 30s; baixar 2,1 GB
	// a 5 MB/s leva minutos e nao pode ter timeout total nenhum.
	//
	// Guardar o cliente da descoberta em f.HTTP fazia o Baixar herdar aquele
	// timeout, e o download do Simples.zip (293 MB) morria com
	// "context deadline exceeded" depois de 30s. Nenhum teste pegou isso porque
	// nenhum httptest serve 293 MB — so a carga real revelou.
	//
	// Por isso o cliente interno NAO e guardado: quando o chamador nao passa um
	// (producao), f.HTTP fica nil e cada operacao monta o cliente que precisa.
	// Quando o chamador passa (testes), respeitamos a escolha dele.
	descoberta := cliente
	if descoberta == nil {
		descoberta = &http.Client{Timeout: 30 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raiz+"/", nil)
	if err != nil {
		return nil, fmt.Errorf("montar requisicao para a raiz: %w", err)
	}

	resp, err := descoberta.Do(req)
	if err != nil {
		return nil, fmt.Errorf("acessar a raiz da Receita: %w", err)
	}
	defer resp.Body.Close()

	// A URL final, apos os redirects, carrega o hash.
	m := padraoHash.FindStringSubmatch(resp.Request.URL.Path)
	if m == nil {
		return nil, fmt.Errorf("%w: a raiz respondeu %s sem redirecionar para /s/<hash>; "+
			"o portal pode ter mudado de estrutura", ErrHashNaoEncontrado, resp.Status)
	}

	return &Fonte{BaseURL: raiz, Hash: m[1], HTTP: cliente}, nil
}

func validarReferencia(referencia string) error {
	if !padraoReferencia.MatchString(referencia) {
		return fmt.Errorf("referencia invalida: %q (esperado AAAA-MM)", referencia)
	}
	return nil
}

// URLDoLote devolve a URL WebDAV da pasta de um lote mensal.
func (f *Fonte) URLDoLote(referencia string) (string, error) {
	if err := validarReferencia(referencia); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%s/%s/", f.BaseURL, caminhoCNPJ, referencia), nil
}

// URLDoArquivo devolve a URL de um arquivo dentro do lote. O nome e validado
// contra um alfabeto restrito para que nao possa escapar da pasta.
func (f *Fonte) URLDoArquivo(referencia, nome string) (string, error) {
	if err := validarReferencia(referencia); err != nil {
		return "", err
	}
	if nome == "" || nome == "." || !padraoNome.MatchString(nome) || strings.Contains(nome, "..") {
		return "", fmt.Errorf("nome de arquivo invalido: %q", nome)
	}
	return fmt.Sprintf("%s%s/%s/%s", f.BaseURL, caminhoCNPJ, referencia, nome), nil
}
