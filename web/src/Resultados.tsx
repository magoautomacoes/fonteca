import type { Filtros, Lead } from './api'
import {
  diasDesde,
  formatarCNPJ,
  formatarData,
  formatarNumero,
  formatarCelular,
  formatarTelefone,
  haQuanto,
  listaPorExtenso,
} from './formato'
import {
  IconeAvancar,
  IconeDownload,
  IconeEmail,
  IconeVoltar,
  IconeWhatsApp,
} from './icones'

// Aberta ha ate uma semana: vale destacar, e quem chega primeiro vende.
const DIAS_NOVA = 7

export function Resumo({
  filtros,
  atividades,
  total,
  leads,
  pagina,
  temMais,
  aoExportar,
  exportando,
}: {
  filtros: Filtros
  atividades: string
  // Total exato vindo da contagem por estado; sem ele, so da para dizer
  // "50+" a partir da pagina carregada.
  total?: number
  leads: Lead[]
  pagina: number
  temMais: boolean
  aoExportar: () => void
  exportando: string | null
}) {
  const onde = filtros.ufs.length === 0 ? 'no Brasil inteiro' : `em ${listaPorExtenso(filtros.ufs)}`
  const quantas =
    total !== undefined
      ? formatarNumero(total)
      : pagina === 0 && !temMais
        ? String(leads.length)
        : `${pagina * 50 + leads.length}${temMais ? '+' : ''}`
  const novas = leads.filter((l) => l.data_inicio && diasDesde(l.data_inicio) <= DIAS_NOVA).length

  return (
    <header className="resumo">
      <div className="resumo-texto">
        <h2>
          <span className="num">{quantas}</span> {quantas === '1' ? 'empresa' : 'empresas'} de{' '}
          <em>{atividades}</em> {onde}
        </h2>
        <p>
          {filtros.situacao === 'ATIVA' ? 'Ativas' : `Situação ${filtros.situacao.toLowerCase()}`}, abertas nos
          últimos {filtros.ultimosDias} dias
          {filtros.somenteCelular && ', todas com celular'}
          {filtros.excluirMEI && ', sem MEI'}.
          {novas > 0 && (
            <>
              {' '}
              <strong>
                {novas} {novas === 1 ? 'abriu' : 'abriram'} nesta semana.
              </strong>
            </>
          )}
        </p>
      </div>
      <button type="button" className="botao botao-contorno" onClick={aoExportar} disabled={exportando !== null}>
        <IconeDownload />
        {exportando ?? 'Exportar planilha'}
      </button>
    </header>
  )
}

export function Tabela({ leads, hoje }: { leads: Lead[]; hoje: Date }) {
  return (
    <div className="tabela-moldura">
      <table className="tabela">
        <thead>
          <tr>
            <th scope="col">Empresa</th>
            <th scope="col">Contato</th>
            <th scope="col">Atividade</th>
            <th scope="col" className="col-data">
              Abertura
            </th>
            <th scope="col">Cidade</th>
          </tr>
        </thead>
        <tbody>
          {leads.map((l, i) => {
            const nova = l.data_inicio ? diasDesde(l.data_inicio, hoje) <= DIAS_NOVA : false
            return (
              <tr key={l.cnpj} style={{ '--i': Math.min(i, 12) } as React.CSSProperties}>
                <td className="col-empresa">
                  <span className="empresa-nome">{l.nome_fantasia || l.razao_social}</span>
                  {l.nome_fantasia && l.nome_fantasia.toUpperCase() !== l.razao_social && (
                    <span className="empresa-razao">{l.razao_social}</span>
                  )}
                  <span className="empresa-cnpj num">{formatarCNPJ(l.cnpj)}</span>
                </td>
                <td className="col-contato" data-rotulo="Contato">
                  {l.whatsapp ? (
                    <a
                      className="whatsapp"
                      href={`https://wa.me/${l.whatsapp}`}
                      target="_blank"
                      rel="noreferrer"
                      aria-label={`Abrir WhatsApp de ${l.nome_fantasia || l.razao_social}`}
                    >
                      <IconeWhatsApp />
                      <span className="num">{formatarCelular(l.whatsapp)}</span>
                    </a>
                  ) : (
                    l.telefone && <span className="telefone num">{formatarTelefone(l.telefone)}</span>
                  )}
                  {l.email && (
                    <a className="email" href={`mailto:${l.email}`}>
                      <IconeEmail />
                      <span>{l.email}</span>
                    </a>
                  )}
                </td>
                <td className="col-atividade" data-rotulo="Atividade">
                  <span>{l.cnae_descricao ?? 'Atividade não descrita'}</span>
                  <span className="atividade-cod num">CNAE {l.cnae_principal}</span>
                </td>
                <td className="col-data" data-rotulo="Abertura">
                  <span className="data num">{formatarData(l.data_inicio)}</span>
                  <span className={nova ? 'selo-nova' : 'data-rel'}>{haQuanto(l.data_inicio, hoje)}</span>
                </td>
                <td className="col-cidade" data-rotulo="Cidade">
                  <span className="cidade">{titulo(l.municipio ?? '')}</span>
                  <span className="uf-sigla">{l.uf}</span>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

// A Receita grava tudo em caixa alta; na tela, "SAO JOSE DOS CAMPOS" vira
// "Sao Jose dos Campos" — mais facil de ler, sem inventar acento.
function titulo(s: string): string {
  const minusculas = new Set(['de', 'da', 'do', 'das', 'dos', 'e'])
  return s
    .toLowerCase()
    .split(' ')
    .map((p, i) => (i > 0 && minusculas.has(p) ? p : p.charAt(0).toUpperCase() + p.slice(1)))
    .join(' ')
}

export function Esqueleto() {
  return (
    <div className="tabela-moldura" aria-busy="true" aria-label="Carregando resultados">
      <div className="esqueleto">
        {Array.from({ length: 8 }, (_, i) => (
          <div className="esqueleto-linha" key={i}>
            <span style={{ width: `${48 + ((i * 17) % 30)}%` }} />
            <span style={{ width: '60%' }} />
            <span style={{ width: `${55 + ((i * 11) % 35)}%` }} />
            <span style={{ width: '50%' }} />
            <span style={{ width: '70%' }} />
          </div>
        ))}
      </div>
    </div>
  )
}

export function Paginacao({
  pagina,
  quantidade,
  temAnterior,
  temProxima,
  carregando,
  anterior,
  proxima,
}: {
  pagina: number
  quantidade: number
  temAnterior: boolean
  temProxima: boolean
  carregando: boolean
  anterior: () => void
  proxima: () => void
}) {
  const inicio = pagina * 50 + 1
  const fim = pagina * 50 + quantidade
  return (
    <nav className="paginacao" aria-label="Paginação">
      <button type="button" className="botao botao-leve" disabled={!temAnterior || carregando} onClick={anterior}>
        <IconeVoltar /> Anterior
      </button>
      <span className="paginacao-faixa num">
        {quantidade === 0 ? 'Nenhum resultado' : `${inicio}–${fim}`}
      </span>
      <button type="button" className="botao botao-leve" disabled={!temProxima || carregando} onClick={proxima}>
        Próxima <IconeAvancar />
      </button>
    </nav>
  )
}

export function Vazio({ aoLimpar }: { aoLimpar: () => void }) {
  return (
    <section className="vazio">
      <h2>Nenhuma empresa com esses filtros.</h2>
      <p>
        Tente ampliar o período de abertura, incluir mais estados ou desligar “Só com celular”. Empresas muito
        recentes podem entrar só no próximo lote mensal da Receita.
      </p>
      <button type="button" className="botao botao-leve" onClick={aoLimpar}>
        Ampliar para o Brasil inteiro
      </button>
    </section>
  )
}
