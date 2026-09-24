import type { CSSProperties } from 'react'
import type { Contagem, Situacao } from './api'
import { NOMES_UF, PERIODOS, REGIOES, SITUACOES } from './catalogo'
import mapa from './dados/mapa-uf.json'
import { formatarNumero } from './formato'

export interface FiltrosRadar {
  ufs: string[]
  situacao: Situacao
  ultimosDias: number
  somenteCelular: boolean
  excluirMEI: boolean
}

interface Props {
  valor: FiltrosRadar
  mudar: (f: FiltrosRadar) => void
  contagem: Contagem | null
  escaneando: boolean
  escanear: () => void
}

const [, , LARGURA, ALTURA] = mapa.viewBox.split(' ').map(Number)
const CX = LARGURA / 2
const CY = ALTURA / 2
const estados = Object.entries(mapa.estados) as [string, { d: string; centro: [number, number] }][]

// Angulo do centro de cada estado, medido do topo em sentido horario: e onde a
// linha da varredura passa por ele. O ponto pisca nesse instante.
function anguloDoTopo([x, y]: [number, number]): number {
  const a = (Math.atan2(x - CX, -(y - CY)) * 180) / Math.PI
  return (a + 360) % 360
}

function alternar(lista: string[], item: string): string[] {
  return lista.includes(item) ? lista.filter((x) => x !== item) : [...lista, item]
}

export function Radar({ valor, mudar, contagem, escaneando, escanear }: Props) {
  const set = <K extends keyof FiltrosRadar>(k: K, v: FiltrosRadar[K]) => mudar({ ...valor, [k]: v })
  const porUF = contagem?.por_uf ?? {}
  const maximo = Math.max(1, ...Object.values(porUF))
  const ranking = Object.entries(porUF).sort((a, b) => b[1] - a[1])
  const selecionados = new Set(valor.ufs)
  const noFiltro = valor.ufs.length === 0 ? contagem?.total ?? 0 : valor.ufs.reduce((n, uf) => n + (porUF[uf] ?? 0), 0)
  const regiaoAtiva = (ufs: string[]) => ufs.every((u) => selecionados.has(u))

  return (
    <div className="radar">
      <div className={escaneando ? 'radar-tela escaneando' : 'radar-tela'}>
        <svg viewBox={mapa.viewBox} className="radar-mapa" role="group" aria-label="Mapa do Brasil. Clique nos estados para filtrar.">
          <g className="radar-aneis" aria-hidden="true">
            {[0.18, 0.34, 0.5, 0.66].map((r) => (
              <circle key={r} cx={CX} cy={CY} r={LARGURA * r} />
            ))}
            <line x1={CX} y1={0} x2={CX} y2={ALTURA} />
            <line x1={0} y1={CY} x2={LARGURA} y2={CY} />
          </g>
          {estados.map(([uf, e]) => {
            const n = porUF[uf] ?? 0
            const calor = contagem ? Math.log1p(n) / Math.log1p(maximo) : 0
            const nome = NOMES_UF[uf]
            return (
              <path
                key={uf}
                d={e.d}
                className={selecionados.has(uf) ? 'uf-mapa selecionado' : 'uf-mapa'}
                style={{ '--calor': calor } as CSSProperties}
                role="checkbox"
                aria-checked={selecionados.has(uf)}
                aria-label={contagem ? `${nome}: ${formatarNumero(n)} empresas` : nome}
                tabIndex={0}
                onClick={() => set('ufs', alternar(valor.ufs, uf))}
                onKeyDown={(ev) => {
                  if (ev.key === ' ' || ev.key === 'Enter') {
                    ev.preventDefault()
                    set('ufs', alternar(valor.ufs, uf))
                  }
                }}
              >
                <title>{contagem ? `${nome} · ${formatarNumero(n)} empresas` : nome}</title>
              </path>
            )
          })}
          {contagem && (
            <g className="radar-pontos" aria-hidden="true">
              {estados.map(([uf, e]) => {
                const n = porUF[uf] ?? 0
                if (!n) return null
                const r = 7 + 17 * Math.sqrt(n / maximo)
                return (
                  <g key={uf} style={{ '--atraso': anguloDoTopo(e.centro) / 360 } as CSSProperties}>
                    <circle className="ponto-onda" cx={e.centro[0]} cy={e.centro[1]} r={r} />
                    <circle className="ponto" cx={e.centro[0]} cy={e.centro[1]} r={Math.max(4, r * 0.42)} />
                  </g>
                )
              })}
              {ranking.slice(0, 6).map(([uf, n]) => {
                const [x, y] = mapa.estados[uf as keyof typeof mapa.estados].centro
                return (
                  <text key={uf} className="ponto-rotulo" x={x + 14} y={y - 12}>
                    {uf} <tspan className="num">{formatarNumero(n)}</tspan>
                  </text>
                )
              })}
            </g>
          )}
        </svg>
        <div className="radar-varredura" aria-hidden="true" />
        <p className="radar-estado" role="status">
          {escaneando
            ? 'Escaneando o cadastro…'
            : contagem
              ? `${formatarNumero(contagem.total)} empresas em ${ranking.length} ${ranking.length === 1 ? 'estado' : 'estados'}`
              : 'Radar pronto'}
        </p>
      </div>

      <div className="radar-controles">
        <fieldset className="grupo">
          <legend>
            Estados
            <span className="legenda-valor">
              {valor.ufs.length === 0 ? 'Brasil inteiro' : `${valor.ufs.length} selecionado${valor.ufs.length > 1 ? 's' : ''}`}
            </span>
          </legend>
          <p className="dica-mapa">Clique no mapa ou escolha uma região.</p>
          <div className="regioes">
            <button type="button" className="pilula" aria-pressed={valor.ufs.length === 0} onClick={() => set('ufs', [])}>
              Brasil
            </button>
            {REGIOES.map((r) => (
              <button
                key={r.nome}
                type="button"
                className="pilula"
                aria-pressed={regiaoAtiva(r.ufs)}
                onClick={() =>
                  set(
                    'ufs',
                    regiaoAtiva(r.ufs)
                      ? valor.ufs.filter((u) => !r.ufs.includes(u))
                      : Array.from(new Set([...valor.ufs, ...r.ufs])),
                  )
                }
              >
                {r.nome}
              </button>
            ))}
          </div>
          {valor.ufs.length > 0 && (
            <p className="ufs-escolhidas">
              {valor.ufs
                .slice()
                .sort()
                .map((uf) => (
                  <button key={uf} type="button" className="uf-ficha" onClick={() => set('ufs', alternar(valor.ufs, uf))} aria-label={`Remover ${NOMES_UF[uf]}`}>
                    {uf} <span aria-hidden="true">×</span>
                  </button>
                ))}
            </p>
          )}
        </fieldset>

        <fieldset className="grupo">
          <legend>Abertura</legend>
          <div className="segmentos" role="radiogroup" aria-label="Abertas nos últimos">
            {PERIODOS.map((d) => (
              <button
                key={d}
                type="button"
                role="radio"
                aria-checked={valor.ultimosDias === d}
                className="segmento"
                onClick={() => set('ultimosDias', d)}
              >
                {d === 365 ? '1 ano' : `${d} dias`}
              </button>
            ))}
          </div>
        </fieldset>

        <fieldset className="grupo">
          <legend>Situação e contato</legend>
          <label className="campo-select">
            <span className="rotulo-discreto">Situação cadastral</span>
            <select value={valor.situacao} onChange={(e) => set('situacao', e.target.value as Situacao)}>
              {SITUACOES.map((s) => (
                <option key={s.valor} value={s.valor}>
                  {s.rotulo}
                </option>
              ))}
            </select>
          </label>
          <label className="chave-liga">
            <input type="checkbox" role="switch" checked={valor.somenteCelular} onChange={(e) => set('somenteCelular', e.target.checked)} />
            <span>
              Só com celular
              <small>para abordar pelo WhatsApp</small>
            </span>
          </label>
          <label className="chave-liga">
            <input type="checkbox" role="switch" checked={valor.excluirMEI} onChange={(e) => set('excluirMEI', e.target.checked)} />
            <span>
              Excluir MEI
              <small>microempreendedores individuais</small>
            </span>
          </label>
        </fieldset>

        <div className="etapa-acao">
          <button type="button" className="botao botao-primario botao-escanear" onClick={escanear} disabled={escaneando}>
            {escaneando ? 'Escaneando…' : contagem ? 'Escanear de novo' : 'Escanear'}
          </button>
          {contagem && !escaneando && (
            <p className="dica">
              <span className="num">{formatarNumero(noFiltro)}</span> {noFiltro === 1 ? 'empresa' : 'empresas'} nos estados do filtro.
            </p>
          )}
        </div>
      </div>
    </div>
  )
}
