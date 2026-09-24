package ingest

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// conversorSimples converte "codigo;nome" em duas colunas.
func conversorSimples(campos []string) ([]any, error) {
	if len(campos) < 2 {
		return nil, ErrLinhaInvalida
	}
	n, err := strconv.Atoi(campos[0])
	if err != nil {
		return nil, ErrLinhaInvalida
	}
	return []any{int32(n), campos[1]}, nil
}

func TestFonteDeLinhasPercorreTudo(t *testing.T) {
	conteudo := []byte("\"1\";\"UM\"\n\"2\";\"DOIS\"\n\"3\";\"TRES\"\n")
	l, err := AbrirZip(zipDeTeste(t, conteudo))
	if err != nil {
		t.Fatalf("AbrirZip: %v", err)
	}
	defer l.Close()

	f := NovaFonteDeLinhas(l, conversorSimples)

	var nomes []string
	for f.Next() {
		v, err := f.Values()
		if err != nil {
			t.Fatalf("Values: %v", err)
		}
		if len(v) != 2 {
			t.Fatalf("colunas = %d; quer 2", len(v))
		}
		nomes = append(nomes, v[1].(string))
	}
	if err := f.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	if len(nomes) != 3 {
		t.Fatalf("leu %v; quer 3 linhas", nomes)
	}
	if nomes[2] != "TRES" {
		t.Errorf("ultima = %q; quer TRES", nomes[2])
	}
}

// Linha malformada e PULADA, nao aborta: numa carga de 63 milhoes, uma linha
// ruim no meio nao pode derrubar horas de trabalho.
func TestFonteDeLinhasPulaMalformada(t *testing.T) {
	conteudo := []byte("\"1\";\"UM\"\n\"XX\";\"LIXO\"\n\"2\";\"DOIS\"\n\"3\"\n\"4\";\"QUATRO\"\n")
	l, err := AbrirZip(zipDeTeste(t, conteudo))
	if err != nil {
		t.Fatalf("AbrirZip: %v", err)
	}
	defer l.Close()

	f := NovaFonteDeLinhas(l, conversorSimples)

	var n int
	for f.Next() {
		if _, err := f.Values(); err != nil {
			t.Fatalf("Values: %v", err)
		}
		n++
	}
	if err := f.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	if n != 3 {
		t.Errorf("aceitou %d linhas; quer 3 (codigo nao-numerico e linha curta saem)", n)
	}
	if f.Ignoradas() != 2 {
		t.Errorf("Ignoradas = %d; quer 2", f.Ignoradas())
	}
}

// O pior modo de falha desta camada e a carga parcial silenciosa: se a leitura
// quebra no meio e o erro nao chega ao Err(), o CopyFrom termina "com sucesso"
// tendo gravado parte do arquivo, e ninguem percebe ate uma consulta vir curta.
// Corromper o stream deflate e o jeito de provocar isso de verdade — fechar o
// leitor nao serve, porque o encoding/csv ja bufferizou arquivos pequenos.
func TestFonteDeLinhasPropagaErroDeLeitura(t *testing.T) {
	// Um arquivo grande o bastante para nao caber no buffer do csv.
	var sb strings.Builder
	for i := 1; i <= 20000; i++ {
		fmt.Fprintf(&sb, "\"%d\";\"NOME %d\"\n", i, i)
	}
	caminho := zipDeTeste(t, []byte(sb.String()))

	// Corrompe o miolo do zip, depois do cabecalho.
	dados, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatalf("ler zip: %v", err)
	}
	meio := len(dados) / 2
	for i := meio; i < meio+64 && i < len(dados); i++ {
		dados[i] ^= 0xFF
	}
	if err := os.WriteFile(caminho, dados, 0o644); err != nil {
		t.Fatalf("regravar zip: %v", err)
	}

	l, err := AbrirZip(caminho)
	if err != nil {
		// Se o zip nem abre, a corrupcao pegou o cabecalho: nao e o que
		// queremos testar, mas tambem nao e falha do codigo sob teste.
		t.Skipf("corrupcao atingiu o cabecalho do zip: %v", err)
	}
	defer l.Close()

	f := NovaFonteDeLinhas(l, conversorSimples)

	var lidas int
	for f.Next() {
		if _, err := f.Values(); err != nil {
			t.Fatalf("Values: %v", err)
		}
		lidas++
	}

	if f.Err() == nil {
		t.Fatalf("leu %d linhas de 20000 e Err() ficou nil; erro de leitura foi "+
			"engolido — o CopyFrom terminaria 'com sucesso' com carga parcial", lidas)
	}
	if errors.Is(f.Err(), ErrLinhaInvalida) {
		t.Errorf("erro de leitura classificado como linha invalida: %v", f.Err())
	}
	if lidas >= 20000 {
		t.Errorf("leu %d linhas apesar da corrupcao", lidas)
	}
}

// Erro que NAO e ErrLinhaInvalida precisa abortar a carga, nao ser pulado:
// significa que algo esta errado com o arquivo ou o conversor, nao com
// uma linha isolada.
func TestFonteDeLinhasAbortaEmErroFatal(t *testing.T) {
	errFatal := errors.New("erro fatal do conversor")
	conv := func(campos []string) ([]any, error) {
		if campos[0] == "2" {
			return nil, fmt.Errorf("coluna 1: %w", errFatal)
		}
		return []any{campos[0]}, nil
	}

	conteudo := []byte("\"1\";\"UM\"\n\"2\";\"DOIS\"\n\"3\";\"TRES\"\n")
	l, err := AbrirZip(zipDeTeste(t, conteudo))
	if err != nil {
		t.Fatalf("AbrirZip: %v", err)
	}
	defer l.Close()

	f := NovaFonteDeLinhas(l, conv)

	var n int
	for f.Next() {
		n++
	}
	if n != 1 {
		t.Errorf("aceitou %d linhas; quer 1 (a segunda aborta)", n)
	}
	if f.Err() == nil {
		t.Fatal("Err() nil apos erro fatal do conversor")
	}
	if !errors.Is(f.Err(), errFatal) {
		t.Errorf("Err() = %v; devia envolver o erro original", f.Err())
	}
	if f.Ignoradas() != 0 {
		t.Errorf("Ignoradas = %d; erro fatal nao e linha ignorada", f.Ignoradas())
	}
}

// O conversor real embrulha o sentinela com contexto. Se o Next comparasse
// com == em vez de errors.Is, a linha seria tratada como erro fatal e
// abortaria a carga inteira por causa de um registro ruim.
func TestFonteDeLinhasAceitaSentinelaEmbrulhado(t *testing.T) {
	conv := func(campos []string) ([]any, error) {
		if campos[0] == "2" {
			return nil, fmt.Errorf("cnpj %q invalido: %w", campos[0], ErrLinhaInvalida)
		}
		return []any{campos[0]}, nil
	}

	conteudo := []byte("\"1\";\"UM\"\n\"2\";\"DOIS\"\n\"3\";\"TRES\"\n")
	l, err := AbrirZip(zipDeTeste(t, conteudo))
	if err != nil {
		t.Fatalf("AbrirZip: %v", err)
	}
	defer l.Close()

	f := NovaFonteDeLinhas(l, conv)

	var n int
	for f.Next() {
		n++
	}
	if err := f.Err(); err != nil {
		t.Fatalf("Err = %v; sentinela embrulhado devia so pular a linha", err)
	}
	if n != 2 {
		t.Errorf("aceitou %d; quer 2", n)
	}
	if f.Ignoradas() != 1 {
		t.Errorf("Ignoradas = %d; quer 1", f.Ignoradas())
	}
}

// Values() nao pode devolver o mesmo slice duas vezes: o pgx guarda a
// referencia enquanto monta o lote do COPY.
func TestFonteDeLinhasNaoReusaSlice(t *testing.T) {
	conteudo := []byte("\"1\";\"UM\"\n\"2\";\"DOIS\"\n")
	l, err := AbrirZip(zipDeTeste(t, conteudo))
	if err != nil {
		t.Fatalf("AbrirZip: %v", err)
	}
	defer l.Close()

	f := NovaFonteDeLinhas(l, conversorSimples)

	f.Next()
	primeira, _ := f.Values()
	guardada := primeira

	f.Next()
	if _, err := f.Values(); err != nil {
		t.Fatalf("Values: %v", err)
	}

	if guardada[1].(string) != "UM" {
		t.Errorf("a primeira linha virou %v ao ler a segunda; Values devolveu "+
			"o mesmo slice em vez de um novo", guardada)
	}
}
