// Package ingest transforma os arquivos da Receita em linhas no Postgres.
package ingest

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/text/encoding/charmap"
)

// LeitorCSV le um CSV da Receita de dentro de um ZIP, sem materializar nada:
// o conteudo vai do ZIP para o parser em streaming. E o que mantem a RAM em
// dezenas de MB mesmo no arquivo de 2,1 GB.
type LeitorCSV struct {
	zip    *zip.ReadCloser
	csv    *csv.Reader
	fechar io.Closer
}

// AbrirZip abre o primeiro arquivo de dados dentro do ZIP. Os nomes internos
// nao tem extensao .csv — sao do tipo F.K03200$Z.D60912.MUNICCSV.
func AbrirZip(caminho string) (*LeitorCSV, error) {
	zr, err := zip.OpenReader(caminho)
	if err != nil {
		return nil, fmt.Errorf("abrir zip %s: %w", caminho, err)
	}

	var entrada *zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		entrada = f
		break
	}
	if entrada == nil {
		zr.Close()
		return nil, fmt.Errorf("zip %s nao contem arquivo de dados", caminho)
	}

	rc, err := entrada.Open()
	if err != nil {
		zr.Close()
		return nil, fmt.Errorf("abrir %s dentro do zip: %w", entrada.Name, err)
	}

	// Latin-1 para UTF-8 no proprio stream.
	decodificado := charmap.ISO8859_1.NewDecoder().Reader(rc)

	r := csv.NewReader(decodificado)
	r.Comma = ';'
	r.LazyQuotes = true    // a base tem aspas soltas em alguns campos
	r.FieldsPerRecord = -1 // nao exigir numero fixo de colunas
	r.ReuseRecord = true   // evita alocar um slice por linha em 63 milhoes de linhas

	return &LeitorCSV{zip: zr, csv: r, fechar: rc}, nil
}

// Proxima devolve a proxima linha com campos ja sanitizados, ou io.EOF no fim.
// Linhas em branco sao PULADAS, nunca tratadas como fim de arquivo: interpretar
// linha vazia como EOF ja truncou a carga de muita gente na comunidade.
func (l *LeitorCSV) Proxima() ([]string, error) {
	for {
		campos, err := l.csv.Read()
		if err == io.EOF {
			return nil, io.EOF
		}
		if err != nil {
			return nil, fmt.Errorf("ler linha: %w", err)
		}
		if linhaVazia(campos) {
			continue
		}
		limpos := make([]string, len(campos))
		for i, c := range campos {
			limpos[i] = Sanitizar(c)
		}
		return limpos, nil
	}
}

func linhaVazia(campos []string) bool {
	for _, c := range campos {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// Close libera o ZIP e o arquivo interno.
func (l *LeitorCSV) Close() error {
	if l.fechar != nil {
		l.fechar.Close()
	}
	if l.zip != nil {
		return l.zip.Close()
	}
	return nil
}

// Sanitizar remove NUL e a faixa de controle C1 (U+0080 a U+009F). A base da
// Receita contem bytes que nenhuma codificacao Latin representa como caractere
// (0x8F, por exemplo): decodificam sem erro, viram controles invisiveis e
// entram no banco. Acentos legitimos nao sao afetados.
func Sanitizar(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == 0 || (r >= 0x80 && r <= 0x9F) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// ParsearData converte AAAAMMDD em data. "", "0" e "00000000" viram nil.
// A validacao de faixa e necessaria porque time.Parse ACEITA 20260230 e
// normaliza silenciosamente para 2 de marco.
func ParsearData(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" || s == "00000000" {
		return nil, nil
	}
	if len(s) != 8 {
		return nil, fmt.Errorf("data invalida: %q (esperado AAAAMMDD)", s)
	}

	ano, err1 := strconv.Atoi(s[0:4])
	mes, err2 := strconv.Atoi(s[4:6])
	dia, err3 := strconv.Atoi(s[6:8])
	if err1 != nil || err2 != nil || err3 != nil {
		return nil, fmt.Errorf("data nao numerica: %q", s)
	}
	if ano < 1900 || mes < 1 || mes > 12 || dia < 1 || dia > 31 {
		return nil, fmt.Errorf("data fora de faixa: %q", s)
	}

	t := time.Date(ano, time.Month(mes), dia, 0, 0, 0, 0, time.UTC)
	// Round-trip: se time normalizou (30/02 -> 02/03), a data era invalida.
	if t.Year() != ano || int(t.Month()) != mes || t.Day() != dia {
		return nil, fmt.Errorf("data inexistente: %q", s)
	}
	return &t, nil
}

// decimalDaReceita e o formato do capital social: digitos, virgula opcional e
// ate duas casas. Sem sinal, sem milhar, sem notacao cientifica.
var decimalDaReceita = regexp.MustCompile(`^(\d+)(?:,(\d{1,2}))?$`)

// maxDigitosInteiros e o que cabe em numeric(18,2): 16 antes da virgula. Um
// valor maior nao pode chegar ao COPY — la ele aborta a carga inteira com
// "numeric field overflow", e nao so a linha.
const maxDigitosInteiros = 16

// ParsearDecimal converte o formato da Receita (virgula decimal) direto em
// numeric, SEM passar por float64: float64 guarda so ~15 digitos
// significativos, e numeric(18,2) guarda 18 — um capital de 16 digitos
// perderia os centavos no caminho.
func ParsearDecimal(s string) (pgtype.Numeric, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Numeric{Int: big.NewInt(0), Valid: true}, nil
	}
	m := decimalDaReceita.FindStringSubmatch(s)
	if m == nil {
		return pgtype.Numeric{}, fmt.Errorf("decimal invalido: %q", s)
	}
	inteiro := strings.TrimLeft(m[1], "0")
	if len(inteiro) > maxDigitosInteiros {
		return pgtype.Numeric{}, fmt.Errorf("decimal fora de faixa: %q", s)
	}
	casas := m[2]
	valor, ok := new(big.Int).SetString(m[1]+casas, 10)
	if !ok {
		return pgtype.Numeric{}, fmt.Errorf("decimal invalido: %q", s)
	}
	return pgtype.Numeric{Int: valor, Exp: -int32(len(casas)), Valid: true}, nil
}
