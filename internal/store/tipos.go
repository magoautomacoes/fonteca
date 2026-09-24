// Package store define o contrato de dados do Fonteca: tipos de dominio,
// schema SQL e consultas. E o unico pacote que conhece o banco.
package store

import (
	"strings"
	"time"
)

// Codigos de situacao cadastral da Receita Federal.
const (
	SituacaoNula     int16 = 1
	SituacaoAtiva    int16 = 2
	SituacaoSuspensa int16 = 3
	SituacaoInapta   int16 = 4
	SituacaoBaixada  int16 = 8
)

// Codigos de porte da empresa.
const (
	PorteNaoInformado = "00"
	PorteMicro        = "01"
	PortePequeno      = "03"
	PorteDemais       = "05"
)

var situacoesPorNome = map[string]int16{
	"NULA":     SituacaoNula,
	"ATIVA":    SituacaoAtiva,
	"SUSPENSA": SituacaoSuspensa,
	"INAPTA":   SituacaoInapta,
	"BAIXADA":  SituacaoBaixada,
}

// SituacaoPorNome converte o nome textual usado na API para o codigo numerico
// gravado no banco. Aceita qualquer caixa e espacos em volta.
func SituacaoPorNome(nome string) (int16, bool) {
	codigo, ok := situacoesPorNome[strings.ToUpper(strings.TrimSpace(nome))]
	return codigo, ok
}

// NomeDaSituacao faz o caminho inverso. Devolve "" para codigo desconhecido.
func NomeDaSituacao(codigo int16) string {
	for nome, c := range situacoesPorNome {
		if c == codigo {
			return nome
		}
	}
	return ""
}

// Estabelecimento e a unidade fisica: matriz ou filial. Carrega CNAE,
// situacao, endereco resumido e contato. Tabela de ~63 milhoes de linhas.
type Estabelecimento struct {
	CNPJ            string // 14 digitos, sem pontuacao
	CNPJBasico      string // 8 primeiros digitos; junta com Empresa
	MatrizFilial    int16  // 1=matriz, 2=filial
	NomeFantasia    string
	Situacao        int16
	DataSituacao    *time.Time
	DataInicio      *time.Time // inicio de atividade
	CNAEPrincipal   string     // 7 digitos
	CNAESecundarios string     // lista separada por virgula, como vem da Receita
	UF              string     // 2 letras maiusculas
	Municipio       int32      // codigo SIAFI, NAO IBGE (ver MunicipioIBGE)
	DDD1            string
	Telefone1       string
	DDD2            string
	Telefone2       string
	Email           string
	Celular         bool // coluna gerada pelo banco: telefone_1 comeca com 9
}

// Empresa e a pessoa juridica raiz, comum a todos os estabelecimentos
// que compartilham o CNPJ basico.
type Empresa struct {
	CNPJBasico       string
	RazaoSocial      string
	NaturezaJuridica string
	CapitalSocial    float64
	Porte            string // "00", "01", "03" ou "05"
}

// Socio e um integrante do quadro societario. O CPF vem mascarado da
// Receita e por isso nao e armazenado.
type Socio struct {
	CNPJBasico   string
	Nome         string
	Qualificacao string // ex.: "Socio-Administrador"
	DataEntrada  *time.Time
	FaixaEtaria  int16
}

// Simples registra a opcao pelo Simples Nacional e pelo MEI.
type Simples struct {
	CNPJBasico   string
	OpcaoSimples bool
	OpcaoMEI     bool
}

// Municipio e a tabela auxiliar da Receita: codigo SIAFI para nome.
type Municipio struct {
	Codigo int32 // SIAFI
	Nome   string
}

// MunicipioIBGE e o de-para entre o codigo SIAFI usado pela Receita e o
// codigo IBGE usado por todo o resto do governo. Sem ele, qualquer join
// geografico com dados do IBGE quebra silenciosamente.
type MunicipioIBGE struct {
	CodigoSIAFI int32
	CodigoIBGE  int32
	Nome        string
	UF          string
}

// Lote e um conjunto mensal importado. Vive em meta, nao no schema do lote.
type Lote struct {
	Schema      string // "lote_2026_09"
	Referencia  string // "2026-09"
	Vigente     bool
	Linhas      int64
	ImportadoEm time.Time
}

// Conta e um consumidor da API. Multi-tenant desde o inicio.
type Conta struct {
	ID       string // uuid
	Nome     string
	Plano    string
	CriadaEm time.Time
	Ativa    bool
}

// APIKey guarda apenas o hash da chave, nunca a chave.
type APIKey struct {
	Hash      string
	ContaID   string
	Nome      string
	UltimoUso *time.Time
	Revogada  bool
}

// Uso agrega o consumo diario por conta, para cota e futura cobranca.
type Uso struct {
	ContaID          string
	Dia              time.Time
	Consultas        int64
	LinhasRetornadas int64
}
