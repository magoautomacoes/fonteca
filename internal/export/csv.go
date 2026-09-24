// Package export escreve resultados de busca em formatos de troca.
package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// EscreverCSV transforma o resultado da busca em CSV, em streaming: nada e
// acumulado, entao exportar mil ou um milhao de linhas custa a mesma memoria.
// Uma coluna NULL vira campo vazio, nunca a string literal "<nil>".
func EscreverCSV(w io.Writer, rows pgx.Rows) (int64, error) {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	descricoes := rows.FieldDescriptions()
	cabecalho := make([]string, len(descricoes))
	for i, d := range descricoes {
		cabecalho[i] = string(d.Name)
	}
	if err := cw.Write(cabecalho); err != nil {
		return 0, fmt.Errorf("escrever cabecalho: %w", err)
	}

	var n int64
	for rows.Next() {
		valores, err := rows.Values()
		if err != nil {
			return n, fmt.Errorf("ler linha %d: %w", n+1, err)
		}
		texto := make([]string, len(valores))
		for i, v := range valores {
			texto[i] = formatar(v)
		}
		if err := cw.Write(texto); err != nil {
			return n, fmt.Errorf("escrever linha %d: %w", n+1, err)
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return n, fmt.Errorf("iterar: %w", err)
	}
	cw.Flush()
	return n, cw.Error()
}

// formatar converte um valor do banco em texto de CSV. O %v cru nao serve:
// um numeric do pgx sai como "{30000000 -2 false finite true}" e um timestamp
// como "2026-08-01 00:00:00 +0000 UTC" — ambos inuteis numa lista que uma
// pessoa vai abrir no Excel.
func formatar(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case time.Time:
		// Data pura: a base nao tem hora util nesses campos.
		return t.Format("2006-01-02")
	case pgtype.Numeric:
		// Dinheiro com duas casas. Float64Value() e o acessor de pgx v5.11.
		if !t.Valid {
			return ""
		}
		f, err := t.Float64Value()
		if err != nil || !f.Valid {
			return ""
		}
		return strconv.FormatFloat(f.Float64, 'f', 2, 64)
	case []byte:
		return string(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}
