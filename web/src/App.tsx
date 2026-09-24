import { useEffect, useRef, useState } from 'react'
import { contar, ErroDaAPI, health, pesquisar, type Contagem, type Filtros, type Health, type Lead, type Pagina } from './api'
import type { Atalho } from './catalogo'
import { carregarCatalogo, nomeCurto, type Catalogo } from './cnae'
import { leadsParaCSV } from './csv'
import { contarDemo, healthDemo, pesquisarDemo } from './demo'
import { EtapaAtividades } from './EtapaAtividades'
import { EtapaRamo } from './EtapaRamo'
import { formatarData, formatarNumero, listaPorExtenso } from './formato'
import { IconeChave, IconeLua, IconeSol } from './icones'
import { Radar, type FiltrosRadar } from './Radar'
import { Esqueleto, Paginacao, Resumo, Tabela, Vazio } from './Resultados'

const POR_PAGINA = 50
// Exportar pagina por cursor com o teto do servidor (1.000 por resposta).
// O limite de paginas evita que um filtro amplo consuma a cota do dia.
const EXPORT_POR_PAGINA = 1000
const EXPORT_MAX_PAGINAS = 10
// A varredura roda pelo menos isto antes de mostrar o resultado: e o tempo de
// uma volta e meia, suficiente para ver o radar trabalhar sem fazer esperar.
const VARREDURA_MINIMA_MS = 1500
// Quando o radar ja escaneou e a pessoa muda um filtro, reescaneia sozinho:
// a espera junta cliques seguidos, e a varredura fica curta para nao atrasar.
const ESPERA_AUTOMATICA_MS = 350
const VARREDURA_AUTOMATICA_MS = 500

const CHAVE_SESSAO = 'fonteca.api-key'

type Modo = 'demo' | 'real'
type Tema = 'escuro' | 'claro'
type Etapa = 1 | 2 | 3

// O script do index.html ja aplicou o tema antes da primeira pintura.
function temaInicial(): Tema {
  return document.documentElement.dataset.tema === 'claro' ? 'claro' : 'escuro'
}

const radarInicial: FiltrosRadar = {
  ufs: [],
  situacao: 'ATIVA',
  ultimosDias: 90,
  somenteCelular: true,
  excluirMEI: true,
}

function lerSessao(): string {
  try {
    return sessionStorage.getItem(CHAVE_SESSAO) ?? ''
  } catch {
    return ''
  }
}

function gravarSessao(chave: string) {
  try {
    if (chave) sessionStorage.setItem(CHAVE_SESSAO, chave)
    else sessionStorage.removeItem(CHAVE_SESSAO)
  } catch {
    // navegador sem storage: a chave so vale ate recarregar
  }
}

function mensagemDeErro(e: unknown): string {
  if (e instanceof ErroDaAPI) {
    switch (e.status) {
      case 401:
        return 'A chave de API não foi aceita. Confira se ela foi copiada inteira.'
      case 429:
        return 'Limite de consultas atingido. Espere um minuto e tente de novo.'
      case 503:
        return 'Ainda não há lote importado. Rode a ingestão da Receita antes de pesquisar.'
      case 400:
        return `Filtro inválido: ${e.message}.`
    }
    return e.correlacao
      ? `O servidor falhou ao responder. Código para o suporte: ${e.correlacao}.`
      : `O servidor falhou ao responder (${e.message}).`
  }
  if (e instanceof DOMException && e.name === 'AbortError') return ''
  return 'Não foi possível falar com a API. Ela está no ar?'
}

function baixar(nome: string, conteudo: string) {
  const url = URL.createObjectURL(new Blob([conteudo], { type: 'text/csv;charset=utf-8' }))
  const a = document.createElement('a')
  a.href = url
  a.download = nome
  a.click()
  URL.revokeObjectURL(url)
}

const esperar = (ms: number) => new Promise((ok) => setTimeout(ok, ms))

export default function App() {
  const [chave, setChave] = useState(lerSessao)
  const [rascunhoChave, setRascunhoChave] = useState('')
  const [modo, setModo] = useState<Modo>(() => (lerSessao() ? 'real' : 'demo'))
  const [editandoChave, setEditandoChave] = useState(false)
  const [tema, setTema] = useState<Tema>(temaInicial)

  const [catalogo, setCatalogo] = useState<Catalogo | null>(null)
  const [etapa, setEtapa] = useState<Etapa>(1)
  const [divisao, setDivisao] = useState<string | null>(null)
  const [cnaes, setCnaes] = useState<string[]>([])
  const [radar, setRadar] = useState<FiltrosRadar>(radarInicial)

  const [filtros, setFiltros] = useState<Filtros | null>(null)
  const [contagem, setContagem] = useState<Contagem | null>(null)
  const [escaneando, setEscaneando] = useState(false)

  // null = nenhuma pagina carregada ainda (ou a primeira falhou): a tabela
  // nao aparece, para um erro nao se passar por "nenhum lead".
  const [leads, setLeads] = useState<Lead[] | null>(null)
  // cursores[i] e o cursor que abre a pagina i; a pagina 0 nao tem cursor.
  const [cursores, setCursores] = useState<(string | undefined)[]>([undefined])
  const [pagina, setPagina] = useState(0)
  const [proximo, setProximo] = useState<string | undefined>()

  const [carregando, setCarregando] = useState(false)
  const [exportando, setExportando] = useState<string | null>(null)
  const [erro, setErro] = useState('')
  const [aviso, setAviso] = useState('')
  const [lote, setLote] = useState<Health['lote'] | undefined>()

  const emCurso = useRef<AbortController | null>(null)
  const reescanear = useRef<number | undefined>(undefined)
  const refEtapa2 = useRef<HTMLLIElement>(null)
  const refEtapa3 = useRef<HTMLLIElement>(null)
  const refResultados = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let vivo = true
    carregarCatalogo()
      .then((c) => vivo && setCatalogo(c))
      .catch(() => vivo && setErro('Não foi possível carregar o catálogo de atividades.'))
    return () => {
      vivo = false
    }
  }, [])

  useEffect(() => {
    if (modo === 'demo') return
    const c = new AbortController()
    health(c.signal)
      .then((h) => setLote(h.lote))
      .catch(() => setLote(undefined))
    return () => c.abort()
  }, [modo])

  function rolarPara(ref: React.RefObject<HTMLElement | null>) {
    requestAnimationFrame(() => ref.current?.scrollIntoView({ block: 'start', behavior: 'smooth' }))
  }

  function buscarPagina(f: Filtros, limite: number, cursor?: string, sinal?: AbortSignal): Promise<Pagina> {
    if (modo === 'demo') return pesquisarDemo(f, limite, cursor)
    return pesquisar(chave, f, limite, cursor, sinal)
  }

  function buscarContagem(f: Filtros, sinal?: AbortSignal): Promise<Contagem> {
    if (modo === 'demo') return contarDemo(f)
    return contar(chave, f, sinal)
  }

  // ---- Etapa 1: ramo ----

  function escolherDivisao(codigo: string) {
    const d = catalogo?.porDivisao.get(codigo)
    if (!d) return
    setDivisao(codigo)
    // O ramo inteiro entra marcado; a pessoa desmarca o que nao interessa.
    setCnaes(d.grupos.flatMap((g) => g.subclasses.map((s) => s.codigo)))
    setEtapa(2)
    rolarPara(refEtapa2)
  }

  function escolherSubclasse(codigo: string) {
    setDivisao(codigo.slice(0, 2))
    setCnaes((cs) => (cs.includes(codigo) ? cs : [...cs, codigo]))
    setEtapa(2)
    rolarPara(refEtapa2)
  }

  function usarAtalho(a: Atalho) {
    setDivisao(a.cnaes[0].slice(0, 2))
    setCnaes(a.cnaes)
    const novo = { ...radar, ufs: a.ufs, ultimosDias: a.ultimosDias }
    setRadar(novo)
    setEtapa(3)
    void escanear(a.cnaes, novo)
    rolarPara(refEtapa3)
  }

  // ---- Etapa 3: radar e busca ----

  async function carregar(f: Filtros, indice: number, cursor: string | undefined) {
    emCurso.current?.abort()
    const c = new AbortController()
    emCurso.current = c
    setCarregando(true)
    setErro('')
    try {
      const r = await buscarPagina(f, POR_PAGINA, cursor, c.signal)
      if (c.signal.aborted) return
      setLeads(r.resultados)
      setPagina(indice)
      setProximo(r.proximo_cursor)
      if (r.proximo_cursor) {
        setCursores((cs) => {
          const novo = cs.slice(0, indice + 1)
          novo[indice + 1] = r.proximo_cursor
          return novo
        })
      }
      if (indice > 0) rolarPara(refResultados)
    } catch (e) {
      const m = mensagemDeErro(e)
      if (m) setErro(m)
    } finally {
      if (emCurso.current === c) setCarregando(false)
    }
  }

  // Mudar filtro depois do primeiro escaneamento atualiza sozinho. Se so os
  // estados mudaram, o mapa (que conta o Brasil inteiro) nao muda: basta
  // recarregar a lista.
  function mudarRadar(novo: FiltrosRadar) {
    const anterior = radar
    setRadar(novo)
    if (!contagem || !filtros) return
    const soEstados =
      anterior.ultimosDias === novo.ultimosDias &&
      anterior.situacao === novo.situacao &&
      anterior.somenteCelular === novo.somenteCelular &&
      anterior.excluirMEI === novo.excluirMEI
    window.clearTimeout(reescanear.current)
    reescanear.current = window.setTimeout(() => {
      if (soEstados) {
        const alvo: Filtros = { ...filtros, ufs: novo.ufs }
        setFiltros(alvo)
        setCursores([undefined])
        void carregar(alvo, 0, undefined)
      } else {
        void escanear(cnaes, novo, VARREDURA_AUTOMATICA_MS)
      }
    }, ESPERA_AUTOMATICA_MS)
  }

  async function escanear(codigos = cnaes, r = radar, minimo = VARREDURA_MINIMA_MS) {
    if (modo === 'real' && !chave) {
      setEditandoChave(true)
      setErro('Informe a chave de API para escanear os dados reais, ou use a demonstração.')
      return
    }
    if (codigos.length === 0) return
    emCurso.current?.abort()
    const c = new AbortController()
    emCurso.current = c
    const alvo: Filtros = { cnaes: codigos, ...r }
    setFiltros(alvo)
    // Reescaneamento automatico mantem a lista anterior (esmaecida) ate a
    // nova chegar, em vez de piscar o esqueleto a cada clique.
    if (minimo === VARREDURA_MINIMA_MS) setLeads(null)
    setCursores([undefined])
    setErro('')
    setAviso('')
    setEscaneando(true)
    setCarregando(true)
    try {
      // O mapa conta o Brasil inteiro, para mostrar onde ha empresa alem do
      // filtro; a lista respeita os estados escolhidos.
      const [cont, pag] = await Promise.all([
        buscarContagem({ ...alvo, ufs: [] }, c.signal),
        buscarPagina(alvo, POR_PAGINA, undefined, c.signal),
        esperar(minimo),
      ])
      if (c.signal.aborted) return
      setContagem(cont)
      setLeads(pag.resultados)
      setPagina(0)
      setProximo(pag.proximo_cursor)
      if (pag.proximo_cursor) setCursores([undefined, pag.proximo_cursor])
    } catch (e) {
      const m = mensagemDeErro(e)
      if (m) setErro(m)
    } finally {
      if (emCurso.current === c) {
        setEscaneando(false)
        setCarregando(false)
      }
    }
  }

  // ---- Modo, chave, tema ----

  function trocarModo(m: Modo) {
    emCurso.current?.abort()
    setModo(m)
    setFiltros(null)
    setLeads(null)
    setContagem(null)
    setErro('')
    setAviso('')
    if (m === 'real' && !chave) setEditandoChave(true)
  }

  function alternarTema() {
    const novo: Tema = tema === 'escuro' ? 'claro' : 'escuro'
    setTema(novo)
    document.documentElement.dataset.tema = novo
    try {
      localStorage.setItem('fonteca.tema', novo)
    } catch {
      // sem storage o tema vale so ate recarregar
    }
  }

  function salvarChave() {
    const k = rascunhoChave.trim()
    if (!k) return
    setChave(k)
    gravarSessao(k)
    setRascunhoChave('')
    setEditandoChave(false)
    setErro('')
    if (modo !== 'real') trocarModo('real')
  }

  async function exportar() {
    if (!filtros) return
    setErro('')
    setAviso('')
    const todos: Lead[] = []
    let cursor: string | undefined
    try {
      for (let i = 0; i < EXPORT_MAX_PAGINAS; i++) {
        setExportando(i === 0 ? 'Preparando…' : `Página ${i + 1}…`)
        const r = await buscarPagina(filtros, EXPORT_POR_PAGINA, cursor)
        todos.push(...r.resultados)
        cursor = r.proximo_cursor
        if (!cursor) break
      }
      const data = new Date().toISOString().slice(0, 10)
      baixar(`fonteca-leads-${data}${modo === 'demo' ? '-demonstracao' : ''}.csv`, leadsParaCSV(todos))
      setAviso(
        cursor
          ? `Planilha com as primeiras ${formatarNumero(todos.length)} empresas. Refine o filtro para exportar o restante.`
          : `Planilha com ${formatarNumero(todos.length)} empresas baixada.`,
      )
    } catch (e) {
      const m = mensagemDeErro(e)
      if (m) setErro(m)
    } finally {
      setExportando(null)
    }
  }

  // ---- Textos de resumo ----

  const d = divisao ? catalogo?.porDivisao.get(divisao) : undefined
  const descricaoAtividades = (() => {
    if (!catalogo || cnaes.length === 0) return ''
    if (cnaes.length === 1) return nomeCurto(catalogo.porCodigo.get(cnaes[0])?.descricao ?? cnaes[0])
    if (d && cnaes.length === d.total && cnaes.every((c) => c.startsWith(d.codigo))) return nomeCurto(d.descricao)
    if (cnaes.length <= 3) {
      // Nomes oficiais longos viram uma frase ilegivel no titulo; a lista
      // completa ja esta na etapa 2.
      const frase = listaPorExtenso(cnaes.map((c) => nomeCurto(catalogo.porCodigo.get(c)?.descricao ?? c)))
      if (frase.length <= 48 && !frase.includes('…')) return frase
    }
    return `${cnaes.length} atividades`
  })()

  const hoje = new Date()
  const loteExibido = modo === 'demo' ? healthDemo().lote : lote

  return (
    <div className="app">
      <header className="topo">
        <a className="marca" href="/" aria-label="Fonteca, página inicial">
          <img className="marca-simbolo" src="/marca/simbolo.png" alt="" width="277" height="278" />
          <span className="marca-textos">
            <img
              className="marca-logotipo"
              src={tema === 'escuro' ? '/marca/logotipo-claro.png' : '/marca/logotipo-escuro.png'}
              alt="Fonteca"
              width="956"
              height="139"
            />
            <img
              className="marca-assinatura"
              src={tema === 'escuro' ? '/marca/assinatura-clara.png' : '/marca/assinatura-escura.png'}
              alt="Dados oficiais. Decisões melhores."
              width="966"
              height="45"
            />
          </span>
        </a>

        <div className="acesso">
          <div className="modo" role="radiogroup" aria-label="Origem dos dados">
            <button type="button" role="radio" aria-checked={modo === 'demo'} onClick={() => trocarModo('demo')}>
              Demonstração
            </button>
            <button type="button" role="radio" aria-checked={modo === 'real'} onClick={() => trocarModo('real')}>
              Dados reais
            </button>
          </div>
          <button
            type="button"
            className="tema"
            onClick={alternarTema}
            aria-label={tema === 'escuro' ? 'Usar tema claro' : 'Usar tema escuro'}
            title={tema === 'escuro' ? 'Tema claro' : 'Tema escuro'}
          >
            {tema === 'escuro' ? <IconeSol /> : <IconeLua />}
          </button>
          {modo === 'real' && chave && !editandoChave && (
            <button type="button" className="link-discreto" onClick={() => setEditandoChave(true)}>
              <IconeChave /> Trocar chave
            </button>
          )}
        </div>
      </header>

      {editandoChave && (
        <form
          className="faixa-chave"
          onSubmit={(e) => {
            e.preventDefault()
            salvarChave()
          }}
        >
          <label htmlFor="chave">
            <IconeChave /> Chave de API
          </label>
          <input
            id="chave"
            type="password"
            autoComplete="off"
            spellCheck={false}
            placeholder="fnt_live_…"
            value={rascunhoChave}
            onChange={(e) => setRascunhoChave(e.target.value)}
            autoFocus
          />
          <button type="submit" className="botao botao-primario" disabled={!rascunhoChave.trim()}>
            Usar chave
          </button>
          <button
            type="button"
            className="botao botao-leve"
            onClick={() => {
              setEditandoChave(false)
              if (!chave) trocarModo('demo')
            }}
          >
            Cancelar
          </button>
          <p className="faixa-chave-nota">A chave fica só nesta aba do navegador e some ao fechá-la.</p>
        </form>
      )}

      {modo === 'demo' && (
        <p className="faixa-demo" role="note">
          <strong>Demonstração.</strong> Empresas, telefones e e-mails desta tela são fictícios. Nenhum contato real é
          exibido.
        </p>
      )}

      <section className="abertura">
        <h1>Encontre as empresas que acabaram de abrir no seu mercado.</h1>
        <p>
          Escolha o ramo, ajuste as atividades e ligue o radar. A Fonteca varre o cadastro oficial da Receita Federal e
          devolve quem abriu agora, com celular para chamar no WhatsApp.
        </p>
      </section>

      {erro && (
        <p className="alerta" role="alert">
          {erro}
        </p>
      )}

      <ol className="etapas">
        <li className={etapa === 1 ? 'etapa ativa' : 'etapa'}>
          <header className="etapa-cabeca">
            <span className="etapa-numero" aria-hidden="true">1</span>
            <div className="etapa-titulos">
              <h2>Ramo</h2>
              {etapa !== 1 && d && <p className="etapa-resumo">{d.descricao}</p>}
              {etapa === 1 && <p className="etapa-resumo">Qual é o seu mercado?</p>}
            </div>
            {etapa !== 1 && (
              <button type="button" className="botao botao-leve" onClick={() => setEtapa(1)}>
                Trocar ramo
              </button>
            )}
          </header>
          {etapa === 1 && (
            <EtapaRamo
              catalogo={catalogo}
              escolherDivisao={escolherDivisao}
              escolherSubclasse={escolherSubclasse}
              usarAtalho={usarAtalho}
            />
          )}
        </li>

        <li className={etapa === 2 ? 'etapa ativa' : etapa > 2 ? 'etapa' : 'etapa futura'} ref={refEtapa2}>
          <header className="etapa-cabeca">
            <span className="etapa-numero" aria-hidden="true">2</span>
            <div className="etapa-titulos">
              <h2>Atividades</h2>
              <p className="etapa-resumo">
                {cnaes.length === 0
                  ? 'Os códigos CNAE do ramo escolhido'
                  : `${cnaes.length} ${cnaes.length === 1 ? 'atividade selecionada' : 'atividades selecionadas'}`}
              </p>
            </div>
            {etapa === 3 && (
              <button type="button" className="botao botao-leve" onClick={() => setEtapa(2)}>
                Ajustar
              </button>
            )}
          </header>
          {etapa === 2 && catalogo && divisao && (
            <EtapaAtividades
              catalogo={catalogo}
              divisao={divisao}
              selecionadas={cnaes}
              mudar={setCnaes}
              continuar={() => {
                setEtapa(3)
                rolarPara(refEtapa3)
              }}
            />
          )}
        </li>

        <li className={etapa === 3 ? 'etapa ativa' : 'etapa futura'} ref={refEtapa3}>
          <header className="etapa-cabeca">
            <span className="etapa-numero" aria-hidden="true">3</span>
            <div className="etapa-titulos">
              <h2>Radar</h2>
              <p className="etapa-resumo">Onde, desde quando e com qual contato</p>
            </div>
          </header>
          {etapa === 3 && (
            <Radar valor={radar} mudar={mudarRadar} contagem={contagem} escaneando={escaneando} escanear={() => escanear()} />
          )}
        </li>
      </ol>

      <div className="resultados-area" ref={refResultados}>
        {aviso && (
          <p className="aviso" role="status">
            {aviso}
          </p>
        )}

        {filtros && leads === null && carregando && <Esqueleto />}

        {filtros && leads && (
          <div className={carregando ? 'resultados atualizando' : 'resultados'}>
            <Resumo
              filtros={filtros}
              atividades={descricaoAtividades}
              total={
                contagem
                  ? filtros.ufs.length === 0
                    ? contagem.total
                    : filtros.ufs.reduce((n, uf) => n + (contagem.por_uf[uf] ?? 0), 0)
                  : undefined
              }
              leads={leads}
              pagina={pagina}
              temMais={Boolean(proximo)}
              aoExportar={exportar}
              exportando={exportando}
            />
            {leads.length === 0 ? (
              <Vazio
                aoLimpar={() => {
                  const novo = { ...radar, ufs: [], ultimosDias: 365 }
                  setRadar(novo)
                  void escanear(cnaes, novo)
                }}
              />
            ) : (
              <>
                <Tabela leads={leads} hoje={hoje} key={`${pagina}-${leads[0]?.cnpj}`} />
                <Paginacao
                  pagina={pagina}
                  quantidade={leads.length}
                  temAnterior={pagina > 0}
                  temProxima={Boolean(proximo)}
                  carregando={carregando}
                  anterior={() => carregar(filtros, pagina - 1, cursores[pagina - 1])}
                  proxima={() => carregar(filtros, pagina + 1, proximo)}
                />
              </>
            )}
          </div>
        )}
      </div>

      <footer className="colofao">
        <p>
          <span className="colofao-rotulo">Fonte</span> Cadastro Nacional da Pessoa Jurídica, dados abertos da Receita
          Federal. Classificação de atividades: CNAE 2.3, IBGE.
          {modo === 'real' && loteExibido === undefined && ' Status da base indisponível.'}
          {modo === 'real' && loteExibido === null && ' Nenhum lote importado ainda.'}
          {modo === 'real' &&
            loteExibido &&
            ` Lote ${loteExibido.referencia}, importado em ${formatarData(loteExibido.importado_em)} · ${formatarNumero(loteExibido.linhas)} empresas.`}
          {modo === 'demo' && ' Nesta demonstração as empresas são geradas e não existem.'}
        </p>
        <p>A Fonteca não tem vínculo com a Receita Federal.</p>
      </footer>
    </div>
  )
}
