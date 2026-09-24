package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// servidorDeArquivo serve conteudo com suporte a Range, como a Receita faz.
// Registra quantas requisicoes recebeu e qual Range veio na ultima, para que
// os testes possam provar QUE CAMINHO foi tomado, nao apenas que os bytes
// finais bateram.
func servidorDeArquivo(t *testing.T, conteudo []byte, contarRequisicoes *int, ultimoRange *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contarRequisicoes != nil {
			*contarRequisicoes++
		}
		if ultimoRange != nil {
			*ultimoRange = r.Header.Get("Range")
		}
		faixa := r.Header.Get("Range")
		if faixa == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(conteudo)))
			w.WriteHeader(http.StatusOK)
			w.Write(conteudo)
			return
		}
		// "bytes=N-" -> continuar de N
		inicio, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(faixa, "bytes="), "-"))
		if err != nil || inicio > len(conteudo) {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Range",
			"bytes "+strconv.Itoa(inicio)+"-"+strconv.Itoa(len(conteudo)-1)+"/"+strconv.Itoa(len(conteudo)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(conteudo[inicio:])
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestBaixarArquivoCompleto(t *testing.T) {
	conteudo := bytes.Repeat([]byte("fonteca"), 1000)
	srv := servidorDeArquivo(t, conteudo, nil, nil)
	f := &Fonte{BaseURL: srv.URL, Hash: "ABC", HTTP: srv.Client()}

	destino := filepath.Join(t.TempDir(), "Municipios.zip")
	a := Arquivo{Nome: "Municipios.zip", Tamanho: int64(len(conteudo))}

	if err := f.Baixar(context.Background(), "2026-09", a, destino); err != nil {
		t.Fatalf("Baixar: %v", err)
	}

	got, err := os.ReadFile(destino)
	if err != nil {
		t.Fatalf("ler destino: %v", err)
	}
	if !bytes.Equal(got, conteudo) {
		t.Errorf("conteudo baixado difere do servido (%d vs %d bytes)", len(got), len(conteudo))
	}
}

// O teste que justifica a complexidade: um arquivo pela metade deve ser
// COMPLETADO, nao rebaixado do zero.
func TestBaixarRetomaArquivoParcial(t *testing.T) {
	conteudo := bytes.Repeat([]byte("fonteca"), 1000)
	var requisicoes int
	var ultimoRange string
	srv := servidorDeArquivo(t, conteudo, &requisicoes, &ultimoRange)
	f := &Fonte{BaseURL: srv.URL, Hash: "ABC", HTTP: srv.Client()}

	destino := filepath.Join(t.TempDir(), "Municipios.zip")
	metade := len(conteudo) / 2
	if err := os.WriteFile(destino, conteudo[:metade], 0o644); err != nil {
		t.Fatalf("preparar parcial: %v", err)
	}

	a := Arquivo{Nome: "Municipios.zip", Tamanho: int64(len(conteudo))}
	if err := f.Baixar(context.Background(), "2026-09", a, destino); err != nil {
		t.Fatalf("Baixar: %v", err)
	}

	got, _ := os.ReadFile(destino)
	if !bytes.Equal(got, conteudo) {
		t.Errorf("arquivo retomado ficou corrompido: %d bytes, queria %d", len(got), len(conteudo))
	}

	if requisicoes != 1 {
		t.Errorf("requisicoes = %d; quer 1", requisicoes)
	}
	querRange := fmt.Sprintf("bytes=%d-", metade)
	if ultimoRange != querRange {
		t.Errorf("Range = %q; quer %q — sem isso o teste passaria tambem com "+
			"re-download completo, que e justamente o que ele existe para impedir",
			ultimoRange, querRange)
	}
}

// Arquivo ja completo: nao deve gerar nenhuma requisicao.
func TestBaixarPulaArquivoCompleto(t *testing.T) {
	conteudo := []byte("ja esta aqui inteiro")
	var requisicoes int
	srv := servidorDeArquivo(t, conteudo, &requisicoes, nil)
	f := &Fonte{BaseURL: srv.URL, Hash: "ABC", HTTP: srv.Client()}

	destino := filepath.Join(t.TempDir(), "Municipios.zip")
	os.WriteFile(destino, conteudo, 0o644)

	a := Arquivo{Nome: "Municipios.zip", Tamanho: int64(len(conteudo))}
	if err := f.Baixar(context.Background(), "2026-09", a, destino); err != nil {
		t.Fatalf("Baixar: %v", err)
	}
	if requisicoes != 0 {
		t.Errorf("fez %d requisicoes; arquivo completo nao deve gerar nenhuma", requisicoes)
	}
}

func TestBaixarDetectaTamanhoDivergente(t *testing.T) {
	srv := servidorDeArquivo(t, []byte("curto"), nil, nil)
	f := &Fonte{BaseURL: srv.URL, Hash: "ABC", HTTP: srv.Client()}

	destino := filepath.Join(t.TempDir(), "Municipios.zip")
	// O WebDAV anunciou 9999 bytes, mas o servidor entrega 5.
	a := Arquivo{Nome: "Municipios.zip", Tamanho: 9999}

	err := f.Baixar(context.Background(), "2026-09", a, destino)
	if !errors.Is(err, ErrTamanhoDivergente) {
		t.Fatalf("erro = %v; quer ErrTamanhoDivergente", err)
	}
}

// Um servidor que ignora Range responde 200 com o arquivo INTEIRO. Se o codigo
// anexasse esse corpo ao parcial que ja existe, o prefixo ficaria duplicado e o
// zip sairia corrompido — falha que so apareceria horas depois, na carga.
func TestBaixarServidorQueIgnoraRange(t *testing.T) {
	conteudo := bytes.Repeat([]byte("fonteca"), 1000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ignora o Range de proposito: responde 200 com tudo.
		w.Header().Set("Content-Length", strconv.Itoa(len(conteudo)))
		w.WriteHeader(http.StatusOK)
		w.Write(conteudo)
	}))
	t.Cleanup(srv.Close)
	f := &Fonte{BaseURL: srv.URL, Hash: "ABC", HTTP: srv.Client()}

	destino := filepath.Join(t.TempDir(), "Municipios.zip")
	metade := len(conteudo) / 2
	if err := os.WriteFile(destino, conteudo[:metade], 0o644); err != nil {
		t.Fatalf("preparar parcial: %v", err)
	}

	a := Arquivo{Nome: "Municipios.zip", Tamanho: int64(len(conteudo))}
	if err := f.Baixar(context.Background(), "2026-09", a, destino); err != nil {
		t.Fatalf("Baixar: %v", err)
	}

	got, _ := os.ReadFile(destino)
	if len(got) != len(conteudo) {
		t.Fatalf("arquivo com %d bytes; quer %d — prefixo duplicado", len(got), len(conteudo))
	}
	if !bytes.Equal(got, conteudo) {
		t.Error("conteudo difere: o corpo de um 200 foi anexado em vez de truncar")
	}
}

// Quando o WebDAV nao informa getcontentlength, Arquivo.Tamanho chega zerado
// (listagem.go nao valida isso). Sem tamanho conhecido nao ha como confiar no
// parcial local, entao Baixar deve recomecar do zero em vez de pedir uma
// faixa que o servidor recusaria com 416.
func TestBaixarComTamanhoDesconhecido(t *testing.T) {
	conteudo := []byte("conteudo completo aqui")
	var ultimoRange string
	srv := servidorDeArquivo(t, conteudo, nil, &ultimoRange)
	f := &Fonte{BaseURL: srv.URL, Hash: "ABC", HTTP: srv.Client()}

	destino := filepath.Join(t.TempDir(), "Municipios.zip")
	os.WriteFile(destino, conteudo, 0o644) // ja completo, mas nao ha como saber

	// Tamanho 0 = o WebDAV nao informou getcontentlength.
	a := Arquivo{Nome: "Municipios.zip", Tamanho: 0}
	if err := f.Baixar(context.Background(), "2026-09", a, destino); err != nil {
		t.Fatalf("Baixar com tamanho desconhecido devia funcionar, deu: %v", err)
	}
	if ultimoRange != "" {
		t.Errorf("Range = %q; sem tamanho conhecido nao se pode pedir faixa "+
			"(o servidor responderia 416 e a ingestao morreria)", ultimoRange)
	}

	got, _ := os.ReadFile(destino)
	if !bytes.Equal(got, conteudo) {
		t.Error("conteudo final incorreto")
	}
}
