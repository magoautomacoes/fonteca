import type { Situacao } from './api'

export const UFS = [
  'AC', 'AL', 'AP', 'AM', 'BA', 'CE', 'DF', 'ES', 'GO', 'MA', 'MT', 'MS', 'MG', 'PA',
  'PB', 'PR', 'PE', 'PI', 'RJ', 'RN', 'RS', 'RO', 'RR', 'SC', 'SP', 'SE', 'TO',
]

export const NOMES_UF: Record<string, string> = {
  AC: 'Acre', AL: 'Alagoas', AP: 'Amapá', AM: 'Amazonas', BA: 'Bahia', CE: 'Ceará',
  DF: 'Distrito Federal', ES: 'Espírito Santo', GO: 'Goiás', MA: 'Maranhão', MT: 'Mato Grosso',
  MS: 'Mato Grosso do Sul', MG: 'Minas Gerais', PA: 'Pará', PB: 'Paraíba', PR: 'Paraná',
  PE: 'Pernambuco', PI: 'Piauí', RJ: 'Rio de Janeiro', RN: 'Rio Grande do Norte',
  RS: 'Rio Grande do Sul', RO: 'Rondônia', RR: 'Roraima', SC: 'Santa Catarina',
  SP: 'São Paulo', SE: 'Sergipe', TO: 'Tocantins',
}

export const REGIOES: { nome: string; ufs: string[] }[] = [
  { nome: 'Sul', ufs: ['PR', 'SC', 'RS'] },
  { nome: 'Sudeste', ufs: ['SP', 'RJ', 'MG', 'ES'] },
  { nome: 'Centro-Oeste', ufs: ['DF', 'GO', 'MT', 'MS'] },
  { nome: 'Nordeste', ufs: ['BA', 'SE', 'AL', 'PE', 'PB', 'RN', 'CE', 'PI', 'MA'] },
  { nome: 'Norte', ufs: ['AM', 'PA', 'AC', 'RO', 'RR', 'AP', 'TO'] },
]

export const PERIODOS = [30, 90, 180, 365]

export const SITUACOES: { valor: Situacao; rotulo: string }[] = [
  { valor: 'ATIVA', rotulo: 'Ativa' },
  { valor: 'SUSPENSA', rotulo: 'Suspensa' },
  { valor: 'INAPTA', rotulo: 'Inapta' },
  { valor: 'BAIXADA', rotulo: 'Baixada' },
  { valor: 'NULA', rotulo: 'Nula' },
]

export interface Atalho {
  titulo: string
  detalhe: string
  cnaes: string[]
  ufs: string[]
  ultimosDias: number
}

// Pesquisas prontas para a tela vazia: ensinam o que o filtro faz.
export const ATALHOS: Atalho[] = [
  {
    titulo: 'Restaurantes e lanchonetes em São Paulo',
    detalhe: 'abertos nos últimos 90 dias',
    cnaes: ['5611201', '5611203'],
    ufs: ['SP'],
    ultimosDias: 90,
  },
  {
    titulo: 'Oficinas mecânicas no Sul',
    detalhe: 'mecânica e funilaria · PR, SC e RS · últimos 30 dias',
    cnaes: ['4520001', '4520002'],
    ufs: ['PR', 'SC', 'RS'],
    ultimosDias: 30,
  },
  {
    titulo: 'Salões de beleza no Brasil inteiro',
    detalhe: 'cabeleireiros e estética · últimos 180 dias',
    cnaes: ['9602501', '9602502'],
    ufs: [],
    ultimosDias: 180,
  },
]
