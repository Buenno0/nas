// Sobreposição do "assistir junto" no player: quem está na sala, o convite,
// as reações que sobem pela tela e o aviso quando alguém está carregando.
import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import type { useSala } from '../lib/useSala'

const EMOJIS = ['😂', '😱', '😍', '👏', '😢', '🔥', '🍿', '👀']

export function PainelDaSala({
  codigo,
  sala,
  visivel,
  sair,
}: {
  codigo: string
  sala: ReturnType<typeof useSala>
  visivel: boolean
  sair: () => void
}) {
  const { data: eu } = useQuery({ queryKey: ['me'], queryFn: api.me })
  const [saindo, setSaindo] = useState(false)
  // Recém-criada, a sala já abre o convite: é o próximo passo de quem a criou.
  const [convite, setConvite] = useState(() => new URLSearchParams(window.location.search).has('novo'))
  const e = sala.estado
  const link = `${window.location.origin}${window.location.pathname}?sala=${codigo}`

  return (
    <>
      {/* Reações flutuando */}
      <div className="pointer-events-none absolute right-6 bottom-28 flex w-16 flex-col items-center" aria-live="polite">
        {sala.reacoes.map((r, i) => (
          <span
            key={r.id}
            className="sala-reacao absolute bottom-0 text-4xl"
            style={{ left: `${(i * 37) % 40}px` }}
            title={r.de}
          >
            {r.emoji}
          </span>
        ))}
      </div>

      {/* Painel no topo, à direita */}
      <div
        className={[
          'absolute top-4 right-4 z-10 flex max-w-[calc(100%-5rem)] flex-col items-end gap-2 text-white transition-opacity duration-300',
          visivel ? 'opacity-100' : 'pointer-events-none opacity-0',
        ].join(' ')}
      >
        <div className="flex items-center gap-2 rounded-full bg-black/55 py-1.5 pr-1.5 pl-3 text-xs backdrop-blur-sm">
          <span className="font-mono tracking-widest">{codigo}</span>
          <span className="text-white/60">·</span>
          <span className="flex -space-x-1.5">
            {(e?.presenca ?? []).map((nome) => (
              <span
                key={nome}
                title={nome}
                className="grid h-6 w-6 place-items-center rounded-full bg-accent text-[11px] font-bold text-accent-ink uppercase ring-2 ring-black/60"
              >
                {nome.slice(0, 1)}
              </span>
            ))}
          </span>
          <button
            type="button"
            onClick={() => setConvite(true)}
            className="rounded-full bg-white/15 px-3 py-1 font-medium transition hover:bg-white/25"
          >
            Convidar
          </button>
          <button
            type="button"
            onClick={() => setSaindo(true)}
            className="rounded-full bg-white/15 px-3 py-1 font-medium transition hover:bg-red-500/70"
          >
            Sair
          </button>
        </div>
        <div className="flex gap-1 rounded-full bg-black/55 p-1 backdrop-blur-sm">
          {EMOJIS.map((emoji) => (
            <button
              key={emoji}
              type="button"
              onClick={() => sala.enviar({ tipo: 'reacao', emoji })}
              aria-label={`Reagir com ${emoji}`}
              className="grid h-8 w-8 place-items-center rounded-full text-lg transition hover:scale-110 hover:bg-white/15"
            >
              {emoji}
            </button>
          ))}
        </div>
        {e?.por && (
          <p className="rounded-full bg-black/55 px-3 py-1 text-xs text-white/80 backdrop-blur-sm">
            {e.tocando ? '▶' : '❚❚'} por {e.por}
          </p>
        )}
      </div>

      {/* Toasts: quem entrou e quem saiu */}
      <div className="pointer-events-none absolute bottom-32 left-1/2 z-10 flex -translate-x-1/2 flex-col items-center gap-2" role="status" aria-live="polite">
        {sala.avisos.map((a) => (
          <p key={a.id} className="sala-toast rounded-full bg-black/75 px-4 py-2 text-sm whitespace-nowrap text-white shadow-lg backdrop-blur-sm">
            {a.texto}
          </p>
        ))}
      </div>

      {e?.aguardando && e.aguardando.length > 0 && (
        <div className="pointer-events-none absolute inset-x-0 top-20 flex justify-center">
          <p className="rounded-full bg-black/70 px-4 py-2 text-sm text-white backdrop-blur-sm">
            Esperando {e.aguardando.join(', ')} carregar…
          </p>
        </div>
      )}

      {sala.erro && (
        <div className="pointer-events-none absolute inset-x-0 top-20 flex justify-center">
          <p className="rounded-full bg-red-900/80 px-4 py-2 text-sm text-white">{sala.erro}</p>
        </div>
      )}

      {convite && <ModalDeConvite codigo={codigo} link={link} pessoas={e?.presenca.length ?? 0} fechar={() => setConvite(false)} />}

      {saindo && (
        <Dialogo titulo="Sair da sala?" fechar={() => setSaindo(false)}>
          <p className="text-sm text-white/60">
            {e && eu && e.dono === eu.username
              ? 'Você abriu esta sala. Pode sair e deixar os outros assistindo, ou encerrar para todo mundo.'
              : 'O vídeo continua daqui, só que sem sincronizar com a sala.'}
          </p>
          <div className="mt-5 flex flex-col gap-2">
            <button
              type="button"
              onClick={sair}
              className="w-full rounded-xl bg-white/10 py-3 font-semibold transition hover:bg-white/20"
            >
              Sair da sala
            </button>
            {e && eu && e.dono === eu.username && (
              <button
                type="button"
                onClick={() => {
                  sala.enviar({ tipo: 'encerrar' })
                  sair()
                }}
                className="w-full rounded-xl bg-red-600 py-3 font-semibold transition hover:bg-red-500"
              >
                Encerrar para todos
              </button>
            )}
          </div>
        </Dialogo>
      )}

      {sala.fim && (
        <Dialogo titulo="A sala foi encerrada" fechar={sair}>
          <p className="text-sm text-white/60">{sala.fim} encerrou a sessão. Você pode continuar assistindo sozinho.</p>
          <button
            type="button"
            onClick={sair}
            className="mt-5 w-full rounded-xl bg-accent py-3 font-semibold text-accent-ink transition hover:opacity-90"
          >
            Continuar sozinho
          </button>
        </Dialogo>
      )}

      {/* O navegador bloqueou o play automático: um toque resolve. */}
      {sala.bloqueado && (
        <button
          type="button"
          onClick={sala.entrar}
          className="absolute inset-0 z-20 grid place-items-center bg-black/60 text-white"
        >
          <span className="rounded-full bg-accent px-6 py-3 text-base font-semibold text-accent-ink">
            A sala já está tocando — toque para entrar
          </span>
        </button>
      )}
    </>
  )
}

function ModalDeConvite({
  codigo,
  link,
  pessoas,
  fechar,
}: {
  codigo: string
  link: string
  pessoas: number
  fechar: () => void
}) {
  const [copia, setCopia] = useState<'ok' | 'falhou'>()
  const campo = useRef<HTMLInputElement>(null)
  const podeCompartilhar = typeof navigator.share === 'function'

  useEffect(() => {
    const aoTeclar = (ev: KeyboardEvent) => {
      if (ev.key === 'Escape') {
        ev.stopPropagation()
        fechar()
      }
    }
    window.addEventListener('keydown', aoTeclar, true)
    return () => window.removeEventListener('keydown', aoTeclar, true)
  }, [fechar])

  const compartilhar = async () => {
    try {
      await navigator.share({ title: 'Assistir junto no Ozymandias', text: `Vem assistir comigo! Sala ${codigo}`, url: link })
      fechar()
    } catch {
      // cancelou: o modal continua aberto
    }
  }

  // navigator.clipboard só existe em HTTPS (ou localhost): pelo endereço
  // .local do Mac ele não está lá, então cai para o execCommand antigo.
  const copiar = async () => {
    let ok = false
    try {
      await navigator.clipboard.writeText(link)
      ok = true
    } catch {
      const el = campo.current
      if (el) {
        el.focus()
        el.select()
        el.setSelectionRange(0, link.length)
        try {
          ok = document.execCommand('copy')
        } catch {
          ok = false
        }
      }
    }
    setCopia(ok ? 'ok' : 'falhou')
    window.setTimeout(() => setCopia(undefined), 2500)
  }

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="convite-titulo"
      className="absolute inset-0 z-30 grid place-items-center bg-black/70 p-4 backdrop-blur-sm"
      onClick={fechar}
    >
      <div
        className="w-full max-w-sm rounded-2xl border border-white/10 bg-neutral-900 p-6 text-white shadow-2xl"
        onClick={(ev) => ev.stopPropagation()}
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 id="convite-titulo" className="text-lg font-semibold">
              Assistir junto
            </h2>
            <p className="mt-1 text-sm text-white/60">
              {pessoas > 1 ? `${pessoas} pessoas na sala` : 'Mande o convite para quem vai assistir com você.'}
            </p>
          </div>
          <button
            type="button"
            onClick={fechar}
            aria-label="Fechar"
            className="grid h-8 w-8 shrink-0 place-items-center rounded-full text-white/60 transition hover:bg-white/10 hover:text-white"
          >
            ✕
          </button>
        </div>

        <p className="mt-6 text-center font-mono text-4xl font-bold tracking-[0.3em]">{codigo}</p>
        <p className="mt-1 text-center text-xs text-white/50">código da sala</p>

        <div className="mt-6 flex items-center gap-2 rounded-xl bg-white/5 p-1.5 pl-3">
          <input
            ref={campo}
            readOnly
            value={link}
            onFocus={(ev) => ev.currentTarget.select()}
            aria-label="Link da sala"
            className="min-w-0 flex-1 bg-transparent text-sm text-white/80 outline-none"
          />
          <button
            type="button"
            onClick={() => void copiar()}
            className={[
              'shrink-0 rounded-lg px-3 py-1.5 text-xs font-medium transition',
              copia === 'ok' ? 'bg-emerald-500 text-white' : 'bg-white/10 hover:bg-white/20',
            ].join(' ')}
          >
            {copia === 'ok' ? 'Copiado ✓' : 'Copiar'}
          </button>
        </div>

        <p role="status" aria-live="polite" className={['mt-2 h-4 text-center text-xs', copia === 'falhou' ? 'text-amber-300' : 'text-emerald-400'].join(' ')}>
          {copia === 'ok' && 'Link copiado — é só colar na conversa.'}
          {copia === 'falhou' && 'Não deu para copiar sozinho: o link está selecionado, copie com ⌘C.'}
        </p>

        {podeCompartilhar ? (
          <button
            type="button"
            onClick={() => void compartilhar()}
            className="mt-4 w-full rounded-xl bg-accent py-3 text-base font-semibold text-accent-ink transition hover:opacity-90"
          >
            Compartilhar convite
          </button>
        ) : (
          <button
            type="button"
            onClick={() => void copiar()}
            className="mt-4 w-full rounded-xl bg-accent py-3 text-base font-semibold text-accent-ink transition hover:opacity-90"
          >
            {copia === 'ok' ? 'Link copiado ✓' : 'Copiar link'}
          </button>
        )}
        <p className="mt-3 text-center text-xs text-white/40">Quem abrir precisa ter uma conta no Ozymandias.</p>
      </div>
    </div>
  )
}

function Dialogo({ titulo, fechar, children }: { titulo: string; fechar: () => void; children: React.ReactNode }) {
  useEffect(() => {
    const aoTeclar = (ev: KeyboardEvent) => {
      if (ev.key === 'Escape') {
        ev.stopPropagation()
        fechar()
      }
    }
    window.addEventListener('keydown', aoTeclar, true)
    return () => window.removeEventListener('keydown', aoTeclar, true)
  }, [fechar])
  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={titulo}
      className="absolute inset-0 z-30 grid place-items-center bg-black/70 p-4 backdrop-blur-sm"
      onClick={fechar}
    >
      <div
        className="w-full max-w-sm rounded-2xl border border-white/10 bg-neutral-900 p-6 text-white shadow-2xl"
        onClick={(ev) => ev.stopPropagation()}
      >
        <h2 className="text-lg font-semibold">{titulo}</h2>
        <div className="mt-2">{children}</div>
      </div>
    </div>
  )
}
