package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/magoautomacoes/fonteca/internal/store"
	"github.com/magoautomacoes/fonteca/internal/tenant"
)

// corpoMax limita o JSON de entrada. Sem teto, um corpo gigante vira memoria.
const corpoMax = 64 << 10 // 64 KB

// PedidoPesquisa segue o formato comum das APIs de CNPJ do mercado, para que migrar
// seja trocar so a URL do no.
type PedidoPesquisa struct {
	CNAEs    []string `json:"codigo_atividade_principal"`
	Situacao string   `json:"situacao_cadastral"`
	UFs      []string `json:"uf"`
	Abertura struct {
		UltimosDias int `json:"ultimos_dias"`
	} `json:"data_abertura"`
	MEI struct {
		ExcluirOptante bool `json:"excluir_optante"`
	} `json:"mei"`
	MaisFiltros struct {
		SomenteCelular bool `json:"somente_celular"`
	} `json:"mais_filtros"`
	Limite int    `json:"limite"`
	Cursor string `json:"cursor"`
}

// Lead e uma linha de resultado.
type Lead struct {
	CNPJ          string     `json:"cnpj"`
	RazaoSocial   string     `json:"razao_social"`
	NomeFantasia  string     `json:"nome_fantasia,omitempty"`
	Telefone      string     `json:"telefone,omitempty"`
	WhatsApp      string     `json:"whatsapp,omitempty"`
	Email         string     `json:"email,omitempty"`
	CNAEPrincipal string     `json:"cnae_principal"`
	CNAEDescricao string     `json:"cnae_descricao,omitempty"`
	DataInicio    *time.Time `json:"data_inicio,omitempty"`
	UF            string     `json:"uf"`
	Municipio     string     `json:"municipio,omitempty"`
	CapitalSocial float64    `json:"capital_social"`
	Porte         string     `json:"porte,omitempty"`
	Natureza      string     `json:"natureza_juridica,omitempty"`
}

type RespostaPesquisa struct {
	Resultados     []Lead `json:"resultados"`
	Total          int    `json:"total"`
	LimiteAplicado int    `json:"limite_aplicado"`
	Lote           string `json:"lote"`
	ProximoCursor  string `json:"proximo_cursor,omitempty"`
}

func (s *servidor) pesquisa(w http.ResponseWriter, r *http.Request, conta *store.Conta) {
	var pedido PedidoPesquisa
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, corpoMax))
	if err := dec.Decode(&pedido); err != nil {
		responderErro(w, http.StatusBadRequest, "corpo invalido: "+err.Error())
		return
	}

	filtro, ok := filtroDoPedido(w, pedido)
	if !ok {
		return
	}

	if pedido.Cursor != "" {
		data, cnpj, err := decodificarCursor(pedido.Cursor)
		if err != nil {
			responderErro(w, http.StatusBadRequest, err.Error())
			return
		}
		filtro.AposData = data
		filtro.AposCNPJ = cnpj
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

	// Buscar pode falhar por dois motivos que voltam pelo mesmo tipo error:
	// filtro invalido (erro do cliente) ou falha de infraestrutura (o banco
	// nao respondeu, por exemplo). store.ErrFiltroInvalido e o sentinela que
	// distingue os dois — sem ele, um erro de banco como "context canceled"
	// vazaria no corpo de uma resposta 400, que promete "a culpa e sua"
	// quando na verdade e nossa. So o primeiro caso e texto nosso; o segundo
	// vai para responderErroInterno, que devolve so o ID de correlacao.
	rows, err := store.Buscar(r.Context(), s.pool, filtro)
	if errors.Is(err, store.ErrFiltroInvalido) {
		responderErro(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		responderErroInterno(w, s.log, err)
		return
	}
	defer rows.Close()

	resultados := make([]Lead, 0, 16)
	for rows.Next() {
		var (
			l                  Lead
			ddd, telefone      *string
			fantasia, email    *string
			municipio, porte   *string
			cnaeDesc, natureza *string
		)
		if err := rows.Scan(&l.CNPJ, &l.RazaoSocial, &fantasia,
			&ddd, &telefone, &email,
			&l.CNAEPrincipal, &cnaeDesc, &l.DataInicio, &l.UF, &municipio,
			&l.CapitalSocial, &porte, &natureza); err != nil {
			responderErroInterno(w, s.log, err)
			return
		}
		l.NomeFantasia = valor(fantasia)
		l.Email = valor(email)
		l.Municipio = valor(municipio)
		l.Porte = valor(porte)
		l.CNAEDescricao = valor(cnaeDesc)
		l.Natureza = valor(natureza)
		l.Telefone, l.WhatsApp = montarTelefones(valor(ddd), valor(telefone))
		resultados = append(resultados, l)
	}
	if err := rows.Err(); err != nil {
		responderErroInterno(w, s.log, err)
		return
	}

	// Falha ao registrar uso nao anula a consulta ja feita: registra e segue.
	if err := tenant.RegistrarUso(r.Context(), s.pool, conta.ID, int64(len(resultados))); err != nil {
		s.log.Error("registrar uso", "conta_id", conta.ID, "erro", err)
	}

	limite := pedido.Limite
	if limite <= 0 || limite > store.LimiteMaximo {
		limite = store.LimiteMaximo
	}

	// So ha proximo cursor quando a pagina veio cheia: se voltou menos que o
	// limite pedido, chegamos ao fim do resultado e nao ha o que continuar.
	var proximo string
	if len(resultados) > 0 && len(resultados) == limite {
		ultimo := resultados[len(resultados)-1]
		proximo = codificarCursor(ultimo.DataInicio, ultimo.CNPJ)
	}

	responderJSON(w, http.StatusOK, RespostaPesquisa{
		Resultados:     resultados,
		Total:          len(resultados),
		LimiteAplicado: limite,
		Lote:           vigente.Referencia,
		ProximoCursor:  proximo,
	})
}

// filtroDoPedido traduz o corpo JSON (pesquisa ou contagem) no filtro do
// store. Responde 400 e devolve false quando o pedido nao tem traducao.
func filtroDoPedido(w http.ResponseWriter, pedido PedidoPesquisa) (store.Filtro, bool) {
	filtro := store.Filtro{
		CNAEs:       pedido.CNAEs,
		UFs:         pedido.UFs,
		UltimosDias: pedido.Abertura.UltimosDias,
		SoCelular:   pedido.MaisFiltros.SomenteCelular,
		ExcluirMEI:  pedido.MEI.ExcluirOptante,
		Limite:      pedido.Limite,
	}

	// A traducao nome -> codigo vive so no store, para que API e banco nunca
	// divirjam (docs/arquitetura.md). Nome desconhecido e 400, nunca default silencioso.
	if pedido.Situacao != "" {
		codigo, ok := store.SituacaoPorNome(pedido.Situacao)
		if !ok {
			responderErro(w, http.StatusBadRequest,
				fmt.Sprintf("situacao_cadastral desconhecida: %q", pedido.Situacao))
			return store.Filtro{}, false
		}
		filtro.Situacao = codigo
	}

	return filtro, true
}

func valor(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// montarTelefones remonta o numero. A Receita guarda 8 digitos; o nono entra
// aqui, na leitura, nunca no banco (docs/arquitetura.md).
func montarTelefones(ddd, telefone string) (fone, whatsapp string) {
	if ddd == "" || telefone == "" {
		return "", ""
	}
	fone = ddd + telefone
	if len(telefone) == 8 && telefone[0] == '9' {
		whatsapp = "55" + ddd + "9" + telefone
	}
	return fone, whatsapp
}
