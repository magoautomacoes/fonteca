package ingest

import (
	"errors"
	"fmt"
	"io"
)

// ErrLinhaInvalida indica linha que o conversor nao soube interpretar. Nao
// aborta a carga: a linha e contada e pulada.
var ErrLinhaInvalida = errors.New("linha malformada")

// Conversor traduz os campos crus de uma linha nas colunas do COPY.
// Devolver ErrLinhaInvalida faz a linha ser pulada; qualquer outro erro
// aborta a carga.
type Conversor func(campos []string) ([]any, error)

// FonteDeLinhas implementa pgx.CopyFromSource puxando do LeitorCSV uma linha
// por vez. E o que permite carregar 63 milhoes de linhas sem materializar
// nada: o COPY consome direto do decodificador.
type FonteDeLinhas struct {
	leitor      *LeitorCSV
	conv        Conversor
	atual       []any
	err         error
	ignoradas   int64
	descartadas int64
	lidas       int64
}

// NovaFonteDeLinhas monta a fonte. O leitor continua sendo do chamador:
// quem abriu, fecha.
func NovaFonteDeLinhas(l *LeitorCSV, conv Conversor) *FonteDeLinhas {
	return &FonteDeLinhas{leitor: l, conv: conv}
}

// Next avanca ate a proxima linha valida, pulando as malformadas.
func (f *FonteDeLinhas) Next() bool {
	for {
		campos, err := f.leitor.Proxima()
		if err == io.EOF {
			return false
		}
		if err != nil {
			f.err = fmt.Errorf("linha %d: %w", f.lidas+1, err)
			return false
		}
		f.lidas++

		valores, err := f.conv(campos)
		if err != nil {
			// Fora do filtro: a linha esta bem formada, so nao interessa.
			// Nao conta como ignorada, senao o aviso de "linhas malformadas"
			// dispararia em toda carga filtrada — sao 98% das linhas.
			if errors.Is(err, ErrForaDoFiltro) {
				f.descartadas++
				continue
			}
			if errors.Is(err, ErrLinhaInvalida) {
				f.ignoradas++
				continue
			}
			f.err = fmt.Errorf("converter linha %d: %w", f.lidas, err)
			return false
		}
		f.atual = valores
		return true
	}
}

// Values devolve as colunas da linha corrente. O conversor sempre aloca um
// slice novo, entao o pgx pode guardar a referencia sem risco.
func (f *FonteDeLinhas) Values() ([]any, error) {
	return f.atual, nil
}

// Err devolve o erro que interrompeu a leitura, se houve.
func (f *FonteDeLinhas) Err() error {
	return f.err
}

// Descartadas conta as linhas que o filtro rejeitou. Diferente de Ignoradas:
// estas estavam bem formadas, so nao interessavam ao recorte pedido.
func (f *FonteDeLinhas) Descartadas() int64 {
	return f.descartadas
}

// Ignoradas conta as linhas puladas por malformacao. Vale registrar no log:
// um numero alto significa que o layout do arquivo mudou.
func (f *FonteDeLinhas) Ignoradas() int64 {
	return f.ignoradas
}
