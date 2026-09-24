import type { Contagem, Filtros, Health, Lead, Pagina } from './api'
import { carregarCatalogo, type Catalogo } from './cnae'
import { diasDesde } from './formato'

// Modo demonstracao: empresas INVENTADAS, geradas de forma deterministica,
// para mostrar a ferramenta (apresentacao, post, print) sem expor contato de
// ninguem. Funciona para qualquer CNAE do catalogo, com a mesma regra de
// filtro, ordenacao e paginacao da API.

function mulberry32(semente: number) {
  let a = semente
  return () => {
    a |= 0
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

// Nome de fantasia por ramo, para os codigos que aparecem em demonstracao
// com mais frequencia. Os demais derivam da descricao oficial.
const ramos: Record<string, string[]> = {
  '2511000': ['Estruturas Metálicas', 'Metalúrgica', 'Aço'],
  '4744001': ['Ferragens', 'Casa das Ferragens', 'Ferramentas'],
  '4744099': ['Materiais de Construção', 'Depósito', 'Construfácil'],
  '4743100': ['Vidraçaria', 'Vidros'],
  '1091102': ['Padaria', 'Panificadora', 'Confeitaria'],
  '5611201': ['Restaurante', 'Cantina', 'Bistrô'],
  '5611203': ['Lanchonete', 'Café'],
  '9602501': ['Salão', 'Studio', 'Espaço Beleza'],
  '4520001': ['Auto Center', 'Oficina', 'Mecânica'],
  '6201501': ['Tech', 'Sistemas', 'Software'],
}

const genericas = new Set(['produtos', 'artigos', 'peças', 'serviços', 'atividades', 'outros', 'outras', 'obras', 'material', 'materiais'])

function ramoDaDescricao(descricao: string): string[] {
  const palavras = descricao
    .replace(/^(Fabricação|Comércio varejista|Comércio atacadista|Serviços|Atividades|Aluguel|Manutenção e reparação|Construção) de /i, '')
    .split(/[\s,;()]+/)
    .filter((p) => p.length > 3 && !genericas.has(p.toLowerCase()))
  const principal = palavras[0] ?? 'Comercial'
  const t = principal.charAt(0).toUpperCase() + principal.slice(1).toLowerCase()
  return [t, `${t} & Cia`, `Grupo ${t}`]
}

const nomes = [
  'Ipê', 'Aroeira', 'Jequitibá', 'Araucária', 'Serra Azul', 'Horizonte', 'Litoral',
  'Cerrado', 'Pampa', 'Canoa', 'Boa Vista', 'Monte Alegre', 'Rio Claro', 'Vale Verde',
  'Três Irmãos', 'São Bento', 'Nova Era', 'Primavera', 'Capivari', 'Itaúna', 'Guará',
  'Jatobá', 'Paineira', 'Bandeirante', 'Correnteza', 'Maré', 'Farol', 'Mirante',
  'Sabiá', 'Tucano', 'Bem-te-vi', 'Buriti', 'Carnaúba', 'Mandacaru', 'Juriti',
]

const cidades: { uf: string; ddd: string; nome: string }[] = [
  { uf: 'SP', ddd: '11', nome: 'São Paulo' }, { uf: 'SP', ddd: '19', nome: 'Campinas' },
  { uf: 'SP', ddd: '16', nome: 'Ribeirão Preto' }, { uf: 'SP', ddd: '15', nome: 'Sorocaba' },
  { uf: 'SP', ddd: '12', nome: 'São José dos Campos' }, { uf: 'SP', ddd: '14', nome: 'Bauru' },
  { uf: 'PR', ddd: '41', nome: 'Curitiba' }, { uf: 'PR', ddd: '43', nome: 'Londrina' },
  { uf: 'PR', ddd: '44', nome: 'Maringá' }, { uf: 'PR', ddd: '45', nome: 'Cascavel' },
  { uf: 'SC', ddd: '48', nome: 'Florianópolis' }, { uf: 'SC', ddd: '47', nome: 'Joinville' },
  { uf: 'SC', ddd: '49', nome: 'Chapecó' }, { uf: 'RS', ddd: '51', nome: 'Porto Alegre' },
  { uf: 'RS', ddd: '54', nome: 'Caxias do Sul' }, { uf: 'RS', ddd: '55', nome: 'Santa Maria' },
  { uf: 'RJ', ddd: '21', nome: 'Rio de Janeiro' }, { uf: 'RJ', ddd: '24', nome: 'Petrópolis' },
  { uf: 'MG', ddd: '31', nome: 'Belo Horizonte' }, { uf: 'MG', ddd: '34', nome: 'Uberlândia' },
  { uf: 'MG', ddd: '32', nome: 'Juiz de Fora' }, { uf: 'ES', ddd: '27', nome: 'Vitória' },
  { uf: 'GO', ddd: '62', nome: 'Goiânia' }, { uf: 'DF', ddd: '61', nome: 'Brasília' },
  { uf: 'MT', ddd: '65', nome: 'Cuiabá' }, { uf: 'MS', ddd: '67', nome: 'Campo Grande' },
  { uf: 'BA', ddd: '71', nome: 'Salvador' }, { uf: 'BA', ddd: '77', nome: 'Vitória da Conquista' },
  { uf: 'SE', ddd: '79', nome: 'Aracaju' }, { uf: 'AL', ddd: '82', nome: 'Maceió' },
  { uf: 'PE', ddd: '81', nome: 'Recife' }, { uf: 'PE', ddd: '87', nome: 'Petrolina' },
  { uf: 'PB', ddd: '83', nome: 'João Pessoa' }, { uf: 'RN', ddd: '84', nome: 'Natal' },
  { uf: 'CE', ddd: '85', nome: 'Fortaleza' }, { uf: 'PI', ddd: '86', nome: 'Teresina' },
  { uf: 'MA', ddd: '98', nome: 'São Luís' }, { uf: 'PA', ddd: '91', nome: 'Belém' },
  { uf: 'AM', ddd: '92', nome: 'Manaus' }, { uf: 'TO', ddd: '63', nome: 'Palmas' },
  { uf: 'RO', ddd: '69', nome: 'Porto Velho' }, { uf: 'AC', ddd: '68', nome: 'Rio Branco' },
  { uf: 'AP', ddd: '96', nome: 'Macapá' }, { uf: 'RR', ddd: '95', nome: 'Boa Vista' },
]

// A atividade economica se concentra no Sul e Sudeste; o peso reflete isso.
const pesoUF: Record<string, number> = { SP: 7, PR: 4, MG: 4, RS: 3, SC: 3, RJ: 3, BA: 2, GO: 2, PE: 2, CE: 2 }

function digitoCNPJ(base: string): string {
  const pesos = base.length === 12 ? [5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2] : [6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2]
  const soma = base.split('').reduce((s, c, i) => s + Number(c) * pesos[i], 0)
  const r = soma % 11
  return String(r < 2 ? 0 : 11 - r)
}

function slug(s: string): string {
  return s
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '')
}

interface LeadDemo extends Lead {
  situacao: 'ATIVA' | 'BAIXADA'
  mei: boolean
  celular: boolean
}

function gerarDoCNAE(codigo: string, cat: Catalogo, hoje: Date): LeadDemo[] {
  const r = mulberry32(Number(codigo) ^ 20260923)
  const escolher = <T,>(xs: T[]) => xs[Math.floor(r() * xs.length)]
  const descricao = cat.porCodigo.get(codigo)?.descricao ?? `Atividade ${codigo}`
  const nomesDoRamo = ramos[codigo] ?? ramoDaDescricao(descricao)
  const cidadesPonderadas = cidades.flatMap((c) => Array(pesoUF[c.uf] ?? 1).fill(c))
  const quantidade = 25 + Math.floor(r() * 70)

  const leads: LeadDemo[] = []
  for (let i = 0; i < quantidade; i++) {
    const fantasia = `${escolher(nomesDoRamo)} ${escolher(nomes)}`
    const cidade = escolher(cidadesPonderadas)
    const sufixo = r() < 0.7 ? 'LTDA' : r() < 0.5 ? 'EIRELI' : 'S.A.'
    const razao = `${fantasia.toUpperCase()} ${r() < 0.35 ? 'COMÉRCIO E SERVIÇOS ' : ''}${sufixo}`

    const basico = String(10000000 + Math.floor(r() * 89999999))
    let cnpj = basico + '0001'
    cnpj += digitoCNPJ(cnpj)
    cnpj += digitoCNPJ(cnpj)

    const dias = Math.floor(Math.pow(r(), 1.3) * 400)
    const data = new Date(hoje.getFullYear(), hoje.getMonth(), hoje.getDate() - dias)
    const iso = `${data.getFullYear()}-${String(data.getMonth() + 1).padStart(2, '0')}-${String(data.getDate()).padStart(2, '0')}T00:00:00Z`

    const celular = r() < 0.86
    // Faixa 9000-xxxx: fora da numeracao movel em uso, nao chama ninguem.
    const numero = celular
      ? '9000' + String(Math.floor(r() * 10000)).padStart(4, '0')
      : '3' + String(Math.floor(r() * 10000000)).padStart(7, '0')
    const capital = [10000, 20000, 30000, 50000, 80000, 100000, 150000, 250000, 500000][Math.floor(Math.pow(r(), 1.6) * 9)]

    leads.push({
      cnpj,
      razao_social: razao,
      nome_fantasia: r() < 0.85 ? fantasia : undefined,
      telefone: cidade.ddd + numero,
      whatsapp: celular ? `55${cidade.ddd}9${numero}` : undefined,
      email: r() < 0.8 ? `contato@${slug(fantasia)}.com.br` : undefined,
      cnae_principal: codigo,
      cnae_descricao: descricao,
      data_inicio: iso,
      uf: cidade.uf,
      municipio: cidade.nome.toUpperCase(),
      capital_social: capital,
      porte: capital >= 250000 ? '03' : '01',
      natureza_juridica: sufixo === 'S.A.' ? 'Sociedade Anônima Fechada' : 'Sociedade Empresária Limitada',
      situacao: r() < 0.94 ? 'ATIVA' : 'BAIXADA',
      mei: r() < 0.12,
      celular,
    })
  }
  return leads
}

const cache = new Map<string, LeadDemo[]>()
let diaDoCache = ''

async function base(cnaes: string[], hoje: Date): Promise<LeadDemo[]> {
  const cat = await carregarCatalogo()
  const dia = hoje.toDateString()
  if (dia !== diaDoCache) {
    cache.clear()
    diaDoCache = dia
  }
  const todos: LeadDemo[] = []
  for (const c of cnaes) {
    if (!cache.has(c)) cache.set(c, gerarDoCNAE(c, cat, hoje))
    todos.push(...cache.get(c)!)
  }
  return todos
}

function filtrar(leads: LeadDemo[], f: Filtros, hoje: Date, comUF: boolean): LeadDemo[] {
  return leads.filter(
    (l) =>
      l.situacao === f.situacao &&
      (!comUF || f.ufs.length === 0 || f.ufs.includes(l.uf)) &&
      (f.ultimosDias === 0 || diasDesde(l.data_inicio!, hoje) <= f.ultimosDias) &&
      (!f.somenteCelular || l.celular) &&
      (!f.excluirMEI || !l.mei),
  )
}

function semCamposInternos(l: LeadDemo): Lead {
  const { situacao: _s, mei: _m, celular: _c, ...lead } = l
  void _s
  void _m
  void _c
  return lead
}

export async function pesquisarDemo(f: Filtros, limite: number, cursor?: string): Promise<Pagina> {
  const hoje = new Date()
  const filtrados = filtrar(await base(f.cnaes, hoje), f, hoje, true).sort((a, b) =>
    a.data_inicio === b.data_inicio
      ? b.cnpj.localeCompare(a.cnpj)
      : (b.data_inicio ?? '').localeCompare(a.data_inicio ?? ''),
  )
  const inicio = cursor ? Number(cursor) || 0 : 0
  const pagina = filtrados.slice(inicio, inicio + limite)
  const fim = inicio + pagina.length
  return {
    resultados: pagina.map(semCamposInternos),
    total: pagina.length,
    limite_aplicado: limite,
    lote: 'demonstração',
    proximo_cursor: fim < filtrados.length ? String(fim) : undefined,
  }
}

export async function contarDemo(f: Filtros): Promise<Contagem> {
  const hoje = new Date()
  const por_uf: Record<string, number> = {}
  for (const l of filtrar(await base(f.cnaes, hoje), f, hoje, true)) por_uf[l.uf] = (por_uf[l.uf] ?? 0) + 1
  return { total: Object.values(por_uf).reduce((a, b) => a + b, 0), por_uf, lote: 'demonstração' }
}

export function healthDemo(): Health {
  return {
    status: 'ok',
    lote: { referencia: 'demonstração', linhas: 0, importado_em: new Date().toISOString() },
  }
}
