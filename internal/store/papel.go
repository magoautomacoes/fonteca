package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrPapelInseguro indica que o usuario conectado escapa da RLS de meta.uso.
var ErrPapelInseguro = errors.New("papel inseguro para servir a API")

// VerificarPapelSeguro recusa servir sob um usuario ao qual a RLS de meta.uso
// nao se aplica: superusuario, BYPASSRLS, ou dono de meta.uso — inclusive por
// ser membro do papel dono. pg_has_role com MEMBER (e nao USAGE) pega tambem o
// membro NOINHERIT, que nao herda o privilegio mas pode fazer SET ROLE para o
// dono a qualquer momento. Sob qualquer um deles a politica existe
// mas fica inerte, e o isolamento entre contas depende so do filtro na query.
// A API deve rodar como fonteca_api (deploy/papeis.sql).
func VerificarPapelSeguro(ctx context.Context, pool *pgxpool.Pool) error {
	var (
		usuario             string
		super, bypass, dono bool
	)
	err := pool.QueryRow(ctx, `
		SELECT r.rolname, r.rolsuper, r.rolbypassrls,
		       pg_has_role(current_user, c.relowner, 'MEMBER')
		FROM pg_roles r, pg_class c
		WHERE r.rolname = current_user
		  AND c.oid = 'meta.uso'::regclass`).
		Scan(&usuario, &super, &bypass, &dono)
	if err != nil {
		return fmt.Errorf("verificar papel do banco: %w", err)
	}

	switch {
	case super:
		return fmt.Errorf("%w: %q e superusuario", ErrPapelInseguro, usuario)
	case bypass:
		return fmt.Errorf("%w: %q tem BYPASSRLS", ErrPapelInseguro, usuario)
	case dono:
		return fmt.Errorf("%w: %q e dono de meta.uso", ErrPapelInseguro, usuario)
	}
	return nil
}
