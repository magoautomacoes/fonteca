package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
)

// ErrTamanhoDivergente indica que o arquivo baixado nao tem o tamanho que o
// WebDAV anunciou — download truncado ou arquivo trocado no meio do caminho.
var ErrTamanhoDivergente = errors.New("tamanho baixado difere do anunciado")

// Baixar grava o arquivo em destino, retomando se ja houver conteudo parcial.
// E idempotente: chamar com o arquivo ja completo nao faz requisicao alguma.
func (f *Fonte) Baixar(ctx context.Context, referencia string, a Arquivo, destino string) error {
	url, err := f.URLDoArquivo(referencia, a.Nome)
	if err != nil {
		return err
	}

	var jaTem int64
	if info, err := os.Stat(destino); err == nil {
		jaTem = info.Size()
		if a.Tamanho > 0 && jaTem == a.Tamanho {
			return nil // nada a fazer
		}
		if a.Tamanho > 0 && jaTem > a.Tamanho {
			// Arquivo local maior que o anunciado: recomecar do zero.
			if err := os.Remove(destino); err != nil {
				return fmt.Errorf("descartar parcial invalido %s: %w", destino, err)
			}
			jaTem = 0
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("montar GET de %s: %w", a.Nome, err)
	}
	req.SetBasicAuth(f.Hash, "")
	if jaTem > 0 && a.Tamanho > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", jaTem))
	} else {
		jaTem = 0 // tamanho desconhecido: nao da para confiar no parcial local
	}

	cliente := f.HTTP
	if cliente == nil {
		// Sem timeout total: 2,1 GB a 5,5 MB/s leva minutos. O ctx controla.
		cliente = &http.Client{}
	}

	resp, err := cliente.Do(req)
	if err != nil {
		return fmt.Errorf("baixar %s: %w", a.Nome, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		jaTem = 0 // servidor ignorou o Range: recomecar
	case http.StatusPartialContent:
		// continua de onde parou
	default:
		return fmt.Errorf("baixar %s: servidor devolveu %s", a.Nome, resp.Status)
	}

	flags := os.O_CREATE | os.O_WRONLY
	if jaTem > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}

	arq, err := os.OpenFile(destino, flags, 0o644)
	if err != nil {
		return fmt.Errorf("abrir %s: %w", destino, err)
	}

	escritos, copiaErr := io.Copy(arq, resp.Body)
	fecharErr := arq.Close()
	if copiaErr != nil {
		return fmt.Errorf("gravar %s: %w", destino, copiaErr)
	}
	if fecharErr != nil {
		return fmt.Errorf("fechar %s: %w", destino, fecharErr)
	}

	total := jaTem + escritos
	if a.Tamanho > 0 && total != a.Tamanho {
		return fmt.Errorf("%w: %s tem %d bytes, esperado %d",
			ErrTamanhoDivergente, a.Nome, total, a.Tamanho)
	}
	return nil
}
