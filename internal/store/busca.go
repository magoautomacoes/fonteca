package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LimiteMaximo e o teto de linhas por consulta. E do servidor, nao do cliente:
// sem ele um filtro vazio tentaria devolver 63 milhoes de linhas.
const LimiteMaximo = 1000

// ErrFiltroInvalido marca os erros que vem da validacao do filtro, ou seja,
// erro do CLIENTE. Buscar tambem pode falhar por motivo de infraestrutura
// (LoteVigente sem conseguir consultar o banco, por exemplo) — esses NAO
// carregam este sentinela. E o unico jeito de quem chama Buscar distinguir
// "voce mandou um filtro ruim" de "o banco falhou", porque os dois voltam
// pelo mesmo tipo error.
var ErrFiltroInvalido = errors.New("filtro invalido")

var (
	padraoCNAE = regexp.MustCompile(`^\d{7}$`)
	padraoUF   = regexp.MustCompile(`^[A-Z]{2}$`)
)

// Filtro e a consulta-alvo parametrizada.
type Filtro struct {
	CNAEs       []string
	UFs         []string
	Situacao    int16
	UltimosDias int
	SoCelular   bool
	ExcluirMEI  bool
	Limite      int

	// AposData e AposCNPJ posicionam a busca depois de um resultado conhecido,
	// para paginar sem OFFSET — que degrada em tabela de 63 milhoes de linhas.
	// Juntos formam o cursor: a tupla (data_inicio, cnpj) que casa com o
	// ORDER BY de Buscar. AposCNPJ vazio ("") significa "sem cursor, comecar
	// do inicio"; AposData nil com AposCNPJ preenchido significa "o cursor
	// aponta para uma linha com data_inicio NULL".
	AposData *time.Time
	AposCNPJ string
}

// validar confere cada campo do filtro antes de qualquer consulta tocar o
// banco. CNAE e UF sao validados por formato aqui porque, mais abaixo, viram
// bind parameters normais — a validacao existe para recusar entrada invalida
// cedo, nao para defender contra injecao (isso e papel do bind parameter).
func (f *Filtro) validar() error {
	if len(f.CNAEs) == 0 {
		return fmt.Errorf("%w: informe ao menos um CNAE", ErrFiltroInvalido)
	}
	for _, c := range f.CNAEs {
		if !padraoCNAE.MatchString(c) {
			return fmt.Errorf("%w: CNAE invalido: %q (esperado 7 digitos)", ErrFiltroInvalido, c)
		}
	}
	for i, u := range f.UFs {
		f.UFs[i] = strings.ToUpper(strings.TrimSpace(u))
		if !padraoUF.MatchString(f.UFs[i]) {
			return fmt.Errorf("%w: UF invalida: %q", ErrFiltroInvalido, u)
		}
	}
	if f.Limite <= 0 || f.Limite > LimiteMaximo {
		f.Limite = LimiteMaximo
	}
	return nil
}

// Buscar roda a consulta-alvo no lote vigente: empresas nos CNAEs
// pedidos, abertas nos ultimos N dias, situacao ativa, com celular, nas
// UFs pedidas, excluindo optantes pelo MEI. O nome do schema e a UNICA
// coisa interpolada na string SQL, e vem de meta.lote via LoteVigente —
// nunca do usuario; todo o resto sao bind parameters ($1..$9).
//
// cnae_descricao e natureza_juridica vem das auxiliares por LEFT JOIN: lote
// importado antes delas existirem, ou codigo que a Receita nao descreve,
// devolve NULL nessas colunas em vez de sumir com a linha.
//
// Nota sobre o cursor (AposData/AposCNPJ, $8/$9): a comparacao de tupla
// "(data_inicio, cnpj) < ($8, $9)" casa com "ORDER BY data_inicio DESC,
// cnpj DESC" porque a ordenacao e decrescente nos dois campos — "depois" na
// ordenacao e "menor" no valor, para data_inicio E para cnpj. O cnpj entra
// no ORDER BY e na comparacao para desempatar linhas com o mesmo
// data_inicio: sem ele, linhas empatadas podem ser puladas ou repetidas
// entre paginas.
//
// data_inicio e NULLable (schema_lote.sql). NULL nao participa de
// comparacao de tupla ("NULL < qualquer coisa" e sempre desconhecido, nunca
// verdadeiro), entao a clausula trata os dois casos de cursor separadamente:
//   - cursor SEM data (a ultima linha da pagina anterior tinha data_inicio
//     NULL): so avanca dentro do bloco NULL, comparando cnpj < $9;
//   - cursor COM data: avanca tanto pelas linhas NULL (que vem depois de
//     qualquer data em NULLS LAST) quanto pelas linhas com data menor que a
//     do cursor, ou mesma data e cnpj menor.
//
// Sem essa separacao, um cursor com data nunca alcancaria as linhas NULL (a
// tupla "(NULL, cnpj) < (data, cnpj)" e sempre desconhecida) e um cursor sem
// data reiniciaria a busca do zero (mesma razao) — o cliente entraria em
// loop. Se a ordenacao mudar, o cursor tem que mudar junto.
func Buscar(ctx context.Context, pool *pgxpool.Pool, f Filtro) (pgx.Rows, error) {
	if err := f.validar(); err != nil {
		return nil, err
	}

	vigente, err := LoteVigente(ctx, pool)
	if err != nil {
		return nil, err
	}
	if vigente == nil {
		return nil, fmt.Errorf("nenhum lote importado ainda; rode `fonteca ingest`")
	}
	if err := ValidarSchema(vigente.Schema); err != nil {
		return nil, err
	}

	sql := fmt.Sprintf(`
		SELECT e.cnpj, m.razao_social, e.nome_fantasia,
		       e.ddd_1, e.telefone_1, e.email,
		       e.cnae_principal, ca.descricao AS cnae_descricao,
		       e.data_inicio, e.uf, mu.nome AS municipio,
		       m.capital_social, m.porte, na.descricao AS natureza_juridica
		FROM %s.estabelecimento e
		JOIN %s.empresa m USING (cnpj_basico)
		LEFT JOIN %s.simples s USING (cnpj_basico)
		LEFT JOIN %s.municipio mu ON mu.codigo = e.municipio
		LEFT JOIN %s.cnae ca ON ca.codigo = e.cnae_principal
		LEFT JOIN %s.natureza na ON na.codigo = m.natureza_juridica
		`+condicoesDoFiltro+`
		  AND ($9::text = '' OR
		       ($8::date IS NULL AND e.data_inicio IS NULL AND e.cnpj < $9::text) OR
		       ($8::date IS NOT NULL AND
		        (e.data_inicio IS NULL OR (e.data_inicio, e.cnpj) < ($8::date, $9::text))))
		ORDER BY e.data_inicio DESC NULLS LAST, e.cnpj DESC
		LIMIT $7`,
		vigente.Schema, vigente.Schema, vigente.Schema,
		vigente.Schema, vigente.Schema, vigente.Schema)

	var aposData any
	if f.AposData != nil {
		aposData = *f.AposData
	}
	args := append(f.argumentosDoFiltro(), f.Limite, aposData, f.AposCNPJ)
	return pool.Query(ctx, sql, args...)
}

// condicoesDoFiltro e o WHERE da consulta-alvo ($1..$6), compartilhado por
// Buscar e ContarPorUF: a contagem por estado tem de contar exatamente as
// empresas que a busca devolveria, senao o mapa promete o que a lista nao
// entrega. Supoe os aliases e (estabelecimento) e s (simples).
const condicoesDoFiltro = `WHERE e.cnae_principal = ANY($1)
		  AND e.situacao = $2
		  AND ($3::text[] IS NULL OR e.uf = ANY($3))
		  AND ($4 = 0 OR e.data_inicio >= CURRENT_DATE - $4::int)
		  AND (NOT $5 OR e.celular)
		  AND (NOT $6 OR COALESCE(s.opcao_mei, false) = false)`

// argumentosDoFiltro sao os binds $1..$6 de condicoesDoFiltro, na ordem.
func (f *Filtro) argumentosDoFiltro() []any {
	var ufs any
	if len(f.UFs) > 0 {
		ufs = f.UFs
	}
	// Situacao zero (zero-value do Filtro, quando o chamador nao informa)
	// e forcada para ATIVA. Isso significa que hoje nao ha como pedir
	// "qualquer situacao" atraves de Filtro.Situacao — um futuro chamador
	// que precise disso vai ter que adicionar um valor sentinela distinto.
	situacao := f.Situacao
	if situacao == 0 {
		situacao = SituacaoAtiva
	}
	return []any{f.CNAEs, situacao, ufs, f.UltimosDias, f.SoCelular, f.ExcluirMEI}
}

// ContarPorUF conta, por estado, as empresas que Buscar devolveria com o
// mesmo filtro (sem limite nem cursor). E o que acende o mapa do radar.
// Estado sem nenhuma empresa nao aparece no mapa devolvido.
func ContarPorUF(ctx context.Context, pool *pgxpool.Pool, f Filtro) (map[string]int64, error) {
	if err := f.validar(); err != nil {
		return nil, err
	}
	vigente, err := LoteVigente(ctx, pool)
	if err != nil {
		return nil, err
	}
	if vigente == nil {
		return nil, fmt.Errorf("nenhum lote importado ainda; rode `fonteca ingest`")
	}
	if err := ValidarSchema(vigente.Schema); err != nil {
		return nil, err
	}

	sql := fmt.Sprintf(`
		SELECT e.uf, count(*)
		FROM %s.estabelecimento e
		LEFT JOIN %s.simples s USING (cnpj_basico)
		`+condicoesDoFiltro+`
		GROUP BY e.uf`,
		vigente.Schema, vigente.Schema)

	rows, err := pool.Query(ctx, sql, f.argumentosDoFiltro()...)
	if err != nil {
		return nil, fmt.Errorf("contar por UF: %w", err)
	}
	defer rows.Close()

	porUF := map[string]int64{}
	for rows.Next() {
		var uf *string
		var n int64
		if err := rows.Scan(&uf, &n); err != nil {
			return nil, fmt.Errorf("ler contagem: %w", err)
		}
		if uf != nil {
			porUF[*uf] = n
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ler contagem: %w", err)
	}
	return porUF, nil
}
