import type { Lead } from './api'
import { formatarCelular, formatarTelefone } from './formato'

// Colunas do CSV, na ordem em que o time comercial le: quem e, como falar,
// o que faz, onde fica.
const colunas: { titulo: string; valor: (l: Lead) => string }[] = [
  { titulo: 'CNPJ', valor: (l) => l.cnpj },
  { titulo: 'Razao social', valor: (l) => l.razao_social },
  { titulo: 'Nome fantasia', valor: (l) => l.nome_fantasia ?? '' },
  { titulo: 'WhatsApp', valor: (l) => l.whatsapp ?? '' },
  // Celular com o nono digito ja restaurado; fixo como a Receita guarda.
  {
    titulo: 'Telefone',
    valor: (l) => (l.whatsapp ? formatarCelular(l.whatsapp) : formatarTelefone(l.telefone ?? '')),
  },
  { titulo: 'Email', valor: (l) => l.email ?? '' },
  { titulo: 'CNAE', valor: (l) => l.cnae_principal },
  { titulo: 'Atividade', valor: (l) => l.cnae_descricao ?? '' },
  { titulo: 'Abertura', valor: (l) => (l.data_inicio ?? '').slice(0, 10) },
  { titulo: 'UF', valor: (l) => l.uf },
  { titulo: 'Municipio', valor: (l) => l.municipio ?? '' },
  { titulo: 'Natureza juridica', valor: (l) => l.natureza_juridica ?? '' },
  { titulo: 'Porte', valor: (l) => l.porte ?? '' },
  {
    titulo: 'Capital social',
    valor: (l) => l.capital_social.toFixed(2).replace('.', ','),
  },
]

// Celula que comeca com = + - @ (ou tab/CR) vira formula ao abrir no Excel:
// razao social e email vem da Receita, nao de nos. O apostrofo neutraliza.
function neutralizarFormula(s: string): string {
  return /^[=+\-@\t\r]/.test(s) ? `'${s}` : s
}

function celula(s: string): string {
  const v = neutralizarFormula(s)
  return /[";\n\r]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v
}

// leadsParaCSV gera o arquivo no formato que o Excel em portugues abre
// direto: separador ";", decimal com virgula e BOM para ele reconhecer UTF-8
// (sem o BOM, acentos viram lixo).
export function leadsParaCSV(leads: Lead[]): string {
  const linhas = [colunas.map((c) => celula(c.titulo)).join(';')]
  for (const l of leads) {
    linhas.push(colunas.map((c) => celula(c.valor(l))).join(';'))
  }
  return '﻿' + linhas.join('\r\n') + '\r\n'
}
