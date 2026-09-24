import { useId, useMemo, useState } from 'react'
import { ATALHOS, type Atalho } from './catalogo'
import { buscar, RAMOS_EM_DESTAQUE, type Achado, type Catalogo } from './cnae'
import { IconeBusca, IconeSeta } from './icones'

interface Props {
  catalogo: Catalogo | null
  escolherDivisao: (codigo: string) => void
  escolherSubclasse: (codigo: string) => void
  usarAtalho: (a: Atalho) => void
}

export function EtapaRamo({ catalogo, escolherDivisao, escolherSubclasse, usarAtalho }: Props) {
  const [consulta, setConsulta] = useState('')
  const [ativo, setAtivo] = useState(0)
  const [verSetores, setVerSetores] = useState(false)
  const idLista = useId()

  const achados = useMemo(() => (catalogo ? buscar(catalogo, consulta) : []), [catalogo, consulta])
  const aberta = consulta.trim().length >= 2

  function escolher(a: Achado) {
    setConsulta('')
    if (a.tipo === 'divisao') escolherDivisao(a.codigo)
    else escolherSubclasse(a.codigo)
  }

  return (
    <div className="etapa-ramo">
      <div className="busca" role="combobox" aria-expanded={aberta && achados.length > 0} aria-owns={idLista} aria-haspopup="listbox">
        <IconeBusca className="busca-icone" />
        <input
          type="search"
          value={consulta}
          onChange={(e) => {
            setConsulta(e.target.value)
            setAtivo(0)
          }}
          onKeyDown={(e) => {
            if (!aberta || achados.length === 0) return
            if (e.key === 'ArrowDown') {
              e.preventDefault()
              setAtivo((i) => Math.min(i + 1, achados.length - 1))
            } else if (e.key === 'ArrowUp') {
              e.preventDefault()
              setAtivo((i) => Math.max(i - 1, 0))
            } else if (e.key === 'Enter') {
              e.preventDefault()
              escolher(achados[ativo])
            } else if (e.key === 'Escape') {
              setConsulta('')
            }
          }}
          placeholder={catalogo ? 'Digite o ramo: padaria, oficina mecânica, salão de beleza, 5611201…' : 'Carregando atividades…'}
          disabled={!catalogo}
          aria-autocomplete="list"
          aria-controls={idLista}
          aria-activedescendant={aberta && achados[ativo] ? `${idLista}-${ativo}` : undefined}
          aria-label="Buscar ramo ou atividade"
          autoComplete="off"
          spellCheck={false}
        />
        {aberta && (
          <ul className="busca-lista" id={idLista} role="listbox">
            {achados.length === 0 && <li className="busca-vazia">Nenhuma atividade com “{consulta.trim()}”.</li>}
            {achados.map((a, i) => (
              <li
                key={a.tipo + a.codigo}
                id={`${idLista}-${i}`}
                role="option"
                aria-selected={i === ativo}
                className="busca-item"
                onMouseEnter={() => setAtivo(i)}
                onMouseDown={(e) => {
                  e.preventDefault()
                  escolher(a)
                }}
              >
                <span className={a.tipo === 'divisao' ? 'busca-tipo busca-tipo-ramo' : 'busca-tipo'}>
                  {a.tipo === 'divisao' ? 'Ramo' : a.codigo}
                </span>
                <span className="busca-texto">
                  <span className="busca-desc">{a.descricao}</span>
                  <span className="busca-contexto">{a.contexto}</span>
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="ramos">
        <p className="ramos-rotulo">Ramos mais procurados</p>
        <ul className="ramos-lista">
          {RAMOS_EM_DESTAQUE.map((c) => {
            const d = catalogo?.porDivisao.get(c)
            return (
              <li key={c}>
                <button type="button" className="ramo" disabled={!d} onClick={() => escolherDivisao(c)}>
                  <span className="ramo-nome">{d?.descricao ?? '…'}</span>
                  {d && <span className="ramo-total num">{d.total} atividades</span>}
                </button>
              </li>
            )
          })}
        </ul>

        <button type="button" className="link-discreto" onClick={() => setVerSetores((v) => !v)} aria-expanded={verSetores}>
          {verSetores ? 'Esconder os setores' : 'Ver todos os 21 setores da economia'}
        </button>
        {verSetores && catalogo && (
          <div className="setores">
            {Object.entries(catalogo.secoes).map(([s, nome]) => (
              <section key={s} className="setor">
                <h4>{nome}</h4>
                <ul>
                  {catalogo.divisoes
                    .filter((d) => d.secao === s)
                    .map((d) => (
                      <li key={d.codigo}>
                        <button type="button" className="setor-divisao" onClick={() => escolherDivisao(d.codigo)}>
                          {d.descricao}
                        </button>
                      </li>
                    ))}
                </ul>
              </section>
            ))}
          </div>
        )}
      </div>

      <div className="exemplos">
        <p className="ramos-rotulo">Ou comece por um exemplo pronto</p>
        <ul className="atalhos">
          {ATALHOS.map((a) => (
            <li key={a.titulo}>
              <button type="button" className="atalho" onClick={() => usarAtalho(a)}>
                <span className="atalho-texto">
                  <span className="atalho-titulo">{a.titulo}</span>
                  <span className="atalho-detalhe">{a.detalhe}</span>
                </span>
                <IconeSeta className="atalho-seta" />
              </button>
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}
