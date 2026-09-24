import { describe, expect, it } from 'vitest'
import type { Filtros } from './api'
import { contarDemo, pesquisarDemo } from './demo'
import { diasDesde } from './formato'

const filtro: Filtros = {
  cnaes: ['2512800', '4744001'],
  ufs: ['SP', 'PR'],
  situacao: 'ATIVA',
  ultimosDias: 180,
  somenteCelular: true,
  excluirMEI: true,
}

describe('modo demonstracao', () => {
  it('respeita os filtros como a API', async () => {
    const { resultados } = await pesquisarDemo(filtro, 1000)
    expect(resultados.length).toBeGreaterThan(10)
    for (const l of resultados) {
      expect(filtro.cnaes).toContain(l.cnae_principal)
      expect(filtro.ufs).toContain(l.uf)
      expect(diasDesde(l.data_inicio!)).toBeLessThanOrEqual(180)
      expect(l.whatsapp).toBeTruthy()
    }
  })

  it('pagina por cursor sem repetir e termina', async () => {
    const vistos = new Set<string>()
    let cursor: string | undefined
    for (let i = 0; i < 50; i++) {
      const p = await pesquisarDemo(filtro, 7, cursor)
      for (const l of p.resultados) {
        expect(vistos.has(l.cnpj)).toBe(false)
        vistos.add(l.cnpj)
      }
      cursor = p.proximo_cursor
      if (!cursor) break
    }
    expect(cursor).toBeUndefined()
    expect(vistos.size).toBe((await pesquisarDemo(filtro, 1000)).resultados.length)
  })

  it('usa so telefones fora da numeracao movel em uso', async () => {
    for (const l of (await pesquisarDemo({ ...filtro, ufs: [], somenteCelular: false }, 1000)).resultados) {
      expect(l.telefone!.slice(2)).toMatch(/^(9000|3)\d+$/)
    }
  })
})

describe('contagem da demonstracao', () => {
  it('bate com a pesquisa em cada estado, para qualquer CNAE', async () => {
    const f: Filtros = { ...filtro, cnaes: ['1091102', '5611201'], ufs: [] }
    const cont = await contarDemo(f)
    const { resultados } = await pesquisarDemo(f, 1000)
    expect(cont.total).toBe(resultados.length)
    for (const [uf, n] of Object.entries(cont.por_uf)) {
      expect(resultados.filter((l) => l.uf === uf).length).toBe(n)
    }
  })
})
