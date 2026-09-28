// Sobreposição do "assistir junto" no player: quem está na sala, o convite,
// as reações que sobem pela tela e o aviso quando alguém está carregando.
import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import { esquecerSala, type Conexao, type useSala } from '../lib/useSala'

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
  const [chatAberto, setChatAberto] = useState(false)
  const [lidas, setLidas] = useState(0)
  const e = sala.estado
  const link = `${window.location.origin}${window.location.pathname}?sala=${codigo}`
  const naoLidas = chatAberto ? 0 : Math.max(0, sala.chat.length - lidas)
  useEffect(() => {
    if (chatAberto) setLidas(sala.chat.length)
  }, [chatAberto, sala.chat.length])
  // Sair de propósito não deixa o "voltar para a sala" na Home.
  const sairDaSala = () => {
    esquecerSala()
    sair()
  }

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
                title={`${nome} · ${descreverConexao(nome === sala.eu ? undefined : sala.conexoes[nome])}`}
                className="relative grid h-6 w-6 place-items-center rounded-full bg-accent text-[11px] font-bold text-accent-ink uppercase ring-2 ring-black/60"
              >
                {nome.slice(0, 1)}
                {nome !== sala.eu && (
                  <span
                    aria-hidden="true"
                    className={`absolute -right-0.5 -bottom-0.5 h-2.5 w-2.5 rounded-full ring-2 ring-black/70 ${corDaConexao(sala.conexoes[nome])}`}
                  />
                )}
              </span>
            ))}
          </span>
          <button
            type="button"
            onClick={() => setChatAberto((a) => !a)}
            aria-pressed={chatAberto}
            className="relative rounded-full bg-white/15 px-3 py-1 font-medium transition hover:bg-white/25"
          >
            Chat
            {naoLidas > 0 && (
              <span className="absolute -top-1.5 -right-1.5 grid h-4 min-w-4 place-items-center rounded-full bg-accent px-1 text-[10px] font-bold text-accent-ink">
                {naoLidas}
              </span>
            )}
          </button>
          {sala.souDono && (
            <button
              type="button"
              onClick={() => sala.enviar({ tipo: 'modo', so_dono: !e?.so_dono })}
              aria-pressed={!!e?.so_dono}
              title="Modo cinema: só você controla play, pausa e pulos"
              className={`rounded-full px-3 py-1 font-medium transition ${e?.so_dono ? 'bg-accent text-accent-ink' : 'bg-white/15 hover:bg-white/25'}`}
            >
              🎬 Cinema
            </button>
          )}
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
        {e?.so_dono && !sala.souDono && (
          <p className="rounded-full bg-black/55 px-3 py-1 text-xs text-white/80 backdrop-blur-sm">🎬 {e.dono} controla o vídeo</p>
        )}
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

      <Contagem sala={sala} />

      {chatAberto ? (
        <ChatDaSala sala={sala} fechar={() => setChatAberto(false)} />
      ) : (
        <UltimasDoChat sala={sala} desde={lidas} abrir={() => setChatAberto(true)} />
      )}

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
              onClick={sairDaSala}
              className="w-full rounded-xl bg-white/10 py-3 font-semibold transition hover:bg-white/20"
            >
              Sair da sala
            </button>
            {e && eu && e.dono === eu.username && (
              <button
                type="button"
                onClick={() => {
                  sala.enviar({ tipo: 'encerrar' })
                  sairDaSala()
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
        <Dialogo titulo="A sala foi encerrada" fechar={sairDaSala}>
          <p className="text-sm text-white/60">{sala.fim} encerrou a sessão. Você pode continuar assistindo sozinho.</p>
          <button
            type="button"
            onClick={sairDaSala}
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

function corDaConexao(c?: Conexao) {
  if (!c || Date.now() - c.em > 15_000) return 'bg-neutral-500'
  if (c.travado || Math.abs(c.dif) > 2) return 'bg-red-500'
  if (Math.abs(c.dif) > 0.5) return 'bg-amber-400'
  return 'bg-emerald-400'
}

function descreverConexao(c?: Conexao) {
  if (!c) return 'você'
  if (Date.now() - c.em > 15_000) return 'sem notícias'
  if (c.travado) return 'carregando'
  const s = Math.abs(c.dif).toLocaleString('pt-BR', { maximumFractionDigits: 1 })
  return Math.abs(c.dif) < 0.5 ? 'em sincronia' : `${s} s ${c.dif > 0 ? 'atrás' : 'à frente'}`
}

/** "3, 2, 1" antes do play que vem depois de uma pausa longa. */
function Contagem({ sala }: { sala: ReturnType<typeof useSala> }) {
  const e = sala.estado
  const [falta, setFalta] = useState(0)
  useEffect(() => {
    if (!e?.tocando) {
      setFalta(0)
      return
    }
    const passo = () => setFalta(Math.max(0, e.em - sala.agoraServidor()))
    passo()
    const t = window.setInterval(passo, 100)
    return () => window.clearInterval(t)
  }, [e?.tocando, e?.em, sala])
  if (falta <= 0) return null
  const n = Math.ceil(falta / 1000)
  return (
    <div className="pointer-events-none absolute inset-0 z-20 grid place-items-center bg-black/40" role="status" aria-live="assertive">
      <span key={n} className="sala-contagem text-9xl font-bold text-white tabular-nums drop-shadow-2xl">
        {n}
      </span>
    </div>
  )
}

function ChatDaSala({ sala, fechar }: { sala: ReturnType<typeof useSala>; fechar: () => void }) {
  const [texto, setTexto] = useState('')
  const fim = useRef<HTMLDivElement>(null)
  useEffect(() => fim.current?.scrollIntoView({ block: 'end' }), [sala.chat.length])
  const mandar = (ev: React.FormEvent) => {
    ev.preventDefault()
    const t = texto.trim()
    if (!t) return
    sala.falar(t)
    setTexto('')
  }
  return (
    <aside className="absolute top-28 right-4 bottom-28 z-20 flex w-[min(20rem,calc(100%-2rem))] flex-col rounded-2xl border border-white/10 bg-black/75 text-white shadow-2xl backdrop-blur-md">
      <header className="flex items-center justify-between border-b border-white/10 px-4 py-2.5">
        <h2 className="text-sm font-semibold">Chat da sala</h2>
        <button type="button" onClick={fechar} aria-label="Fechar chat" className="grid h-7 w-7 place-items-center rounded-full text-white/60 hover:bg-white/10 hover:text-white">
          ✕
        </button>
      </header>
      <div className="flex-1 space-y-2 overflow-y-auto px-4 py-3 text-sm">
        {sala.chat.length === 0 && <p className="text-center text-white/40">Ninguém falou nada ainda.</p>}
        {sala.chat.map((l, i) => (
          <p key={`${l.em}-${i}`} className={l.de === sala.eu ? 'text-right' : ''}>
            <span className="block text-[11px] text-white/45">{l.de === sala.eu ? 'você' : l.de}</span>
            <span className={`inline-block max-w-full rounded-2xl px-3 py-1.5 break-words ${l.de === sala.eu ? 'bg-accent text-accent-ink' : 'bg-white/10'}`}>
              {l.texto}
            </span>
          </p>
        ))}
        <div ref={fim} />
      </div>
      <form onSubmit={mandar} className="flex gap-2 border-t border-white/10 p-2">
        <input
          value={texto}
          onChange={(ev) => setTexto(ev.target.value)}
          maxLength={300}
          placeholder="Escreva algo…"
          aria-label="Mensagem"
          autoFocus
          className="min-w-0 flex-1 rounded-full bg-white/10 px-4 py-2 text-sm outline-none placeholder:text-white/40 focus:bg-white/15"
        />
        <button type="submit" disabled={!texto.trim()} className="rounded-full bg-accent px-4 text-sm font-semibold text-accent-ink disabled:opacity-40">
          Enviar
        </button>
      </form>
    </aside>
  )
}

/** Com o chat fechado, as mensagens novas aparecem um instante no canto. */
function UltimasDoChat({ sala, desde, abrir }: { sala: ReturnType<typeof useSala>; desde: number; abrir: () => void }) {
  const [agora, setAgora] = useState(() => Date.now())
  useEffect(() => {
    const t = window.setInterval(() => setAgora(Date.now()), 1000)
    return () => window.clearInterval(t)
  }, [])
  const recentes = sala.chat
    .slice(desde)
    .filter((l) => l.de !== sala.eu && agora - (l.em - (sala.agoraServidor() - Date.now())) < 6000)
    .slice(-3)
  if (recentes.length === 0) return null
  return (
    <button type="button" onClick={abrir} className="absolute bottom-32 left-4 z-10 flex max-w-xs flex-col items-start gap-1.5 text-left">
      {recentes.map((l, i) => (
        <span key={`${l.em}-${i}`} className="sala-toast rounded-2xl bg-black/75 px-3 py-1.5 text-sm text-white shadow-lg backdrop-blur-sm">
          <b className="font-semibold">{l.de}:</b> {l.texto}
        </span>
      ))}
    </button>
  )
}
