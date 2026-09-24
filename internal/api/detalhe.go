package api

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/magoautomacoes/fonteca/internal/store"
	"github.com/magoautomacoes/fonteca/internal/tenant"
)

var padraoCNPJ = regexp.MustCompile(`^\d{14}$`)

// Detalhe traz empresa, estabelecimento e Simples. NAO traz socios: a tabela
// fica vazia por decisao registrada no docs/arquitetura.md (LGPD, e o dado envelhece).
type Detalhe struct {
	CNPJ          string  `json:"cnpj"`
	RazaoSocial   string  `json:"razao_social"`
	NomeFantasia  string  `json:"nome_fantasia,omitempty"`
	Situacao      string  `json:"situacao_cadastral"`
	CNAEPrincipal string  `json:"cnae_principal"`
	CNAEDescricao string  `json:"cnae_descricao,omitempty"`
	UF            string  `json:"uf"`
	Municipio     string  `json:"municipio,omitempty"`
	Telefone      string  `json:"telefone,omitempty"`
	WhatsApp      string  `json:"whatsapp,omitempty"`
	Email         string  `json:"email,omitempty"`
	CapitalSocial float64 `json:"capital_social"`
	Porte         string  `json:"porte,omitempty"`
	Natureza      string  `json:"natureza_juridica,omitempty"`
	OptanteMEI    bool    `json:"optante_mei"`
}

func (s *servidor) detalhe(w http.ResponseWriter, r *http.Request, conta *store.Conta) {
	cnpj := r.PathValue("cnpj")
	if !padraoCNPJ.MatchString(cnpj) {
		responderErro(w, http.StatusBadRequest, "cnpj deve ter 14 digitos, sem pontuacao")
		return
	}

	vigente, err := store.LoteVigente(r.Context(), s.pool)
	if err != nil {
		responderErroInterno(w, s.log, err)
		return
	}
	if vigente == nil {
		responderErro(w, http.StatusServiceUnavailable, "nenhum lote importado ainda")
		return
	}
	if err := store.ValidarSchema(vigente.Schema); err != nil {
		responderErroInterno(w, s.log, err)
		return
	}

	// O schema e a UNICA coisa interpolada; $1 e bind parameter (docs/arquitetura.md).
	sql := fmt.Sprintf(`
		SELECT e.cnpj, m.razao_social, e.nome_fantasia, e.situacao,
		       e.cnae_principal, ca.descricao, e.uf, mu.nome,
		       e.ddd_1, e.telefone_1, e.email,
		       m.capital_social, m.porte, na.descricao,
		       COALESCE(s.opcao_mei, false)
		FROM %s.estabelecimento e
		JOIN %s.empresa m USING (cnpj_basico)
		LEFT JOIN %s.simples s USING (cnpj_basico)
		LEFT JOIN %s.municipio mu ON mu.codigo = e.municipio
		LEFT JOIN %s.cnae ca ON ca.codigo = e.cnae_principal
		LEFT JOIN %s.natureza na ON na.codigo = m.natureza_juridica
		WHERE e.cnpj = $1`,
		vigente.Schema, vigente.Schema, vigente.Schema,
		vigente.Schema, vigente.Schema, vigente.Schema)

	var (
		d               Detalhe
		situacao        int16
		fantasia, munic *string
		ddd, tel, email *string
		porte           *string
		cnaeDesc, natur *string
	)
	err = s.pool.QueryRow(r.Context(), sql, cnpj).Scan(
		&d.CNPJ, &d.RazaoSocial, &fantasia, &situacao,
		&d.CNAEPrincipal, &cnaeDesc, &d.UF, &munic,
		&ddd, &tel, &email,
		&d.CapitalSocial, &porte, &natur, &d.OptanteMEI)
	if errors.Is(err, pgx.ErrNoRows) {
		responderErro(w, http.StatusNotFound, "cnpj nao encontrado no lote vigente")
		return
	}
	if err != nil {
		responderErroInterno(w, s.log, err)
		return
	}

	d.NomeFantasia = valor(fantasia)
	d.Municipio = valor(munic)
	d.Email = valor(email)
	d.Porte = valor(porte)
	d.CNAEDescricao = valor(cnaeDesc)
	d.Natureza = valor(natur)
	d.Situacao = store.NomeDaSituacao(situacao)
	d.Telefone, d.WhatsApp = montarTelefones(valor(ddd), valor(tel))

	if err := tenant.RegistrarUso(r.Context(), s.pool, conta.ID, 1); err != nil {
		s.log.Error("registrar uso", "conta_id", conta.ID, "erro", err)
	}

	responderJSON(w, http.StatusOK, d)
}
