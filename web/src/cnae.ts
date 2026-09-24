// Catalogo oficial da CNAE 2.3 (IBGE), gerado por scripts/gerar-catalogo-cnae.py.
// E grande (~130 KB compactado), entao so e baixado quando a tela precisa.

export interface Subclasse {
  codigo: string
  descricao: string
  termos: string
  classe: string
  grupo: string
  divisao: string
}

export interface Grupo {
  codigo: string
  descricao: string
  subclasses: Subclasse[]
}

export interface Divisao {
  codigo: string
  descricao: string
  secao: string
  grupos: Grupo[]
  total: number
}

export interface Catalogo {
  secoes: Record<string, string>
  divisoes: Divisao[]
  porDivisao: Map<string, Divisao>
  porCodigo: Map<string, Subclasse>
}

interface Bruto {
  secoes: Record<string, string>
  divisoes: Record<string, [string, string]>
  grupos: Record<string, string>
  classes: Record<string, string>
  subclasses: [string, string, string][]
}

let promessa: Promise<Catalogo> | null = null

export function carregarCatalogo(): Promise<Catalogo> {
  promessa ??= import('./dados/cnae.json').then((m) => montar(m.default as unknown as Bruto))
  return promessa
}

function montar(b: Bruto): Catalogo {
  const porCodigo = new Map<string, Subclasse>()
  const grupos = new Map<string, Grupo>()
  for (const [codigo, descricao, termos] of b.subclasses) {
    const s: Subclasse = {
      codigo,
      descricao,
      termos,
      classe: codigo.slice(0, 5),
      grupo: codigo.slice(0, 3),
      divisao: codigo.slice(0, 2),
    }
    porCodigo.set(codigo, s)
    let g = grupos.get(s.grupo)
    if (!g) {
      g = { codigo: s.grupo, descricao: b.grupos[s.grupo] ?? s.grupo, subclasses: [] }
      grupos.set(s.grupo, g)
    }
    g.subclasses.push(s)
  }
  const divisoes: Divisao[] = Object.entries(b.divisoes).map(([codigo, [descricao, secao]]) => {
    const gs = [...grupos.values()].filter((g) => g.codigo.startsWith(codigo))
    return { codigo, descricao, secao, grupos: gs, total: gs.reduce((n, g) => n + g.subclasses.length, 0) }
  })
  return {
    secoes: b.secoes,
    divisoes,
    porDivisao: new Map(divisoes.map((d) => [d.codigo, d])),
    porCodigo,
  }
}

export function normalizar(s: string): string {
  return s
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
}

export interface Achado {
  tipo: 'divisao' | 'subclasse'
  codigo: string
  descricao: string
  contexto: string
  pontos: number
}

// Busca por prefixo de palavra: "padar" acha "Fabricação de produtos de
// padaria e confeitaria" (os termos do IBGE tem "panificacao", "paes"). Cada palavra da
// consulta precisa aparecer; nome oficial pesa mais que termo popular, e
// ramo inteiro (divisao) aparece antes de atividade solta.
export function buscar(cat: Catalogo, consulta: string, max = 12): Achado[] {
  const q = normalizar(consulta).trim()
  if (q.length < 2) return []
  if (/^\d{2,7}$/.test(q)) {
    const porCodigo: Achado[] = []
    if (q.length === 2 && cat.porDivisao.has(q)) {
      const d = cat.porDivisao.get(q)!
      porCodigo.push({ tipo: 'divisao', codigo: d.codigo, descricao: d.descricao, contexto: cat.secoes[d.secao], pontos: 100 })
    }
    for (const s of cat.porCodigo.values()) {
      if (s.codigo.startsWith(q))
        porCodigo.push({
          tipo: 'subclasse',
          codigo: s.codigo,
          descricao: s.descricao,
          contexto: cat.porDivisao.get(s.divisao)?.descricao ?? '',
          pontos: 50,
        })
    }
    return porCodigo.slice(0, max)
  }

  const palavras = q.split(/\s+/).filter((p) => p.length >= 2)
  const achados: Achado[] = []
  const temPalavra = (texto: string, p: string) => texto.startsWith(p) || texto.includes(' ' + p)

  for (const d of cat.divisoes) {
    const nome = normalizar(d.descricao)
    if (palavras.every((p) => temPalavra(nome, p)))
      achados.push({ tipo: 'divisao', codigo: d.codigo, descricao: d.descricao, contexto: cat.secoes[d.secao], pontos: 30 })
  }
  for (const s of cat.porCodigo.values()) {
    const nome = normalizar(s.descricao)
    let pontos = 0
    let ok = true
    for (const p of palavras) {
      if (temPalavra(nome, p)) pontos += 10
      else if (temPalavra(s.termos, p)) pontos += 4
      else {
        ok = false
        break
      }
    }
    if (ok)
      achados.push({
        tipo: 'subclasse',
        codigo: s.codigo,
        descricao: s.descricao,
        contexto: cat.porDivisao.get(s.divisao)?.descricao ?? '',
        pontos: pontos - s.descricao.length / 100,
      })
  }
  return achados.sort((a, b) => b.pontos - a.pontos).slice(0, max)
}

// Ramos que aparecem de cara, antes de digitar: os mais procurados em
// prospeccao B2B local. Codigos de divisao da CNAE.
export const RAMOS_EM_DESTAQUE = ['56', '47', '45', '96', '86', '43', '10', '62', '41', '46', '25', '85']

// Nome curto para frase ("12 empresas de restaurantes"). Tira o
// prefixo generico da descricao oficial.
export function nomeCurto(descricao: string): string {
  const d = descricao
    .replace(/^(Fabricação|Comércio varejista|Comércio atacadista|Serviços|Atividades|Aluguel|Manutenção e reparação) de /i, '')
    .replace(/^(produtos|artigos|peças) de /i, '')
  // "Restaurantes e similares" vira "restaurantes" e "Estética e outros
  // serviços de..." vira "estética": em frase, o sufixo so atrapalha.
  const curto = d
    .split(/[,;(]/)[0]
    .replace(/ e (similares|afins)$/i, '')
    .replace(/ e outr[oa]s .*$/i, '')
    .trim()
  return curto.length > 48 ? curto.slice(0, 46).trimEnd() + '…' : curto.charAt(0).toLowerCase() + curto.slice(1)
}
