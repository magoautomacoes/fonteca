import { describe, expect, it } from 'vitest'
import type { Lead } from './api'
import { leadsParaCSV } from './csv'
import { listaDeCodigos } from './api'

const base: Lead = {
  cnpj: '10000001000101',
  razao_social: 'RESTAURANTE ALFA LTDA',
  whatsapp: '5511998888777',
  cnae_principal: '5611201',
  cnae_descricao: 'Restaurantes e similares',
  data_inicio: '2026-07-10T00:00:00Z',
  uf: 'SP',
  capital_social: 150000,
}

describe('leadsParaCSV', () => {
  it('comeca com BOM e usa ; como separador', () => {
    const csv = leadsParaCSV([base])
    expect(csv.startsWith('﻿CNPJ;Razao social;')).toBe(true)
    const [, linha] = csv.slice(1).split('\r\n')
    expect(linha.split(';')[0]).toBe('10000001000101')
  })

  it('formata data e capital para o Excel em portugues', () => {
    const linha = leadsParaCSV([base]).split('\r\n')[1]
    expect(linha).toContain(';2026-07-10;')
    expect(linha).toContain('150000,00')
  })

  it('protege celula com ; e aspas', () => {
    const csv = leadsParaCSV([{ ...base, razao_social: 'A;B "C"' }])
    expect(csv).toContain(';"A;B ""C""";')
  })

  it('neutraliza formula vinda da Receita', () => {
    const csv = leadsParaCSV([{ ...base, razao_social: '=HYPERLINK("x")' }])
    expect(csv).toContain(`"'=HYPERLINK(""x"")"`)
  })

  it('telefone sai com o nono digito do celular', () => {
    const linha = leadsParaCSV([{ ...base, telefone: '4797145089', whatsapp: '5547997145089' }]).split('\r\n')[1]
    expect(linha).toContain(';5547997145089;(47) 99714-5089;')
  })

  it('campo ausente vira celula vazia, nao "undefined"', () => {
    expect(leadsParaCSV([base])).not.toContain('undefined')
  })
})

describe('listaDeCodigos', () => {
  it('aceita virgula, espaco e quebra de linha', () => {
    expect(listaDeCodigos(' 2512800, 2511000\n1622602 ;sp')).toEqual([
      '2512800',
      '2511000',
      '1622602',
      'SP',
    ])
  })
})

describe('formatarTelefone', () => {
  it('formata DDD + 8 digitos', async () => {
    const { formatarTelefone } = await import('./formato')
    expect(formatarTelefone('1198888777')).toBe('(11) 9888-8777')
    expect(formatarTelefone('')).toBe('')
  })
})

describe('formatarCelular', () => {
  it('mostra o celular com o nono digito restaurado', async () => {
    const { formatarCelular } = await import('./formato')
    expect(formatarCelular('5547997145089')).toBe('(47) 99714-5089')
  })
})

describe('nomeCurto', () => {
  it('tira prefixo generico e sufixos que atrapalham a frase', async () => {
    const { nomeCurto } = await import('./cnae')
    expect(nomeCurto('Restaurantes e similares')).toBe('restaurantes')
    expect(nomeCurto('Atividades de estética e outros serviços de cuidados com a beleza')).toBe('estética')
    expect(nomeCurto('Cabeleireiros, manicure e pedicure')).toBe('cabeleireiros')
    expect(nomeCurto('Comércio varejista de ferragens e ferramentas')).toBe('ferragens e ferramentas')
  })
})
