import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import { Link } from 'react-router'
import { useEnvios } from '../lib/envios'
import { humanSize } from '../lib/format'
import { NuvemDeEnvio } from './NuvemDeEnvio'
import { ListaDeEnvios } from './ListaDeEnvios'

const CHAVE_POSICAO = 'nas-cartao-envios'
const MARGEM = 12
const LARGURA = 320

interface Posicao {
  x: number
  y: number
}

// A posição é conveniência deste navegador: some numa aba anônima ou com o
// armazenamento bloqueado, e o cartão volta ao canto. Nunca pode quebrar.
function lerPosicao(): Posicao | null {
  try {
    const p = JSON.parse(localStorage.getItem(CHAVE_POSICAO) ?? 'null') as Posicao | null
    return p && Number.isFinite(p.x) && Number.isFinite(p.y) ? p : null
  } catch {
    return null
  }
}

function gravarPosicao(p: Posicao) {
  try {
    localStorage.setItem(CHAVE_POSICAO, JSON.stringify(p))
  } catch {
    // sem armazenamento: a posição vale só até recarregar
  }
}

/** Mantém o cartão inteiro dentro da janela, mesmo depois de ela encolher. */
function dentro(p: Posicao, el: HTMLElement | null): Posicao {
  const w = el?.offsetWidth ?? LARGURA
  const h = el?.offsetHeight ?? 200
  return {
    x: Math.min(Math.max(MARGEM, p.x), Math.max(MARGEM, window.innerWidth - w - MARGEM)),
    y: Math.min(Math.max(MARGEM, p.y), Math.max(MARGEM, window.innerHeight - h - MARGEM)),
  }
}

// Canto superior direito, logo abaixo da barra do topo.
const padrao = (): Posicao => ({ x: window.innerWidth - LARGURA - 16, y: 72 })

/**
 * O cartão flutuante dos envios: aparece em qualquer tela do app enquanto há
 * um lote, com a nuvem, a previsão e a lista. Arrasta pelo cabeçalho (ou com
 * as setas, pelo teclado) e lembra onde ficou. Fechar esconde o lote atual;
 * um lote novo traz o cartão de volta.
 */
export function CartaoDeEnvios() {
  const { linhas, velocidades, lote } = useEnvios()
  const cartao = useRef<HTMLDivElement>(null)
  const [pos, setPosEstado] = useState<Posicao>(() => lerPosicao() ?? padrao())
  // A posição também numa ref: o soltar pode chegar antes do render do
  // último movimento, e gravaria a posição de um passo atrás.
  const posRef = useRef(pos)
  const setPos = (p: Posicao) => {
    posRef.current = p
    setPosEstado(p)
  }
  const [fechadoNoLote, setFechadoNoLote] = useState(0)
  const arrasto = useRef<{ dx: number; dy: number } | null>(null)

  // A janela encolheu (ou o cartão cresceu): puxa de volta para dentro.
  useEffect(() => {
    const ajusta = () => setPos(dentro(posRef.current, cartao.current))
    ajusta()
    window.addEventListener('resize', ajusta)
    return () => window.removeEventListener('resize', ajusta)
  }, [lote?.linhas.length])

  if (!lote || fechadoNoLote === lote.numero) return null
  const doLote = linhas.filter((l) => l.lote === lote.numero)

  const comeca = (e: PointerEvent<HTMLDivElement>) => {
    // Botões e links do cabeçalho continuam clicáveis; a alça é botão (para o
    // teclado) mas é justamente por onde se arrasta.
    const alvo = (e.target as HTMLElement).closest('button, a')
    if (alvo && !alvo.hasAttribute('data-alca')) return
    e.preventDefault()
    arrasto.current = { dx: e.clientX - posRef.current.x, dy: e.clientY - posRef.current.y }
    try {
      e.currentTarget.setPointerCapture(e.pointerId)
    } catch {
      // sem captura o arrasto ainda funciona enquanto o ponteiro estiver em cima
    }
  }
  const move = (e: PointerEvent<HTMLDivElement>) => {
    if (!arrasto.current) return
    setPos(dentro({ x: e.clientX - arrasto.current.dx, y: e.clientY - arrasto.current.dy }, cartao.current))
  }
  const solta = () => {
    if (!arrasto.current) return
    arrasto.current = null
    gravarPosicao(posRef.current)
  }
  const teclado = (e: KeyboardEvent<HTMLButtonElement>) => {
    const passo = e.shiftKey ? 48 : 12
    const d = { ArrowLeft: [-passo, 0], ArrowRight: [passo, 0], ArrowUp: [0, -passo], ArrowDown: [0, passo] }[e.key]
    if (!d) return
    e.preventDefault()
    const novo = dentro({ x: posRef.current.x + d[0], y: posRef.current.y + d[1] }, cartao.current)
    setPos(novo)
    gravarPosicao(novo)
  }

  const titulo = lote.ativo ? 'Enviando para a nuvem' : lote.progresso >= 1 ? 'Na nuvem' : 'Envio pausado'
  return (
    <div
      ref={cartao}
      role="region"
      aria-label="Envios para a nuvem"
      className="fixed z-40 overflow-hidden rounded-xl border border-line bg-surface/95 shadow-2xl shadow-black/40 backdrop-blur-md"
      style={{ left: pos.x, top: pos.y, width: LARGURA }}
    >
      <div
        onPointerDown={comeca}
        onPointerMove={move}
        onPointerUp={solta}
        onPointerCancel={solta}
        className="flex cursor-grab touch-none items-center gap-2 border-b border-line px-3 py-2 select-none active:cursor-grabbing"
      >
        <button
          type="button"
          data-alca
          onKeyDown={teclado}
          aria-label="Mover o cartão (setas do teclado)"
          className="grid h-7 w-5 shrink-0 cursor-grab place-items-center rounded text-muted hover:text-ink"
        >
          <svg width="10" height="16" viewBox="0 0 10 16" fill="currentColor" aria-hidden="true">
            {[2, 8].map((x) => [3, 8, 13].map((y) => <circle key={`${x}-${y}`} cx={x} cy={y} r="1.3" />))}
          </svg>
        </button>
        <span className="min-w-0 flex-1 truncate text-sm font-semibold">{titulo}</span>
        <Link to="/settings" className="shrink-0 text-xs text-muted hover:text-ink">
          ver tudo
        </Link>
        <button
          type="button"
          onClick={() => setFechadoNoLote(lote.numero)}
          aria-label="Fechar"
          className="grid h-7 w-7 shrink-0 place-items-center rounded-md text-muted transition hover:bg-elev hover:text-ink"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
            <path d="M6 6l12 12M18 6 6 18" />
          </svg>
        </button>
      </div>

      <div className="flex flex-col items-center gap-1 px-3 pt-2">
        <div className="w-full max-w-[240px]">
          <NuvemDeEnvio progresso={lote.progresso} ativo={lote.ativo} />
        </div>
        <p className="font-mono text-[11px] text-muted" aria-live="polite">
          {lote.ativo
            ? `${Math.round(lote.progresso * 100)}% de ${humanSize(lote.total)}${lote.falta ? ` · faltam ${lote.falta}` : ' · calculando…'}`
            : lote.progresso >= 1
              ? 'tudo na nuvem'
              : 'pausado: volta com o híbrido'}
        </p>
      </div>

      <div className="max-h-56 overflow-y-auto p-3 pt-2">
        <ListaDeEnvios linhas={doLote} velocidades={velocidades} compacta />
      </div>
    </div>
  )
}
