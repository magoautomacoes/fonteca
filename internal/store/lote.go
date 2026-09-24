package store

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema_lote.sql
var schemaLote string

//go:embed indices_lote.sql
var indicesLote string

// schemaValido e a unica porta de entrada para nome de schema. Identificador
// nao pode ser parametrizado em SQL, entao a validacao substitui o prepared
// statement como defesa contra injecao.
var schemaValido = regexp.MustCompile(`^lote_\d{4}_\d{2}$`)

var referenciaValida = regexp.MustCompile(`^(\d{4})-(\d{2})$`)

// NomeDoSchema converte a referencia do lote ("2026-09") no nome do schema
// ("lote_2026_09"), recusando qualquer coisa fora do formato.
func NomeDoSchema(referencia string) (string, error) {
	m := referenciaValida.FindStringSubmatch(referencia)
	if m == nil {
		return "", fmt.Errorf("referencia invalida: %q (esperado AAAA-MM)", referencia)
	}
	mes := m[2]
	if mes < "01" || mes > "12" {
		return "", fmt.Errorf("mes invalido em %q", referencia)
	}
	return fmt.Sprintf("lote_%s_%s", m[1], mes), nil
}

func validarSchema(schema string) error {
	if !schemaValido.MatchString(schema) {
		return fmt.Errorf("nome de schema invalido: %q (esperado lote_AAAA_MM)", schema)
	}
	return nil
}

// ValidarSchema confirma que o nome segue o padrao lote_AAAA_MM. Exportada para
// que quem consulta possa validar um nome de schema antes de interpola-lo em SQL:
// identificador nao pode ser bind parameter, entao a validacao e a unica defesa.
func ValidarSchema(schema string) error {
	return validarSchema(schema)
}

// CriarSchemaDeLote cria o schema e suas tabelas, sem indices.
// Os indices vem depois da carga (CriarIndices).
func CriarSchemaDeLote(ctx context.Context, pool *pgxpool.Pool, schema string) error {
	if err := validarSchema(schema); err != nil {
		return err
	}
	sql := strings.ReplaceAll(schemaLote, "{{schema}}", schema)
	if _, err := pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("criar schema %s: %w", schema, err)
	}
	if err := concederUsoDoSchema(ctx, pool, schema); err != nil {
		return err
	}
	return nil
}

// concederUsoDoSchema da USAGE no schema recem-criado, mais SELECT em todas
// as suas tabelas, para fonteca_api e fonteca_leitura.
//
// USAGE precisa ser feito aqui, e nao em deploy/papeis.sql, porque o
// Postgres NAO tem ALTER DEFAULT PRIVILEGES para USAGE em schema futuro — so
// para objetos (tabelas, sequences, etc) dentro de um schema que ja existe.
// Sem este GRANT, a proxima ingestao criaria um schema que fonteca_api
// enxerga via SELECT nas tabelas mas nao alcanca, porque o acesso barra na
// porta do schema (SQLSTATE 42501, "permission denied for schema") antes
// mesmo de checar privilegio de tabela.
//
// O GRANT SELECT nas tabelas e emitido aqui tambem, explicitamente, em vez
// de confiar so em ALTER DEFAULT PRIVILEGES FOR ROLE fonteca_ingest (que
// deploy/papeis.sql define): default privilege so se aplica quando quem CRIA
// o objeto e exatamente o role nomeado no ALTER. Em teste, quem cria o
// schema e o dono do banco, nao fonteca_ingest — e mesmo em producao seria
// fragil depender de que CriarSchemaDeLote va sempre rodar autenticado como
// fonteca_ingest. O GRANT explicito funciona nos dois casos.
//
// Os papeis podem nao existir: em desenvolvimento, docker-compose.dev.yml
// sobe o banco com um unico usuario, e deploy/papeis.sql so e aplicado no
// deploy. Por isso checamos pg_roles antes de cada GRANT em vez de tentar e
// ignorar o erro — silenciar o erro esconderia tambem uma falha real (por
// exemplo, nome de role digitado errado).
func concederUsoDoSchema(ctx context.Context, pool *pgxpool.Pool, schema string) error {
	// schema ja foi validado por validarSchema em CriarSchemaDeLote; nome de
	// role e literal fixo nosso, nunca vem de entrada externa.
	for _, role := range []string{"fonteca_api", "fonteca_leitura"} {
		var existe bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, role).
			Scan(&existe); err != nil {
			return fmt.Errorf("verificar role %s: %w", role, err)
		}
		if !existe {
			slog.Info("role de acesso restrito ainda nao existe; pulando grant de schema (normal em desenvolvimento)",
				"role", role, "schema", schema)
			continue
		}
		if _, err := pool.Exec(ctx,
			fmt.Sprintf("GRANT USAGE ON SCHEMA %s TO %s", schema, role)); err != nil {
			return fmt.Errorf("conceder USAGE em %s para %s: %w", schema, role, err)
		}
		if _, err := pool.Exec(ctx,
			fmt.Sprintf("GRANT SELECT ON ALL TABLES IN SCHEMA %s TO %s", schema, role)); err != nil {
			return fmt.Errorf("conceder SELECT nas tabelas de %s para %s: %w", schema, role, err)
		}
	}
	return nil
}

// CriarIndices indexa e roda ANALYZE. Chamar apenas apos a carga dos dados.
func CriarIndices(ctx context.Context, pool *pgxpool.Pool, schema string) error {
	if err := validarSchema(schema); err != nil {
		return err
	}
	sql := strings.ReplaceAll(indicesLote, "{{schema}}", schema)
	if _, err := pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("criar indices em %s: %w", schema, err)
	}
	return nil
}

// TrocarLoteVigente aponta o lote novo como vigente numa unica transacao.
// Ate o COMMIT, quem consulta continua vendo o lote anterior.
func TrocarLoteVigente(ctx context.Context, pool *pgxpool.Pool, schema, referencia string, linhas int64) error {
	if err := validarSchema(schema); err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abrir transacao: %w", err)
	}
	defer tx.Rollback(ctx) // no-op depois do commit

	if _, err := tx.Exec(ctx, `UPDATE meta.lote SET vigente = false WHERE vigente`); err != nil {
		return fmt.Errorf("desmarcar lote anterior: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO meta.lote (schema, referencia, vigente, linhas, importado_em)
		VALUES ($1, $2, true, $3, now())
		ON CONFLICT (schema) DO UPDATE
		SET vigente = true, linhas = EXCLUDED.linhas, importado_em = now()`,
		schema, referencia, linhas)
	if err != nil {
		return fmt.Errorf("registrar lote %s: %w", schema, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmar troca: %w", err)
	}
	return nil
}

// LoteVigente devolve o lote em uso, ou (nil, nil) se nada foi importado ainda.
func LoteVigente(ctx context.Context, pool *pgxpool.Pool) (*Lote, error) {
	var l Lote
	err := pool.QueryRow(ctx, `
		SELECT schema, referencia, vigente, linhas, importado_em
		FROM meta.lote WHERE vigente`).
		Scan(&l.Schema, &l.Referencia, &l.Vigente, &l.Linhas, &l.ImportadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ler lote vigente: %w", err)
	}
	return &l, nil
}

// DescartarSchema apaga um schema de lote. A validacao de nome e o que
// impede apagar 'public' ou 'meta' por engano ou por entrada maliciosa.
func DescartarSchema(ctx context.Context, pool *pgxpool.Pool, schema string) error {
	if err := validarSchema(schema); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema)); err != nil {
		return fmt.Errorf("descartar schema %s: %w", schema, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM meta.lote WHERE schema = $1`, schema); err != nil {
		return fmt.Errorf("remover registro do lote %s: %w", schema, err)
	}
	return nil
}
