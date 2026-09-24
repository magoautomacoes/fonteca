import { useEffect, useRef, useState } from 'react'
import type { Catalogo, Grupo } from './cnae'
import { IconeFechar } from './icones'

interface Props {
  catalogo: Catalogo
  divisao: string
  selecionadas: string[]
  mudar: (codigos: string[]) => void
  continuar: () => void
}

function CaixaGrupo({ grupo, selecionadas, alternarGrupo }: { grupo: Grupo; selecionadas: Set<string>; alternarGrupo: (g: Grupo, marcar: boolean) => void }) {
  const ref = useRef<HTMLInputElement>(null)
  const marcadas = grupo.subclasses.filter((s) => selecionadas.has(s.codigo)).length
  const todas = marcadas === grupo.subclasses.length
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = marcadas > 0 && !todas
  }, [marcadas, todas])
  return (
    <input
      ref={ref}
      type="checkbox"
      checked={todas}
      onChange={() => alternarGrupo(grupo, !todas)}
      aria-label={`Todas as atividades de ${grupo.descricao}`}
    />
  )
}

export function EtapaAtividades({ catalogo, divisao, selecionadas, mudar, continuar }: Props) {
  const d = catalogo.porDivisao.get(divisao)
  const conjunto = new Set(selecionadas)
  const [abertos, setAbertos] = useState<Set<string>>(() => new Set(d?.grupos.slice(0, 3).map((g) => g.codigo)))
  if (!d) return null

  const foraDoRamo = selecionadas.filter((c) => !c.startsWith(divisao))

  function alternar(codigo: string) {
    mudar(conjunto.has(codigo) ? selecionadas.filter((c) => c !== codigo) : [...selecionadas, codigo])
  }

  function alternarGrupo(g: Grupo, marcar: boolean) {
    const codigos = g.subclasses.map((s) => s.codigo)
    mudar(marcar ? Array.from(new Set([...selecionadas, ...codigos])) : selecionadas.filter((c) => !codigos.includes(c)))
  }

  const doRamo = selecionadas.length - foraDoRamo.length

  return (
    <div className="etapa-atividades">
      <div className="atividades-topo">
        <p>
          <span className="num">{doRamo}</span> de <span className="num">{d.total}</span> atividades de{' '}
          <strong>{d.descricao.toLowerCase()}</strong> selecionadas.
        </p>
        <div className="atividades-atalhos">
          <button type="button" className="link-discreto" onClick={() => d.grupos.forEach((g) => alternarGrupo(g, true))}>
            Marcar todas
          </button>
          <button
            type="button"
            className="link-discreto"
            onClick={() => mudar(foraDoRamo)}
          >
            Desmarcar todas
          </button>
        </div>
      </div>

      <ul className="arvore">
        {d.grupos.map((g) => {
          const aberto = abertos.has(g.codigo)
          const marcadas = g.subclasses.filter((s) => conjunto.has(s.codigo)).length
          return (
            <li key={g.codigo} className="arvore-grupo">
              <div className="arvore-cabeca">
                <CaixaGrupo grupo={g} selecionadas={conjunto} alternarGrupo={(gr, m) => {
                  alternarGrupo(gr, m)
                }} />
                <button
                  type="button"
                  className="arvore-alternar"
                  aria-expanded={aberto}
                  onClick={() =>
                    setAbertos((a) => {
                      const n = new Set(a)
                      if (n.has(g.codigo)) n.delete(g.codigo)
                      else n.add(g.codigo)
                      return n
                    })
                  }
                >
                  <span className="arvore-titulo">{g.descricao}</span>
                  <span className="arvore-contagem num">
                    {marcadas}/{g.subclasses.length}
                  </span>
                  <span className="arvore-seta" aria-hidden="true" />
                </button>
              </div>
              {aberto && (
                <ul className="arvore-folhas">
                  {g.subclasses.map((s) => (
                    <li key={s.codigo}>
                      <label className="folha">
                        <input type="checkbox" checked={conjunto.has(s.codigo)} onChange={() => alternar(s.codigo)} />
                        <span className="folha-desc">{s.descricao}</span>
                        <span className="folha-cod num">{s.codigo}</span>
                      </label>
                    </li>
                  ))}
                </ul>
              )}
            </li>
          )
        })}
      </ul>

      {foraDoRamo.length > 0 && (
        <div className="outras">
          <p className="ramos-rotulo">Também na pesquisa, de outros ramos</p>
          <ul className="fichas">
            {foraDoRamo.map((c) => (
              <li key={c} className="ficha-cnae">
                <span>{catalogo.porCodigo.get(c)?.descricao ?? c}</span>
                <button type="button" className="icone-botao" aria-label={`Remover ${c}`} onClick={() => alternar(c)}>
                  <IconeFechar />
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="etapa-acao">
        <button type="button" className="botao botao-primario" disabled={selecionadas.length === 0} onClick={continuar}>
          Continuar para o radar
        </button>
        {selecionadas.length === 0 && <p className="dica">Marque ao menos uma atividade.</p>}
      </div>
    </div>
  )
}
