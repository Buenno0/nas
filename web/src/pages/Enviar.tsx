// Tela de envio: o céu inteiro recebe os arquivos (canvas "Céu aberto") e cada
// arquivo do lote vira um fio de luz que sobe do chão até a nuvem — chuva ao
// contrário: as gotas daquele arquivo sobem pelo fio na velocidade dele. O
// motor é o mesmo de sempre (lib/envios): um envio que começa aqui continua
// no cartão flutuante em qualquer outra tela.
import { useEffect, useMemo, useRef, useState, type DragEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../lib/api'
import { useEnvios, mbps, type Linha } from '../lib/envios'
import { chaveModo, useModoNuvem } from '../lib/nuvem'
import { humanSize } from '../lib/format'
import { ListaDeEnvios } from '../components/ListaDeEnvios'
import { EmptyState } from '../components/states'
import { UploadIcon } from '../components/icons'

const CUMULO = 'M70 170 C30 170 20 130 50 118 C44 84 84 66 110 82 C120 46 170 36 196 64 C214 30 280 30 292 78 C330 70 364 96 352 130 C380 140 376 172 344 172 Z'
const CUMULO_BORDA = 'M50 118 C44 84 84 66 110 82 C120 46 170 36 196 64 C214 30 280 30 292 78 C330 70 364 96 352 130'
const CHAVE_BIBLIOTECA = 'nas-enviar-biblioteca'

// Estrelas e gotas em posições estáveis: o céu não "pula" a cada render.
const ESTRELAS = Array.from({ length: 46 }, (_, i) => ({
  x: (i * 311) % 1440,
  y: 20 + ((i * 173) % 560),
  r: 0.6 + (i % 3) * 0.45,
  atraso: -((i * 0.83) % 7),
}))

function lerBiblioteca(): number {
  try {
    return Number(localStorage.getItem(CHAVE_BIBLIOTECA)) || 0
  } catch {
    return 0
  }
}
function gravarBiblioteca(id: number) {
  try {
    localStorage.setItem(CHAVE_BIBLIOTECA, String(id))
  } catch {
    // sem armazenamento: o destino vale só nesta visita
  }
}

/** "04:12" com cada dígito numa coluna que rola, como um contador mecânico. */
function Relogio({ segundos }: { segundos: number }) {
  const s = Math.max(0, Math.round(segundos))
  const texto = s >= 3600 ? `${Math.floor(s / 3600)}:${String(Math.floor((s % 3600) / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}` : `${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`
  return (
    <span className="inline-flex h-6 overflow-hidden font-mono leading-6" aria-label={`faltam ${texto}`}>
      {texto.split('').map((c, i) =>
        c === ':' ? (
          <span key={i} className="w-2.5 text-center">
            :
          </span>
        ) : (
          <span key={i} className="inline-block h-6 w-3 overflow-hidden">
            <span className="ce-coluna flex flex-col" style={{ transform: `translateY(-${Number(c) * 24}px)` }}>
              {'0123456789'.split('').map((n) => (
                <span key={n} className="h-6 text-center">
                  {n}
                </span>
              ))}
            </span>
          </span>
        ),
      )}
    </span>
  )
}

type Estado = 'vazio' | 'arrastando' | 'enviando' | 'pausado' | 'erro' | 'concluido'

export function Enviar() {
  const queryClient = useQueryClient()
  const { data: user } = useQuery({ queryKey: ['me'], queryFn: api.me })
  const { data: libraries } = useQuery({ queryKey: ['libraries'], queryFn: api.libraries })
  const modo = useModoNuvem()
  const hibrido = modo?.modo === 'hibrido'
  const { linhas, velocidades, lote, adiciona } = useEnvios()
  const [escolhida, setEscolhida] = useState(lerBiblioteca)
  const [arrastando, setArrastando] = useState(false)
  const profundidade = useRef(0)
  const entrada = useRef<HTMLInputElement>(null)

  const destino = libraries?.find((l) => l.id === escolhida)?.id ?? libraries?.[0]?.id ?? 0
  const nomeDestino = libraries?.find((l) => l.id === destino)?.name ?? ''

  const religar = useMutation({
    mutationFn: () => api.setModo('hibrido'),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: chaveModo }),
  })

  // O lote na tela é o mais recente; o do cartão flutuante é o mesmo.
  const doLote: Linha[] = useMemo(() => (lote ? linhas.filter((l) => l.lote === lote.numero) : []), [linhas, lote])
  const f = lote?.progresso ?? 0
  const temErro = doLote.some((l) => l.estado === 'erro')
  const temPausa = doLote.some((l) => l.estado === 'pausado')
  const concluido = !!lote && !lote.ativo && f >= 1 && !temPausa

  let estado: Estado = 'vazio'
  if (arrastando) estado = 'arrastando'
  else if (lote && lote.ativo) estado = temErro ? 'erro' : 'enviando'
  else if (lote && temPausa) estado = 'pausado'
  else if (lote && temErro) estado = 'erro'
  else if (concluido) estado = 'concluido'

  // O raio cai uma vez, na virada de "subindo" para "tudo na nuvem" — não ao
  // abrir a tela com um lote que já tinha terminado.
  const estavaAtivo = useRef(lote?.ativo ?? false)
  const [raio, setRaio] = useState(0)
  useEffect(() => {
    if (estavaAtivo.current && concluido) setRaio((n) => n + 1)
    estavaAtivo.current = lote?.ativo ?? false
  }, [lote?.ativo, concluido])

  if (user && !user.is_admin) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-6 sm:px-6">
        <EmptyState title="Envio para a nuvem" description="Só quem mantém o NAS envia arquivos ao bucket." />
      </div>
    )
  }

  const soltar = (arquivos: FileList | File[]) => {
    if (!hibrido || !destino || arquivos.length === 0) return
    adiciona(arquivos, destino)
  }
  const aoEntrar = (e: DragEvent) => {
    if (!e.dataTransfer.types.includes('Files')) return
    e.preventDefault()
    profundidade.current += 1
    setArrastando(true)
  }
  const aoSair = () => {
    profundidade.current = Math.max(0, profundidade.current - 1)
    if (profundidade.current === 0) setArrastando(false)
  }
  const aoSoltar = (e: DragEvent) => {
    e.preventDefault()
    profundidade.current = 0
    setArrastando(false)
    soltar(e.dataTransfer.files)
  }

  // Nuvem-mãe: cresce de 0,6 a 1,25; acorda ao arrastar; acinzenta no corte.
  const enviando = estado === 'enviando' || estado === 'erro'
  const pausado = estado === 'pausado' || (!hibrido && !!lote)
  const escalaMae = estado === 'arrastando' ? 0.78 : lote ? 0.6 + 0.65 * f : 0.66
  const bordaMae = estado === 'arrastando' ? 0.7 : concluido ? 0.9 : pausado ? 0.22 : 0.18 + 0.42 * f
  const brilho = estado === 'arrastando' ? 0.16 : enviando ? 0.04 + 0.16 * f : concluido ? 0.2 : 0.03

  // Fios: um por arquivo do lote, espalhados sob a nuvem. O fio acende com o
  // progresso; as gotas sobem na velocidade daquele arquivo; ao terminar, o
  // fio se recolhe para dentro da nuvem.
  const vao = Math.min(84, 520 / Math.max(1, doLote.length))
  const fios = doLote.map((l, i) => {
    const p = l.estado === 'pronto' ? 1 : l.total > 0 ? l.feitos / l.total : 0
    const bps = velocidades.get(l.chave) ?? 0
    const pronto = l.estado === 'pronto'
    return {
      l,
      p,
      x: (i - (doLote.length - 1) / 2) * vao,
      pronto,
      // de 3,4 s (parado) a 1 s (rápido): o fio de quem sobe mais depressa corre mais
      dur: bps > 0 ? Math.max(1, 3.4 - Math.min(2.4, bps / 6e6)) : 3.4,
      cor: l.estado === 'erro' ? 'var(--danger)' : pronto ? 'var(--ok)' : pausado ? 'var(--muted)' : 'var(--accent)',
    }
  })

  const acelera = 1 - 0.7 * f
  const quantasGotas = fios.length > 0 ? 0 : enviando ? 8 + Math.round(16 * f) : pausado ? 10 : 0
  const gotas = Array.from({ length: quantasGotas }, (_, i) => ({
    x: -200 + ((i * 131) % 400),
    r: 1.8 + (i % 3) * 0.7,
    dur: (2.4 + (i % 5) * 0.28) * acelera,
    ouro: i % 4 === 0,
  }))

  const restante = lote && lote.bps > 0 ? (lote.total - lote.feito) / lote.bps : 0
  const titulo = {
    vazio: 'Solte e deixe o céu levar',
    arrastando: 'Pode soltar',
    enviando: doLote.length === 1 ? 'Subindo para a nuvem' : `${doLote.length} arquivos subindo`,
    pausado: 'Envio pausado',
    erro: 'Uma parte não subiu',
    concluido: 'Tudo na nuvem',
  }[estado]
  const status = {
    vazio: hibrido ? 'A nuvem está quieta' : 'A nuvem está desligada',
    arrastando: `Solte para enviar a ${nomeDestino}`,
    enviando: lote ? `${Math.round(f * 100)}% de ${humanSize(lote.total)}` : '',
    pausado: `pausado em ${Math.round(f * 100)}%`,
    erro: `${Math.round(f * 100)}% · ${doLote.filter((l) => l.estado === 'erro').length} arquivo precisa de atenção`,
    concluido: `${doLote.length} ${doLote.length === 1 ? 'arquivo' : 'arquivos'} na nuvem`,
  }[estado]
  const detalhe = {
    vazio: hibrido ? 'arraste arquivos para qualquer lugar desta tela' : 'ligue o híbrido em Configurações para enviar',
    arrastando: 'direto ao bucket, sem passar pelo Mac',
    enviando: lote && lote.bps > 0 ? `${mbps(lote.bps)} · partes de 16 MiB · 4 ao mesmo tempo` : 'medindo a velocidade…',
    pausado: 'modo local · nada sai do Mac até o híbrido voltar',
    erro: 'os outros continuam; esse espera você na lista abaixo',
    concluido: 'o worker prepara a versão compatível em alguns minutos',
  }[estado]

  return (
    <div
      onDragEnter={aoEntrar}
      onDragOver={(e) => e.dataTransfer.types.includes('Files') && e.preventDefault()}
      onDragLeave={aoSair}
      onDrop={aoSoltar}
      className="relative min-h-[calc(100dvh-4rem)] overflow-hidden"
    >
      {/* Céu: escurece ao arrastar. */}
      <div
        className="pointer-events-none absolute inset-0 bg-black transition-opacity duration-500"
        style={{ opacity: estado === 'arrastando' ? 0.45 : 0 }}
      />
      <svg viewBox="0 0 1440 600" preserveAspectRatio="xMidYMid slice" className="pointer-events-none absolute inset-x-0 top-0 h-[600px] w-full" aria-hidden="true">
        {ESTRELAS.map((s, i) => (
          <circle key={i} className="ce-estrela" cx={s.x} cy={s.y} r={s.r} fill="var(--ink)" style={{ animationDelay: `${s.atraso.toFixed(2)}s` }} />
        ))}
      </svg>
      <div className="pointer-events-none absolute inset-x-0 bottom-0 h-56 overflow-hidden" aria-hidden="true">
        <svg className="ce-nevoa-a absolute bottom-16 left-0 h-40 w-[200%] opacity-60" viewBox="0 0 2880 200" preserveAspectRatio="none">
          <path d="M0 120 C200 90 360 110 540 100 C760 86 900 118 1100 104 C1260 94 1340 112 1440 108 C1640 90 1800 110 1980 100 C2200 86 2340 118 2540 104 C2700 94 2780 112 2880 108 L2880 200 L0 200 Z" fill="var(--surface)" />
        </svg>
        <svg className="ce-nevoa-b absolute bottom-0 left-0 h-40 w-[200%]" viewBox="0 0 2880 200" preserveAspectRatio="none">
          <path d="M0 110 C240 130 420 96 640 112 C860 128 1060 98 1240 110 C1340 116 1400 106 1440 110 C1680 130 1860 96 2080 112 C2300 128 2500 98 2680 110 C2780 116 2840 106 2880 110 L2880 200 L0 200 Z" fill="var(--elev)" />
        </svg>
      </div>

      <div className="relative mx-auto flex max-w-5xl flex-col gap-4 px-4 pt-8 pb-40 sm:px-6">
        <header className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <p className="font-mono text-xs tracking-[0.28em] text-accent uppercase">Enviar à nuvem</p>
            <h1 className="mt-2 text-2xl font-bold tracking-tight sm:text-3xl">{titulo}</h1>
          </div>
          <nav aria-label="Biblioteca de destino" className="flex flex-wrap items-center gap-2">
            <span className="mr-1 text-xs text-muted">para</span>
            {(libraries ?? []).map((lib) => {
              const ativa = lib.id === destino
              return (
                <button
                  key={lib.id}
                  type="button"
                  aria-pressed={ativa}
                  onClick={() => {
                    setEscolhida(lib.id)
                    gravarBiblioteca(lib.id)
                  }}
                  className={[
                    'min-h-9 rounded-full border px-3.5 text-sm font-semibold transition',
                    ativa ? 'border-accent/55 bg-accent/12 text-ink' : 'border-line text-muted hover:border-muted hover:text-ink',
                  ].join(' ')}
                >
                  {lib.name}
                </button>
              )
            })}
          </nav>
        </header>

        {/* Palco: órbitas, satélites e a nuvem-mãe num só SVG, que escala com a tela. */}
        <div className={estado === 'arrastando' ? 'ce-acorda' : 'ce-respira'}>
          <svg viewBox="-380 -250 760 470" className="mx-auto h-auto w-full max-w-3xl overflow-visible" aria-hidden="true">
            <defs>
              <radialGradient id="ce-brilho">
                <stop offset="0%" stopColor="var(--accent)" stopOpacity={brilho} />
                <stop offset="100%" stopColor="var(--accent)" stopOpacity={0} />
              </radialGradient>
            </defs>
            <ellipse cx="0" cy="0" rx="330" ry="210" fill="url(#ce-brilho)" />

            {/* Nuvem-mãe */}
            <g style={{ transform: `scale(${escalaMae.toFixed(3)})`, transition: 'transform .7s cubic-bezier(.2,.8,.2,1)' }}>
              <g transform="translate(-200 -115)">
                <path d={CUMULO} fill={pausado ? 'var(--surface)' : 'var(--elev)'} style={{ transition: 'fill .6s ease' }} />
                <path
                  d={CUMULO_BORDA}
                  fill="none"
                  stroke={concluido ? 'var(--ink)' : pausado ? 'var(--muted)' : 'var(--ink)'}
                  strokeOpacity={bordaMae}
                  strokeWidth={2.4}
                  strokeLinecap="round"
                  style={{ transition: 'stroke-opacity .6s ease' }}
                />
                {enviando && f > 0.84 && (
                  <>
                    <path className="ce-centelha" d="M180 96 L172 118 L184 118 L176 138" fill="none" stroke="var(--ink)" strokeWidth={1.6} strokeLinecap="round" strokeLinejoin="round" />
                    <path className="ce-centelha" d="M262 90 L256 108 L266 108 L260 124" fill="none" stroke="var(--ink)" strokeWidth={1.3} strokeLinecap="round" strokeLinejoin="round" style={{ animationDelay: '.7s' }} />
                  </>
                )}
                {concluido && (
                  <path key={`marca-${raio}`} className="nv-marca-fim" d="M186 108 L196 118 L216 96" fill="none" stroke="var(--ok)" strokeWidth={3} strokeLinecap="round" strokeLinejoin="round" style={{ strokeDasharray: 40, strokeDashoffset: 40 }} />
                )}
                {temErro &&
                  Array.from({ length: 7 }, (_, i) => (
                    <line key={i} className="ce-garoa" x1={120 + i * 26} y1={178} x2={116 + i * 26} y2={190} stroke="var(--danger)" strokeWidth={1.6} strokeLinecap="round" style={{ animationDelay: `-${(i * 0.13).toFixed(2)}s` }} />
                  ))}
              </g>
            </g>

            {/* Fios de luz: do chão (y=210) até a base da nuvem (y≈40). */}
            {fios.map((fio) => (
              <g key={fio.l.chave} style={{ opacity: fio.pronto ? 0 : 1, transition: 'opacity .9s ease .5s' }}>
                <line
                  x1={fio.x}
                  y1={210}
                  x2={fio.x * 0.35}
                  y2={fio.pronto ? 40 : 46}
                  stroke={fio.cor}
                  strokeOpacity={pausado ? 0.12 : 0.12 + 0.5 * fio.p}
                  strokeWidth={1.4}
                  strokeLinecap="round"
                  style={{ transition: 'stroke-opacity .6s ease' }}
                />
                {/* o trecho já enviado brilha, crescendo de baixo para cima */}
                <line
                  x1={fio.x}
                  y1={210}
                  x2={fio.x + (fio.x * 0.35 - fio.x) * fio.p}
                  y2={210 - (210 - 46) * fio.p}
                  stroke={fio.cor}
                  strokeOpacity={pausado ? 0.25 : 0.85}
                  strokeWidth={2}
                  strokeLinecap="round"
                  style={{ transition: 'all .5s cubic-bezier(.2,.8,.2,1)', filter: pausado ? 'none' : 'drop-shadow(0 0 4px var(--accent))' }}
                />
                {!fio.pronto &&
                  [0, 1, 2].map((k) => (
                    <circle
                      key={k}
                      className={`ce-fio-gota ${pausado ? 'ce-parado' : ''}`}
                      r={k === 0 ? 2.4 : 1.7}
                      fill={pausado ? 'var(--line)' : fio.cor}
                      style={
                        {
                          '--x0': `${fio.x}px`,
                          '--x1': `${(fio.x * 0.35).toFixed(1)}px`,
                          animationDuration: `${fio.dur.toFixed(2)}s`,
                          animationDelay: `-${((k * fio.dur) / 3).toFixed(2)}s`,
                        } as React.CSSProperties
                      }
                    />
                  ))}
                <text x={fio.x} y={230} textAnchor="middle" fill="var(--muted)" className="font-mono" fontSize="11">
                  {fio.pronto ? '✓' : `${Math.round(fio.p * 100)}%`}
                </text>
              </g>
            ))}

            {/* Gotas que sobem até a nuvem */}
            {gotas.map((g, i) => (
              <circle
                key={i}
                className={`ce-gota ${pausado ? 'ce-parado' : ''}`}
                cx={g.x}
                cy={200}
                r={g.r}
                fill={pausado ? 'var(--line)' : g.ouro ? 'var(--accent)' : 'var(--muted)'}
                style={{ animationDuration: `${g.dur.toFixed(2)}s`, animationDelay: `-${((i * 0.41) % g.dur).toFixed(2)}s` }}
              />
            ))}

            {/* O raio do fim do lote, com a chuva dourada. */}
            {raio > 0 && concluido && (
              <g key={`raio-${raio}`}>
                <rect x="-2000" y="-2000" width="4000" height="4000" fill="var(--ink)" className="nv-clarao" />
                <g className="nv-raio" style={{ filter: 'drop-shadow(0 0 10px var(--accent))' }}>
                  <path className="nv-raio-traco" style={{ strokeDasharray: 320, strokeDashoffset: 320 }} d="M8 50 L-10 110 L14 114 L-14 168 L10 172 L-18 230" fill="none" stroke="var(--ink)" strokeWidth={3.2} strokeLinecap="round" strokeLinejoin="round" />
                  <path className="nv-raio-traco" style={{ strokeDasharray: 50, strokeDashoffset: 50, animationDelay: '.09s' }} d="M-10 110 L-36 138" fill="none" stroke="var(--ink)" strokeWidth={1.6} strokeLinecap="round" />
                </g>
                <circle className="nv-impacto" cx="-18" cy="230" r="14" fill="var(--accent)" />
                {Array.from({ length: 16 }, (_, i) => (
                  <line key={i} className="ce-chuva-ouro" x1={-150 + i * 20} y1={70} x2={-150 + i * 20} y2={84} stroke="var(--accent)" strokeWidth={1.4} strokeLinecap="round" style={{ animationDelay: `${(0.9 + (i % 5) * 0.12).toFixed(2)}s` }} />
                ))}
              </g>
            )}
          </svg>
        </div>

        <div className="-mt-4 flex flex-col items-center gap-1 text-center" aria-live="polite">
          <div className="flex flex-wrap items-baseline justify-center gap-x-4 gap-y-1">
            <span className={`text-lg font-semibold ${estado === 'erro' ? 'text-danger' : concluido ? 'text-ok' : pausado ? 'text-muted' : ''}`}>{status}</span>
            {estado === 'enviando' && restante > 0 && (
              <span className="inline-flex items-baseline gap-2 text-lg">
                <span className="font-mono text-[11px] tracking-[0.18em] text-muted uppercase">faltam</span>
                <Relogio segundos={restante} />
              </span>
            )}
          </div>
          <p className="font-mono text-xs text-muted">{detalhe}</p>
        </div>

        {pausado && lote && (
          <div className="mx-auto flex flex-wrap items-center justify-center gap-3 rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-muted">
            A nuvem foi cortada. Nada saiu do Mac desde então; o envio continua da parte em que parou.
            <button type="button" onClick={() => religar.mutate()} disabled={religar.isPending} className="min-h-9 rounded-lg bg-accent px-3.5 text-sm font-bold text-accent-ink transition hover:opacity-90 disabled:opacity-60">
              Ligar o híbrido
            </button>
          </div>
        )}

        {/* Zona de soltar (ou "adicionar ao lote" enquanto sobe). */}
        <label
          className={[
            'ce-zona mx-auto flex w-full max-w-2xl cursor-pointer flex-col items-center gap-2 rounded-2xl border-[1.5px] border-dashed text-center',
            lote && estado !== 'arrastando' ? 'px-5 py-4' : 'px-6 py-9',
            estado === 'arrastando' ? '-translate-y-1.5 scale-[1.02] border-accent bg-accent/8' : 'border-line bg-surface/70 hover:border-accent/60',
            !hibrido ? 'cursor-not-allowed opacity-60' : '',
          ].join(' ')}
        >
          <input
            ref={entrada}
            type="file"
            multiple
            disabled={!hibrido || !destino}
            className="sr-only"
            onChange={(e) => {
              if (e.target.files) soltar(e.target.files)
              e.target.value = ''
            }}
          />
          <UploadIcon className="text-accent" width="1.6em" height="1.6em" />
          <span className="font-semibold">
            {!hibrido
              ? 'Ligue o híbrido para enviar'
              : estado === 'arrastando'
                ? `Solte para enviar a ${nomeDestino}`
                : lote && lote.ativo
                  ? 'Adicionar ao lote'
                  : 'Solte arquivos aqui, ou clique para escolher'}
          </span>
          {!(lote && estado !== 'arrastando') && (
            <span className="text-xs text-muted">filmes, episódios, músicas e fotos · direto ao bucket, sem passar pelo Mac</span>
          )}
        </label>

        {doLote.length > 0 && (
          <div className="mx-auto w-full max-w-2xl">
            <ListaDeEnvios linhas={doLote} velocidades={velocidades} />
          </div>
        )}
      </div>
    </div>
  )
}
