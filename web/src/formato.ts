// Formatos brasileiros usados na tela e no CSV.

// Telefone como a Receita guarda: DDD + 8 digitos, "(11) 3888-8777".
export function formatarTelefone(t: string): string {
  return t.replace(/^(\d{2})(\d{4})(\d{4})$/, '($1) $2-$3')
}

// Celular para exibir, a partir do numero de WhatsApp da API (55 + DDD + 9
// digitos, com o nono ja restaurado): "(47) 99714-5089". Mostrar o telefone
// cru da Receita, com 8 digitos, faz o numero parecer invalido.
export function formatarCelular(whatsapp: string): string {
  return whatsapp.replace(/^55(\d{2})(\d{5})(\d{4})$/, '($1) $2-$3')
}

export function formatarCNPJ(c: string): string {
  return c.replace(/^(\d{2})(\d{3})(\d{3})(\d{4})(\d{2})$/, '$1.$2.$3/$4-$5')
}

// Datas da API sao dias (sem hora) serializados em UTC: le so a parte da data,
// senao o fuso de Brasilia empurraria para o dia anterior.
export function dataDaAPI(iso: string): Date {
  const [a, m, d] = iso.slice(0, 10).split('-').map(Number)
  return new Date(a, m - 1, d)
}

export function formatarData(iso?: string): string {
  if (!iso) return ''
  const [a, m, d] = iso.slice(0, 10).split('-')
  return `${d}/${m}/${a}`
}

const umDia = 24 * 60 * 60 * 1000

export function diasDesde(iso: string, hoje = new Date()): number {
  const base = new Date(hoje.getFullYear(), hoje.getMonth(), hoje.getDate())
  return Math.round((base.getTime() - dataDaAPI(iso).getTime()) / umDia)
}

export function haQuanto(iso?: string, hoje = new Date()): string {
  if (!iso) return ''
  const d = diasDesde(iso, hoje)
  if (d <= 0) return 'hoje'
  if (d === 1) return 'ontem'
  if (d < 30) return `há ${d} dias`
  const meses = Math.round(d / 30)
  return meses === 1 ? 'há 1 mês' : `há ${meses} meses`
}

export function formatarNumero(n: number): string {
  return n.toLocaleString('pt-BR')
}

// "SP, PR e SC" — lista em portugues, com "e" antes do ultimo.
export function listaPorExtenso(itens: string[]): string {
  if (itens.length <= 1) return itens.join('')
  return `${itens.slice(0, -1).join(', ')} e ${itens[itens.length - 1]}`
}
