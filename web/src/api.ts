// Cliente da API REST. O frontend nao tem endpoint proprio: fala com as
// mesmas rotas que qualquer integracao.

export type Situacao = 'ATIVA' | 'BAIXADA' | 'INAPTA' | 'SUSPENSA' | 'NULA'

export interface Filtros {
  cnaes: string[]
  ufs: string[]
  situacao: Situacao
  ultimosDias: number
  somenteCelular: boolean
  excluirMEI: boolean
}

export interface Lead {
  cnpj: string
  razao_social: string
  nome_fantasia?: string
  telefone?: string
  whatsapp?: string
  email?: string
  cnae_principal: string
  cnae_descricao?: string
  data_inicio?: string
  uf: string
  municipio?: string
  capital_social: number
  porte?: string
  natureza_juridica?: string
}

export interface Pagina {
  resultados: Lead[]
  total: number
  limite_aplicado: number
  lote: string
  proximo_cursor?: string
}

export interface Contagem {
  total: number
  por_uf: Record<string, number>
  lote: string
}

export interface Health {
  status: string
  lote: { referencia: string; linhas: number; importado_em: string } | null
}

// ErroDaAPI carrega o status para a tela distinguir chave errada (401),
// limite (429) e base sem lote (503) de falha generica.
export class ErroDaAPI extends Error {
  readonly status: number
  readonly correlacao?: string

  constructor(status: number, mensagem: string, correlacao?: string) {
    super(mensagem)
    this.status = status
    this.correlacao = correlacao
  }
}

async function lerResposta<T>(r: Response): Promise<T> {
  if (r.ok) return (await r.json()) as T
  let mensagem = `erro ${r.status}`
  let correlacao: string | undefined
  try {
    const corpo = (await r.json()) as { erro?: string; correlacao?: string }
    if (corpo.erro) mensagem = corpo.erro
    correlacao = corpo.correlacao
  } catch {
    // corpo nao-JSON (proxy fora do ar, por exemplo): fica a mensagem generica
  }
  throw new ErroDaAPI(r.status, mensagem, correlacao)
}

export function corpoDaPesquisa(f: Filtros, limite: number, cursor?: string) {
  return {
    codigo_atividade_principal: f.cnaes,
    situacao_cadastral: f.situacao,
    uf: f.ufs,
    data_abertura: { ultimos_dias: f.ultimosDias },
    mei: { excluir_optante: f.excluirMEI },
    mais_filtros: { somente_celular: f.somenteCelular },
    limite,
    ...(cursor ? { cursor } : {}),
  }
}

export async function pesquisar(
  chave: string,
  f: Filtros,
  limite: number,
  cursor?: string,
  sinal?: AbortSignal,
): Promise<Pagina> {
  const r = await fetch('/v1/cnpj/pesquisa', {
    method: 'POST',
    headers: { 'api-key': chave, 'content-type': 'application/json' },
    body: JSON.stringify(corpoDaPesquisa(f, limite, cursor)),
    signal: sinal,
  })
  return lerResposta<Pagina>(r)
}

// contar devolve quantas empresas o filtro acha em cada estado. E o que
// acende o mapa do radar; limite e cursor nao se aplicam.
export async function contar(chave: string, f: Filtros, sinal?: AbortSignal): Promise<Contagem> {
  const r = await fetch('/v1/cnpj/contagem', {
    method: 'POST',
    headers: { 'api-key': chave, 'content-type': 'application/json' },
    body: JSON.stringify(corpoDaPesquisa(f, 0)),
    signal: sinal,
  })
  return lerResposta<Contagem>(r)
}

export async function health(sinal?: AbortSignal): Promise<Health> {
  return lerResposta<Health>(await fetch('/v1/health', { signal: sinal }))
}

// Separa por virgula, espaco ou quebra de linha e descarta vazios, para o
// usuario colar uma lista de CNAEs do jeito que vier.
export function listaDeCodigos(texto: string): string[] {
  return texto
    .split(/[\s,;]+/)
    .map((s) => s.trim().toUpperCase())
    .filter(Boolean)
}
