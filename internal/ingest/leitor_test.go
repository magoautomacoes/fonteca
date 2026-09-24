package ingest

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// zipDeTeste cria um ZIP com um CSV em Latin-1, como a Receita publica.
// O nome interno NAO tem extensao .csv — e assim no arquivo real.
func zipDeTeste(t *testing.T, linhasLatin1 []byte) string {
	t.Helper()
	caminho := filepath.Join(t.TempDir(), "Teste.zip")
	arq, err := os.Create(caminho)
	if err != nil {
		t.Fatalf("criar zip: %v", err)
	}
	defer arq.Close()

	w := zip.NewWriter(arq)
	f, err := w.Create("F.K03200$Z.D60912.MUNICCSV")
	if err != nil {
		t.Fatalf("criar entrada: %v", err)
	}
	if _, err := f.Write(linhasLatin1); err != nil {
		t.Fatalf("escrever: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("fechar zip: %v", err)
	}
	return caminho
}

func TestLeitorDecodificaLatin1(t *testing.T) {
	// "GUAJARA-MIRIM" e "SAO JOSE DOS CAMPOS" com acentos em Latin-1:
	// 0xC7 = C-cedilha, 0xC3 = A-til, 0xC9 = E-agudo.
	conteudo := []byte("\"0001\";\"GUAJAR\xc1-MIRIM\"\n\"0002\";\"SAO JOS\xc9\"\n")
	caminho := zipDeTeste(t, conteudo)

	l, err := AbrirZip(caminho)
	if err != nil {
		t.Fatalf("AbrirZip: %v", err)
	}
	defer l.Close()

	primeira, err := l.Proxima()
	if err != nil {
		t.Fatalf("primeira linha: %v", err)
	}
	if len(primeira) != 2 {
		t.Fatalf("campos = %d; quer 2", len(primeira))
	}
	if primeira[1] != "GUAJARÁ-MIRIM" {
		t.Errorf("nome = %q; quer %q (0xC1 devia virar A-agudo)", primeira[1], "GUAJARÁ-MIRIM")
	}

	segunda, err := l.Proxima()
	if err != nil {
		t.Fatalf("segunda linha: %v", err)
	}
	if segunda[1] != "SAO JOSÉ" {
		t.Errorf("nome = %q; quer %q", segunda[1], "SAO JOSÉ")
	}

	if _, err := l.Proxima(); err != io.EOF {
		t.Errorf("fim = %v; quer io.EOF", err)
	}
}

// A armadilha que fez gente perder milhoes de registros: linha em branco no
// meio do arquivo NAO pode ser interpretada como fim.
func TestLeitorIgnoraLinhaEmBranco(t *testing.T) {
	conteudo := []byte("\"0001\";\"PRIMEIRA\"\n\n\"0002\";\"SEGUNDA\"\n\n\n\"0003\";\"TERCEIRA\"\n")
	l, err := AbrirZip(zipDeTeste(t, conteudo))
	if err != nil {
		t.Fatalf("AbrirZip: %v", err)
	}
	defer l.Close()

	var nomes []string
	for {
		campos, err := l.Proxima()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("ler: %v", err)
		}
		nomes = append(nomes, campos[1])
	}
	if len(nomes) != 3 {
		t.Fatalf("leu %d linhas (%v); quer 3 — linha em branco nao pode truncar", len(nomes), nomes)
	}
	if nomes[2] != "TERCEIRA" {
		t.Errorf("ultima = %q; quer TERCEIRA", nomes[2])
	}
}

// encoding/csv ja pula linha totalmente vazia sozinho. O que ele NAO pula e
// linha so com espacos, e linha com campos vazios (";;"). Sem o guarda de
// linhaVazia elas viram registros de lixo no meio da carga.
func TestLeitorIgnoraLinhaSoComEspacos(t *testing.T) {
	conteudo := []byte("\"0001\";\"PRIMEIRA\"\n   \n\"0002\";\"SEGUNDA\"\n;\n\"0003\";\"TERCEIRA\"\n")
	l, err := AbrirZip(zipDeTeste(t, conteudo))
	if err != nil {
		t.Fatalf("AbrirZip: %v", err)
	}
	defer l.Close()

	var nomes []string
	for {
		campos, err := l.Proxima()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("ler: %v", err)
		}
		if len(campos) > 1 {
			nomes = append(nomes, campos[1])
		} else {
			t.Errorf("linha de lixo passou pelo filtro: %q", campos)
		}
	}
	if len(nomes) != 3 {
		t.Fatalf("leu %d linhas (%v); quer 3", len(nomes), nomes)
	}
}

// ; dentro de campo entre aspas pertence ao campo, nao e separador.
func TestLeitorRespeitaPontoEVirgulaEmCampo(t *testing.T) {
	conteudo := []byte("\"0001\";\"COMERCIO DE A; B; C\"\n")
	l, err := AbrirZip(zipDeTeste(t, conteudo))
	if err != nil {
		t.Fatalf("AbrirZip: %v", err)
	}
	defer l.Close()

	campos, err := l.Proxima()
	if err != nil {
		t.Fatalf("ler: %v", err)
	}
	if len(campos) != 2 {
		t.Fatalf("campos = %d (%v); quer 2 — o ; dentro das aspas nao separa", len(campos), campos)
	}
	if campos[1] != "COMERCIO DE A; B; C" {
		t.Errorf("campo = %q; o conteudo com ; devia vir inteiro", campos[1])
	}
}

func TestSanitizarRemoveBytesC1(t *testing.T) {
	// 0x8F em Latin-1 vira U+008F, um controle C1 invisivel. Achado 1.2 da
	// analise: decodifica sem erro, nao aparece na tela, e entra no banco.
	// O NUL vem junto porque a base tambem tem bytes zero embutidos.
	entrada := "JOSESILVA\x00"
	got := Sanitizar(entrada)
	if got != "JOSESILVA" {
		t.Errorf("Sanitizar = %q; quer %q (C1 e NUL removidos)", got, "JOSESILVA")
	}

	// As bordas da faixa C1, para que uma mudanca de limite seja pega.
	for _, r := range []rune{'', '', ''} {
		if got := Sanitizar(string(r)); got != "" {
			t.Errorf("Sanitizar(U+%04X) = %q; quer vazio", r, got)
		}
	}

	// Fora da faixa C1 nao se mexe: acento legitimo e nbsp permanecem.
	if got := Sanitizar("JOSÉ AÇÚCAR"); got != "JOSÉ AÇÚCAR" {
		t.Errorf("Sanitizar removeu acento valido: %q", got)
	}
	if got := Sanitizar("A B"); got != "A B" {
		t.Errorf("Sanitizar removeu U+00A0, que esta fora da faixa C1: %q", got)
	}
}

// csv.Reader com ReuseRecord=true sobrescreve o slice a cada Read. Proxima
// precisa copiar os campos, senao a linha ja devolvida muda sozinha quando a
// proxima e lida — e o erro so aparece em carga grande, nunca em teste manual.
func TestLeitorCopiaCamposEntreLinhas(t *testing.T) {
	conteudo := []byte("\"AAA\";\"111\"\n\"BBB\";\"222\"\n")
	l, err := AbrirZip(zipDeTeste(t, conteudo))
	if err != nil {
		t.Fatalf("AbrirZip: %v", err)
	}
	defer l.Close()

	primeira, err := l.Proxima()
	if err != nil {
		t.Fatalf("primeira: %v", err)
	}
	guardada := primeira // guardar a referencia, como um chamador faria

	if _, err := l.Proxima(); err != nil {
		t.Fatalf("segunda: %v", err)
	}

	if guardada[0] != "AAA" || guardada[1] != "111" {
		t.Errorf("a primeira linha virou %v depois de ler a segunda; "+
			"Proxima devolveu o buffer reaproveitado em vez de uma copia", guardada)
	}
}

// O byte 0x8F passa pelo decodificador Latin-1 como U+008F (controle C1
// invisivel) e e removido pelo Sanitizar. Este teste existe para fixar a
// ESCOLHA DO CODEC: com Windows-1252, os bytes 0x80-0x9F viram caracteres
// visiveis fora da faixa C1 (€, ", Ÿ) que o Sanitizar preservaria, e o lixo
// entraria no banco.
func TestLeitorRemoveC1VindoDoArquivo(t *testing.T) {
	// 0xC1 = A-agudo (acento legitimo, deve sobreviver)
	// 0x8F = indefinido em Latin-1 (deve sumir)
	conteudo := []byte("\"0001\";\"GUAJAR\xc1\x8f-MIRIM\"\n")
	l, err := AbrirZip(zipDeTeste(t, conteudo))
	if err != nil {
		t.Fatalf("AbrirZip: %v", err)
	}
	defer l.Close()

	campos, err := l.Proxima()
	if err != nil {
		t.Fatalf("ler: %v", err)
	}
	if campos[1] != "GUAJARÁ-MIRIM" {
		t.Errorf("nome = %q; quer %q — o acento deve sobreviver e o 0x8F sumir",
			campos[1], "GUAJARÁ-MIRIM")
	}
	for _, r := range campos[1] {
		if r >= 0x80 && r <= 0x9F {
			t.Errorf("sobrou um controle C1 U+%04X no campo", r)
		}
		if r == '€' || r == '“' || r == 'Ÿ' {
			t.Errorf("caractere U+%04X indica decodificacao Windows-1252, "+
				"nao Latin-1", r)
		}
	}
}

func TestParsearData(t *testing.T) {
	casos := []struct {
		entrada string
		nulo    bool
		erro    bool
	}{
		{"20260715", false, false},
		{"", true, false},
		{"0", true, false},
		{"00000000", true, false},
		{"20260230", false, true}, // 30 de fevereiro nao existe
		{"20261301", false, true}, // mes 13
		{"18991231", false, true}, // antes de 1900
		{"abcdefgh", false, true},
	}
	for _, c := range casos {
		got, err := ParsearData(c.entrada)
		if c.erro {
			if err == nil {
				t.Errorf("ParsearData(%q) devia falhar, devolveu %v", c.entrada, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParsearData(%q) erro: %v", c.entrada, err)
			continue
		}
		if c.nulo && got != nil {
			t.Errorf("ParsearData(%q) = %v; quer nil", c.entrada, got)
		}
		if !c.nulo && got == nil {
			t.Errorf("ParsearData(%q) = nil; queria uma data", c.entrada)
		}
	}
}

func TestParsearDecimal(t *testing.T) {
	casos := []struct {
		entrada string
		quer    string
	}{
		{"1000,00", "100000e-2"},
		{"50000,50", "5000050e-2"},
		{"50000,5", "500005e-1"},
		{"0,00", "0e-2"},
		{"", "0"},
		{"1000", "1000"},
		// 16 digitos inteiros com centavos: float64 devolveria ...456,75.
		{"1234567890123456,78", "123456789012345678e-2"},
	}
	for _, c := range casos {
		got, err := ParsearDecimal(c.entrada)
		if err != nil {
			t.Errorf("ParsearDecimal(%q) erro: %v", c.entrada, err)
			continue
		}
		if s := got.Int.String() + expoente(got.Exp); s != c.quer {
			t.Errorf("ParsearDecimal(%q) = %s; quer %s", c.entrada, s, c.quer)
		}
	}

	// Inf/NaN envenenariam somas; sinal, milhar e mais de duas casas nao sao
	// o formato da Receita; 17 digitos inteiros estouram numeric(18,2) e
	// abortariam o COPY inteiro.
	for _, ruim := range []string{"Inf", "-Inf", "NaN", "inf", "-10,00", "1.000,00",
		"10,123", "1e5", "12345678901234567,00"} {
		if _, err := ParsearDecimal(ruim); err == nil {
			t.Errorf("ParsearDecimal(%q) devia falhar", ruim)
		}
	}
}

func expoente(e int32) string {
	if e == 0 {
		return ""
	}
	return fmt.Sprintf("e%d", e)
}
