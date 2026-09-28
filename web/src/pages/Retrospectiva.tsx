// "Seu ano no Ozymandias": o histórico do ano em cartões, do jeito das
// retrospectivas de streaming, com a nuvem que cresce conforme as horas.
import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api, type TituloNaRetrospectiva } from '../lib/api'
import { gradientFor } from '../lib/format'
import { EmptyState, Spinner } from '../components/states'

const CUMULO = 'M70 170 C30 170 20 130 50 118 C44 84 84 66 110 82 C120 46 170 36 196 64 C214 30 280 30 292 78 C330 70 364 96 352 130 C380 140 376 172 344 172 Z'
const CUMULO_BORDA = 'M50 118 C44 84 84 66 110 82 C120 46 170 36 196 64 C214 30 280 30 292 78 C330 70 364 96 352 130'
const MESES = ['jan', 'fev', 'mar', 'abr', 'mai', 'jun', 'jul', 'ago', 'set', 'out', 'nov', 'dez']

const horas = (s: number) => Math.round(s / 3600)
const dataCurta = (dia?: string) =>
  dia ? new Date(`${dia}T12:00:00`).toLocaleDateString('pt-BR', { day: 'numeric', month: 'long' }) : ''

/** Conta de 0 até o valor quando o cartão aparece na tela. */
function Contador({ valor, sufixo = '' }: { valor: number; sufixo?: string }) {
  const ref = useRef<HTMLSpanElement>(null)
  const [mostrado, setMostrado] = useState(0)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      setMostrado(valor)
      return
    }
    let quadro = 0
    const obs = new IntersectionObserver(([e]) => {
      if (!e.isIntersecting) return
      obs.disconnect()
      const inicio = performance.now()
      const passo = (t: number) => {
        const f = Math.min(1, (t - inicio) / 1400)
        setMostrado(Math.round(valor * (1 - Math.pow(1 - f, 3))))
        if (f < 1) quadro = requestAnimationFrame(passo)
      }
      quadro = requestAnimationFrame(passo)
    })
    obs.observe(el)
    return () => {
      obs.disconnect()
      cancelAnimationFrame(quadro)
    }
  }, [valor])
  return (
    <span ref={ref} className="tabular-nums">
      {mostrado.toLocaleString('pt-BR')}
      {sufixo}
    </span>
  )
}

function Cartao({ rotulo, children }: { rotulo: string; children: React.ReactNode }) {
  return (
    <section className="flex min-h-[70vh] snap-start flex-col justify-center gap-6 rounded-2xl border border-line bg-surface px-6 py-10 sm:px-10">
      <p className="font-mono text-xs tracking-[0.28em] text-accent uppercase">{rotulo}</p>
      {children}
    </section>
  )
}

function Poster({ t, grande = false }: { t: TituloNaRetrospectiva; grande?: boolean }) {
  const tamanho = grande ? 'w-40 sm:w-48' : 'w-24 sm:w-28'
  return t.poster ? (
    <img src={t.poster} alt="" className={`${tamanho} aspect-[2/3] shrink-0 rounded-xl object-cover shadow-xl shadow-black/40`} />
  ) : (
    <div className={`${tamanho} aspect-[2/3] shrink-0 rounded-xl`} style={{ background: gradientFor(t.nome) }} aria-hidden="true" />
  )
}

/** A nuvem cresce com as horas do ano (0 h pequena, 300 h+ cheia). */
function NuvemDoAno({ segundos }: { segundos: number }) {
  const f = Math.min(1, horas(segundos) / 300)
  return (
    <svg viewBox="0 0 400 200" className="mx-auto h-auto w-full max-w-md overflow-visible" aria-hidden="true">
      <defs>
        <radialGradient id="retro-brilho">
          <stop offset="0%" stopColor="var(--accent)" stopOpacity={0.08 + 0.2 * f} />
          <stop offset="100%" stopColor="var(--accent)" stopOpacity={0} />
        </radialGradient>
      </defs>
      <ellipse cx="200" cy="120" rx="230" ry="110" fill="url(#retro-brilho)" />
      <g className="ce-respira" style={{ transform: `scale(${(0.6 + 0.5 * f).toFixed(3)})`, transformOrigin: '200px 120px' }}>
        <path d={CUMULO} fill="var(--elev)" />
        <path d={CUMULO_BORDA} fill="none" stroke="var(--ink)" strokeOpacity={0.3 + 0.6 * f} strokeWidth={2.6} strokeLinecap="round" />
      </g>
    </svg>
  )
}

export function Retrospectiva() {
  const agora = new Date().getFullYear()
  const [ano, setAno] = useState(agora)
  const { data, isLoading } = useQuery({ queryKey: ['retrospectiva', ano], queryFn: () => api.retrospectiva(ano) })

  if (isLoading || !data) {
    return (
      <div className="flex justify-center py-20">
        <Spinner />
      </div>
    )
  }

  const anos = Array.from({ length: 4 }, (_, i) => agora - i)
  const seletor = (
    <nav aria-label="Ano" className="flex gap-2">
      {anos.map((a) => (
        <button
          key={a}
          type="button"
          aria-pressed={a === ano}
          onClick={() => setAno(a)}
          className={`min-h-9 rounded-full border px-3.5 text-sm font-semibold transition ${a === ano ? 'border-accent/55 bg-accent/12 text-ink' : 'border-line text-muted hover:text-ink'}`}
        >
          {a}
        </button>
      ))}
    </nav>
  )

  if (data.segundos < 60) {
    return (
      <div className="mx-auto max-w-3xl space-y-6 px-4 py-6 sm:px-6">
        {seletor}
        <EmptyState title={`Nada assistido em ${ano} ainda`} description="A retrospectiva junta o que você assiste: horas, gêneros, maratonas. Volte depois de algumas sessões." />
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-3xl space-y-6 px-4 py-6 sm:px-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">Retrospectiva</h1>
        {seletor}
      </div>

      <div className="space-y-6">
        <Cartao rotulo={`Seu ${ano} no Ozymandias`}>
          <NuvemDoAno segundos={data.segundos} />
          <p className="text-center text-5xl font-bold tracking-tight sm:text-6xl">
            <Contador valor={horas(data.segundos)} sufixo=" h" />
          </p>
          <p className="text-center text-muted">
            assistidas em <Contador valor={data.dias} /> dias diferentes · {data.filmes} filmes e {data.episodios} episódios
          </p>
        </Cartao>

        {data.maior_sequencia >= 2 && (
          <Cartao rotulo="Constância">
            <p className="text-5xl font-bold tracking-tight">
              <Contador valor={data.maior_sequencia} /> dias seguidos
            </p>
            <p className="text-muted">foi a sua maior sequência sem pular um dia.</p>
          </Cartao>
        )}

        {data.mais_vistos.length > 0 && (
          <Cartao rotulo="O que você mais viu">
            <div className="flex items-end gap-5">
              <Poster t={data.mais_vistos[0]} grande />
              <div>
                <p className="text-3xl font-bold tracking-tight">{data.mais_vistos[0].nome}</p>
                <p className="mt-1 text-muted">{horas(data.mais_vistos[0].segundos)} h ao longo do ano</p>
              </div>
            </div>
            {data.mais_vistos.length > 1 && (
              <ol className="grid grid-cols-2 gap-4 sm:grid-cols-4">
                {data.mais_vistos.slice(1).map((t, i) => (
                  <li key={t.id} className="space-y-2">
                    <Link to={t.id ? `/title/${t.id}` : '#'} className="block">
                      <Poster t={t} />
                    </Link>
                    <p className="text-sm font-medium">
                      <span className="font-mono text-muted">{i + 2}.</span> {t.nome}
                    </p>
                  </li>
                ))}
              </ol>
            )}
          </Cartao>
        )}

        {data.generos.length > 0 && (
          <Cartao rotulo="Seus gêneros">
            <ul className="space-y-3">
              {data.generos.map((g, i) => (
                <li key={g.nome}>
                  <div className="flex items-baseline justify-between">
                    <span className={i === 0 ? 'text-2xl font-bold' : 'font-medium'}>{g.nome}</span>
                    <span className="font-mono text-sm text-muted">{horas(g.segundos)} h</span>
                  </div>
                  <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-elev">
                    <div className="arm-barra h-full rounded-full bg-accent" style={{ width: `${(g.segundos / data.generos[0].segundos) * 100}%` }} />
                  </div>
                </li>
              ))}
            </ul>
          </Cartao>
        )}

        {data.maratona && (
          <Cartao rotulo="Sua maior maratona">
            <div className="flex items-end gap-5">
              <Poster t={data.maratona.titulo} grande />
              <div>
                <p className="text-5xl font-bold tracking-tight">
                  <Contador valor={data.maratona.episodios} /> episódios
                </p>
                <p className="mt-1 text-muted">
                  de {data.maratona.titulo.nome} em {dataCurta(data.maratona.dia)}.
                </p>
              </div>
            </div>
          </Cartao>
        )}

        <Cartao rotulo="O seu ritmo">
          <div>
            <div className="flex h-36 items-end gap-1.5" role="img" aria-label="Horas por mês">
              {data.por_mes.map((s, i) => {
                const max = Math.max(...data.por_mes, 1)
                return (
                  <div key={i} className="flex flex-1 flex-col items-center gap-1.5">
                    <div className="w-full rounded-t bg-accent/80" style={{ height: `${Math.max(2, (s / max) * 100)}%` }} title={`${MESES[i]} · ${horas(s)} h`} />
                    <span className="font-mono text-[10px] text-muted">{MESES[i]}</span>
                  </div>
                )
              })}
            </div>
          </div>
          {data.hora_preferida >= 0 && (
            <p className="text-lg">
              Seu horário preferido é por volta das <span className="font-bold text-accent">{data.hora_preferida}h</span>
              {data.hora_preferida >= 0 && data.hora_preferida < 5 ? ' — uma coruja do acervo.' : '.'}
            </p>
          )}
        </Cartao>

        {data.juntos?.length > 0 && (
          <Cartao rotulo="Assistindo junto">
            <p className="text-5xl font-bold tracking-tight">
              <Contador valor={horas(data.juntos.reduce((t, c) => t + c.segundos, 0))} sufixo=" h" />
            </p>
            <p className="text-muted">
              em salas com outras pessoas. Quem mais esteve com você foi{' '}
              <span className="font-semibold text-ink">{data.juntos[0].nome}</span>.
            </p>
            <ul className="space-y-3">
              {data.juntos.map((c) => (
                <li key={c.nome} className="flex items-center gap-3">
                  <span className="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-accent text-lg font-bold text-accent-ink uppercase">
                    {c.nome.slice(0, 1)}
                  </span>
                  <span className="flex-1 font-medium">{c.nome}</span>
                  <span className="font-mono text-sm text-muted">
                    {c.segundos >= 3600 ? `${horas(c.segundos)} h` : `${Math.round(c.segundos / 60)} min`}
                  </span>
                </li>
              ))}
            </ul>
          </Cartao>
        )}

        {data.primeiro && data.ultimo && (
          <Cartao rotulo="Do começo ao fim">
            <div className="grid gap-6 sm:grid-cols-2">
              {[
                ['Primeiro do ano', data.primeiro],
                ['O mais recente', data.ultimo],
              ].map(([rotulo, t]) => {
                const titulo = t as TituloNaRetrospectiva
                return (
                  <div key={rotulo as string} className="flex items-end gap-4">
                    <Poster t={titulo} />
                    <div>
                      <p className="text-xs text-muted">{rotulo as string}</p>
                      <p className="text-lg font-semibold">{titulo.nome}</p>
                      <p className="text-sm text-muted">{dataCurta(titulo.dia)}</p>
                    </div>
                  </div>
                )
              })}
            </div>
          </Cartao>
        )}
      </div>
    </div>
  )
}
