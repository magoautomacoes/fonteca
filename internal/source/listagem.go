package source

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"
)

// Arquivo e uma entrada do lote, com o que o WebDAV informa sem baixar nada.
type Arquivo struct {
	Nome       string
	Tamanho    int64
	Modificado time.Time
}

// Estruturas do XML do WebDAV (RFC 4918).
type multistatus struct {
	XMLName   xml.Name   `xml:"multistatus"`
	Respostas []resposta `xml:"response"`
}

type resposta struct {
	Href     string     `xml:"href"`
	Propstat []propstat `xml:"propstat"`
}

type propstat struct {
	Prop prop `xml:"prop"`
}

type prop struct {
	Tamanho      int64   `xml:"getcontentlength"`
	Modificado   string  `xml:"getlastmodified"`
	ResourceType recurso `xml:"resourcetype"`
}

type recurso struct {
	Collection *struct{} `xml:"collection"`
}

func (r resposta) ehPasta() bool {
	for _, ps := range r.Propstat {
		if ps.Prop.ResourceType.Collection != nil {
			return true
		}
	}
	return false
}

func (r resposta) propriedades() prop {
	for _, ps := range r.Propstat {
		if ps.Prop.Tamanho > 0 || ps.Prop.Modificado != "" {
			return ps.Prop
		}
	}
	if len(r.Propstat) > 0 {
		return r.Propstat[0].Prop
	}
	return prop{}
}

// propfind faz a requisicao e devolve o multistatus parseado.
func (f *Fonte) propfind(ctx context.Context, url string) (*multistatus, error) {
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", url, nil)
	if err != nil {
		return nil, fmt.Errorf("montar PROPFIND: %w", err)
	}
	// O hash do compartilhamento entra como usuario, com senha vazia.
	req.SetBasicAuth(f.Hash, "")
	req.Header.Set("Depth", "1")

	cliente := f.HTTP
	if cliente == nil {
		cliente = &http.Client{Timeout: 60 * time.Second}
	}

	resp, err := cliente.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PROPFIND em %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("PROPFIND em %s devolveu %s; esperado 207 Multi-Status "+
			"(o hash pode ter mudado)", url, resp.Status)
	}

	var ms multistatus
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil && err != io.EOF {
		return nil, fmt.Errorf("parsear XML de %s: %w", url, err)
	}
	return &ms, nil
}

// ListarLote devolve os arquivos de um lote mensal, sem baixar nada.
func (f *Fonte) ListarLote(ctx context.Context, referencia string) ([]Arquivo, error) {
	url, err := f.URLDoLote(referencia)
	if err != nil {
		return nil, err
	}

	ms, err := f.propfind(ctx, url)
	if err != nil {
		return nil, err
	}

	var arquivos []Arquivo
	for _, r := range ms.Respostas {
		if r.ehPasta() {
			continue
		}
		nome := path.Base(strings.TrimSuffix(r.Href, "/"))
		if nome == "" || nome == "." {
			continue
		}
		p := r.propriedades()
		var quando time.Time
		if p.Modificado != "" {
			// O WebDAV usa o formato de data do HTTP (RFC 1123).
			if t, err := time.Parse(time.RFC1123, p.Modificado); err == nil {
				quando = t
			}
		}
		arquivos = append(arquivos, Arquivo{Nome: nome, Tamanho: p.Tamanho, Modificado: quando})
	}
	return arquivos, nil
}

// ListarLotes devolve as referencias disponiveis, da mais antiga para a mais
// recente. Serve para descobrir se ha lote novo sem baixar nada.
func (f *Fonte) ListarLotes(ctx context.Context) ([]string, error) {
	ms, err := f.propfind(ctx, f.BaseURL+caminhoCNPJ+"/")
	if err != nil {
		return nil, err
	}

	var refs []string
	for _, r := range ms.Respostas {
		nome := path.Base(strings.TrimSuffix(r.Href, "/"))
		if padraoReferencia.MatchString(nome) {
			refs = append(refs, nome)
		}
	}
	sort.Strings(refs) // AAAA-MM ordena lexicograficamente igual a cronologicamente
	return refs, nil
}

// MaisRecente devolve a referencia do lote mais novo publicado.
func (f *Fonte) MaisRecente(ctx context.Context) (string, error) {
	refs, err := f.ListarLotes(ctx)
	if err != nil {
		return "", err
	}
	if len(refs) == 0 {
		return "", fmt.Errorf("nenhum lote encontrado em %s", caminhoCNPJ)
	}
	return refs[len(refs)-1], nil
}
