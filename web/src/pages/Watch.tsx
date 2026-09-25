import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import {
  api,
  beaconProgress,
  downloadUrl,
  type PlaybackInfo,
  type PlaybackPlan,
  type PreparoProgresso,
} from '../lib/api'
import { clockTime } from '../lib/format'
import {
  ChevronLeft,
  DownloadIcon,
  FullscreenIcon,
  PauseIcon,
  PipIcon,
  NuvemIcon,
  PlayIcon,
  VolumeIcon,
  WarningIcon,
} from '../components/icons'
import { ErrorState, Spinner } from '../components/states'

const SAVE_EVERY_MS = 10_000
const SKIP_SECONDS = 10
const HIDE_AFTER_MS = 3000
const SPEEDS = [0.75, 1, 1.25, 1.5, 2]

type SafariVideoElement = HTMLVideoElement & {
  webkitDisplayingFullscreen?: boolean
  webkitEnterFullscreen?: () => void
  webkitExitFullscreen?: () => void
}

type LockableScreenOrientation = ScreenOrientation & {
  lock?: (orientation: 'landscape') => Promise<void>
  unlock?: () => void
}

function screenOrientation() {
  return window.screen.orientation as LockableScreenOrientation | undefined
}

export function Watch() {
  const { fileId } = useParams()
  const id = Number(fileId)
  const navigate = useNavigate()

  const videoRef = useRef<HTMLVideoElement>(null)
  const shellRef = useRef<HTMLDivElement>(null)
  const hideTimer = useRef<number | undefined>(undefined)
  const lastSaved = useRef(0)
  const resumed = useRef(false)

  const [playing, setPlaying] = useState(false)
  const [current, setCurrent] = useState(0)
  const [duration, setDuration] = useState(0)
  const [buffered, setBuffered] = useState(0)
  const [volume, setVolume] = useState(1)
  const [muted, setMuted] = useState(false)
  const [speed, setSpeed] = useState(1)
  const [chromeVisible, setChromeVisible] = useState(true)
  const [failed, setFailed] = useState(false)

  // undefined = a faixa padrão do arquivo. Escolher outra muda a chave do
  // preparo no servidor, então tudo que consulta o plano depende disto.
  const [audioIdx, setAudioIdx] = useState<number | undefined>(undefined)
  const [legendaIdx, setLegendaIdx] = useState<number | undefined>(undefined)

  const { data: file, isLoading, isError, error } = useQuery({
    queryKey: ['file', id],
    queryFn: () => api.file(id),
  })
  const { data: next } = useQuery({
    queryKey: ['next', id],
    queryFn: () => api.nextEpisode(id),
    enabled: Number.isFinite(id),
  })

  // O que tocar: direto, ou o arquivo preparado pelo servidor. A decisão é do
  // backend, que conhece pix_fmt e perfil — dados que o navegador não vê.
  const { data: faixas } = useQuery({
    queryKey: ['faixas', id],
    queryFn: () => api.faixas(id),
    enabled: Number.isFinite(id),
  })

  const { data: plano, refetch: reconsultarPlano } = useQuery({
    queryKey: ['playback', id, audioIdx],
    queryFn: () => api.playback(id, audioIdx),
    enabled: Number.isFinite(id),
  })
  const [preparo, setPreparo] = useState<PreparoProgresso | undefined>(undefined)

  const save = useCallback(
    (position: number, total: number) => {
      if (!Number.isFinite(position) || total <= 0) return
      lastSaved.current = Date.now()
      void api.saveProgress(id, position, total).catch(() => {
        // Perder um ponto de progresso não pode interromper a reprodução.
      })
    },
    [id],
  )

  // Outro episódio é outro arquivo, com outras faixas: manter o índice
  // escolhido apontaria para uma faixa que talvez nem exista lá.
  useEffect(() => {
    setAudioIdx(undefined)
    setLegendaIdx(undefined)
  }, [id])

  // Salva ao sair da página: fetch comum é cancelado no unload, beacon não.
  useEffect(() => {
    const flush = () => {
      const video = videoRef.current
      if (video && video.duration > 0 && video.currentTime > 0) {
        beaconProgress(id, video.currentTime, video.duration)
      }
    }
    const aoEsconder = () => {
      if (document.visibilityState === 'hidden') flush()
    }
    window.addEventListener('pagehide', flush)
    document.addEventListener('visibilitychange', aoEsconder)
    return () => {
      flush()
      window.removeEventListener('pagehide', flush)
      document.removeEventListener('visibilitychange', aoEsconder)
    }
  }, [id])

  // Trocar de episódio reaproveita o componente: sem este reset, a posição
  // retomada, a duração e o estado de falha vazavam do arquivo anterior.
  useEffect(() => {
    resumed.current = false
    lastSaved.current = 0
    setFailed(false)
    setCurrent(0)
    setDuration(0)
    setBuffered(0)
    setPreparo(undefined)
  }, [id])

  // Preparo sob demanda: se o plano não é direto, pede o trabalho ao servidor e
  // acompanha por SSE. O trabalho é idempotente — recarregar a página não
  // reinicia o ffmpeg, entra na mesma fila.
  useEffect(() => {
    if (!plano || plano.modo === 'direct') return
    if (!plano.ffmpeg || !plano.transcodificacao_ativa) return
    if (plano.preparo?.estado === 'pronto') return

    let cancelado = false
    let fonte: EventSource | undefined

    void api
      .prepare(id, audioIdx)
      .then((inicial) => {
        if (cancelado) return
        setPreparo(inicial)
        fonte = new EventSource(api.prepareEventsUrl(id, audioIdx))
        fonte.onmessage = (evento) => {
          try {
            const atual = JSON.parse(evento.data) as PreparoProgresso
            setPreparo(atual)
            if (atual.estado === 'pronto') {
              fonte?.close()
              // O plano agora tem a URL do arquivo preparado.
              void reconsultarPlano()
            }
            if (atual.estado === 'erro') fonte?.close()
          } catch {
            // mensagem malformada: espera a próxima
          }
        }
        fonte.onerror = () => fonte?.close()
      })
      .catch((err: Error) => {
        if (!cancelado) setPreparo({ estado: 'erro', erro: err.message } as PreparoProgresso)
      })

    return () => {
      cancelado = true
      fonte?.close()
    }
  }, [id, plano, audioIdx, reconsultarPlano])

  const legendaSelecionada = faixas?.legendas.find((l) => l.idx === legendaIdx)

  // O atributo `default` só vale no primeiro carregamento; trocar de legenda
  // depois exige mexer no modo da trilha à mão, senão ela é montada e fica
  // invisível.
  useEffect(() => {
    const video = videoRef.current
    if (!video) return
    for (const trilha of Array.from(video.textTracks)) {
      trilha.mode = legendaSelecionada ? 'showing' : 'disabled'
    }
  }, [legendaSelecionada, plano])

  // A fonte do <video> é atribuída aqui, e não no JSX: Watch re-renderiza a cada
  // segundo (o onTimeUpdate atualiza estado), e um src no atributo seria
  // reescrito no meio da reprodução.
  useEffect(() => {
    const video = videoRef.current
    if (!video || !plano) return
    if (plano.indisponivel) return
    const alvo = plano.url || (plano.modo === 'direct' ? plano.url_direta : '')
    if (!alvo) return
    if (video.getAttribute('src') === alvo) return

    video.setAttribute('src', alvo)
    video.load()
    resumed.current = false
  }, [plano])

  const revealChrome = useCallback(() => {
    setChromeVisible(true)
    window.clearTimeout(hideTimer.current)
    hideTimer.current = window.setTimeout(() => {
      if (videoRef.current && !videoRef.current.paused) setChromeVisible(false)
    }, HIDE_AFTER_MS)
  }, [])

  const togglePlay = useCallback(() => {
    const video = videoRef.current
    if (!video) return
    if (video.paused) void video.play()
    else video.pause()
    revealChrome()
  }, [revealChrome])

  const seekBy = useCallback(
    (delta: number) => {
      const video = videoRef.current
      if (!video) return
      video.currentTime = Math.min(Math.max(0, video.currentTime + delta), video.duration || 0)
      revealChrome()
    },
    [revealChrome],
  )

  const toggleFullscreen = useCallback(() => {
    const shell = shellRef.current
    const video = videoRef.current as SafariVideoElement | null
    if (!shell || !video) return

    if (document.fullscreenElement) {
      void document.exitFullscreen().finally(() => screenOrientation()?.unlock?.())
      return
    }

    if (video.webkitDisplayingFullscreen) {
      video.webkitExitFullscreen?.()
      return
    }

    const enterNativeVideoFullscreen = () => {
      try {
        video.webkitEnterFullscreen?.()
      } catch {
        // O Safari só permite fullscreen durante uma ação direta do usuário.
      }
    }

    // iPhones que não implementam fullscreen em elementos comuns precisam da
    // API nativa do próprio <video>. Ela também acompanha a rotação do aparelho.
    if (!document.fullscreenEnabled || typeof shell.requestFullscreen !== 'function') {
      enterNativeVideoFullscreen()
      return
    }

    void shell
      .requestFullscreen()
      .then(() => screenOrientation()?.lock?.('landscape').catch(() => {}))
      .catch(enterNativeVideoFullscreen)
  }, [])

  useEffect(() => {
    const releaseOrientation = () => {
      if (!document.fullscreenElement) screenOrientation()?.unlock?.()
    }
    document.addEventListener('fullscreenchange', releaseOrientation)
    return () => document.removeEventListener('fullscreenchange', releaseOrientation)
  }, [])

  // Atalhos de teclado no estilo dos players de streaming.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null
      if (target && ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) return

      switch (e.key) {
        case ' ':
        case 'k':
          e.preventDefault()
          togglePlay()
          break
        case 'ArrowRight':
          seekBy(SKIP_SECONDS)
          break
        case 'ArrowLeft':
          seekBy(-SKIP_SECONDS)
          break
        case 'ArrowUp':
          e.preventDefault()
          setVolume((v) => Math.min(1, v + 0.1))
          break
        case 'ArrowDown':
          e.preventDefault()
          setVolume((v) => Math.max(0, v - 0.1))
          break
        case 'f':
          toggleFullscreen()
          break
        case 'm':
          setMuted((m) => !m)
          break
        case 'Escape':
          if (!document.fullscreenElement) navigate(-1)
          break
      }
      revealChrome()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [navigate, revealChrome, seekBy, toggleFullscreen, togglePlay])

  useEffect(() => {
    const video = videoRef.current
    if (!video) return
    video.volume = volume
    video.muted = muted
    video.playbackRate = speed
  }, [volume, muted, speed])

  if (isLoading) return <Spinner label="Preparando…" />
  if (isError || !file) return <ErrorState error={error} />

  const isAudio = file.media_type === 'audio'
  const percent = duration > 0 ? (current / duration) * 100 : 0
  const bufferedPercent = duration > 0 ? (buffered / duration) * 100 : 0

  const onSeek = (e: React.ChangeEvent<HTMLInputElement>) => {
    const video = videoRef.current
    if (!video || !duration) return
    video.currentTime = (Number(e.target.value) / 100) * duration
    setCurrent(video.currentTime)
  }

  return (
    <div
      ref={shellRef}
      onMouseMove={revealChrome}
      onTouchStart={revealChrome}
      className={[
        'sala-escura relative flex h-dvh w-full flex-col bg-black',
        chromeVisible ? '' : 'cursor-none',
      ].join(' ')}
    >
      <video
        ref={videoRef}
        poster={file.poster}
        autoPlay
        playsInline
        onClick={togglePlay}
        onLoadedMetadata={(e) => {
          const video = e.currentTarget
          setDuration(video.duration || file.duration || 0)
          // Retoma de onde parou, mas não a 5 segundos do fim.
          if (!resumed.current && file.position && file.position > 5) {
            if (!video.duration || file.position < video.duration - 5) {
              video.currentTime = file.position
            }
          }
          resumed.current = true
        }}
        onTimeUpdate={(e) => {
          const video = e.currentTarget
          setCurrent(video.currentTime)
          if (video.buffered.length > 0) {
            setBuffered(video.buffered.end(video.buffered.length - 1))
          }
          if (Date.now() - lastSaved.current > SAVE_EVERY_MS) {
            save(video.currentTime, video.duration)
          }
        }}
        onPlay={() => {
          setPlaying(true)
          revealChrome()
        }}
        onPause={(e) => {
          setPlaying(false)
          setChromeVisible(true)
          save(e.currentTarget.currentTime, e.currentTarget.duration)
        }}
        onEnded={(e) => {
          save(e.currentTarget.duration, e.currentTarget.duration)
          if (next?.next) navigate(`/watch/${next.next}`)
        }}
        onError={() => setFailed(true)}
        className={['h-full w-full', isAudio ? 'object-contain opacity-90' : 'object-contain'].join(' ')}
      >
        {/* Só a legenda escolhida é montada: cada <track> baixado dispara uma
            conversão para WebVTT no servidor, e montar todas gastaria um ffmpeg
            por legenda que ninguém pediu. A key força o remount na troca. */}
        {legendaSelecionada?.url && (
          <track
            key={legendaSelecionada.idx}
            kind="subtitles"
            src={legendaSelecionada.url}
            srcLang={legendaSelecionada.lang || 'und'}
            label={legendaSelecionada.rotulo}
            default
          />
        )}
      </video>

      {/* Item que mora só no bucket, com o modo local: existe, não toca. */}
      {plano?.indisponivel && (
        <div className="absolute inset-0 grid place-items-center bg-black/85 p-6 text-center text-white">
          <div className="max-w-sm">
            <p className="text-lg font-semibold">
              {plano.motivo.startsWith('no Mac') ? 'No Mac, indisponível' : 'Na nuvem, indisponível'}
            </p>
            <p className="mt-2 text-sm text-white/70">
              {plano.motivo.startsWith('no Mac')
                ? 'Este arquivo só existe no disco do Mac. Ele toca daqui quando tiver uma cópia na nuvem, ou abra o Ozymandias do Mac.'
                : 'Este arquivo mora só no bucket e o servidor está no modo local. Ligue o híbrido em Configurações para tocar.'}
            </p>
          </div>
        </div>
      )}

      {/* Preparo em curso: o vídeo ainda não tem fonte, então esta é a tela.
          Antes daqui existir, o caso HEVC dava tela preta com um comando de
          ffmpeg para o dono rodar à mão. */}
      {plano && plano.modo !== 'direct' && plano.url === '' && plano.ffmpeg && plano.transcodificacao_ativa && (
        <PainelDePreparo plano={plano} preparo={preparo} />
      )}

      {/* Sem ffmpeg (ou com transcodificação desligada) não há o que preparar:
          volta o aviso com o comando manual. */}
      {((plano && plano.modo !== 'direct' && (!plano.ffmpeg || !plano.transcodificacao_ativa)) || failed) && (
        <UnsupportedOverlay file={file} plano={plano} />
      )}

      {/* Barra superior */}
      <div
        className={[
          'pointer-events-none absolute inset-x-0 top-0 bg-gradient-to-b from-black/80 to-transparent p-4 transition-opacity duration-300',
          chromeVisible ? 'opacity-100' : 'opacity-0',
        ].join(' ')}
      >
        <div className="pointer-events-auto flex items-center gap-3 text-white">
          <button
            type="button"
            onClick={() => navigate(-1)}
            aria-label="Voltar"
            className="grid h-10 w-10 place-items-center rounded-full bg-black/40 backdrop-blur-sm transition hover:bg-black/60"
          >
            <ChevronLeft />
          </button>
          <div className="min-w-0">
            <p className="line-clamp-1 text-sm font-semibold">
              {file.title_name || file.name}
            </p>
            {(file.season || file.episode) && (
              <p className="text-xs text-white/70">
                T{file.season} · E{file.episode} {file.episode_name ? `· ${file.episode_name}` : ''}
              </p>
            )}
          </div>
        </div>
      </div>

      {/* Controles inferiores */}
      <div
        className={[
          'absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/90 via-black/60 to-transparent px-4 pt-10 pb-5 transition-opacity duration-300',
          chromeVisible ? 'opacity-100' : 'pointer-events-none opacity-0',
        ].join(' ')}
      >
        <div className="relative mb-3">
          <div className="absolute inset-x-0 top-1/2 h-1 -translate-y-1/2 overflow-hidden rounded-full bg-white/25">
            <div className="h-full bg-white/40" style={{ width: `${bufferedPercent}%` }} />
          </div>
          <div
            className="absolute top-1/2 left-0 h-1 -translate-y-1/2 rounded-full bg-accent"
            style={{ width: `${percent}%` }}
          />
          <input
            type="range"
            min={0}
            max={100}
            step={0.1}
            value={percent}
            onChange={onSeek}
            aria-label="Posição"
            className="relative h-4 w-full cursor-pointer appearance-none bg-transparent
                       [&::-webkit-slider-thumb]:h-3.5 [&::-webkit-slider-thumb]:w-3.5
                       [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full
                       [&::-webkit-slider-thumb]:bg-accent"
          />
        </div>

        <div className="flex flex-wrap items-center gap-3 text-white">
          <button
            type="button"
            onClick={togglePlay}
            aria-label={playing ? 'Pausar' : 'Reproduzir'}
            className="grid h-11 w-11 place-items-center rounded-full bg-accent text-accent-ink transition hover:opacity-90"
          >
            {playing ? <PauseIcon /> : <PlayIcon />}
          </button>

          <button
            type="button"
            onClick={() => seekBy(-SKIP_SECONDS)}
            className="rounded-lg px-2 py-1 text-xs font-medium text-white/80 transition hover:bg-white/10"
          >
            −{SKIP_SECONDS}s
          </button>
          <button
            type="button"
            onClick={() => seekBy(SKIP_SECONDS)}
            className="rounded-lg px-2 py-1 text-xs font-medium text-white/80 transition hover:bg-white/10"
          >
            +{SKIP_SECONDS}s
          </button>

          <span className="font-mono text-xs text-white/80 tabular-nums">
            {clockTime(current)} / {clockTime(duration)}
          </span>

          <div className="ml-auto flex items-center gap-2">
            <div className="hidden items-center gap-2 sm:flex">
              <button
                type="button"
                onClick={() => setMuted((m) => !m)}
                aria-label={muted ? 'Reativar som' : 'Silenciar'}
                className="grid h-9 w-9 place-items-center rounded-full transition hover:bg-white/10"
              >
                <VolumeIcon muted={muted || volume === 0} />
              </button>
              <input
                type="range"
                min={0}
                max={1}
                step={0.05}
                value={muted ? 0 : volume}
                onChange={(e) => {
                  setVolume(Number(e.target.value))
                  setMuted(false)
                }}
                aria-label="Volume"
                className="h-1 w-20 cursor-pointer appearance-none rounded-full bg-white/30
                           [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:w-3
                           [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full
                           [&::-webkit-slider-thumb]:bg-accent"
              />
            </div>

            {/* Áudio só aparece quando há escolha a fazer. Um arquivo de uma
                faixa só não ganha um menu de uma opção. */}
            {faixas && faixas.audio.length > 1 && (
              <select
                value={audioIdx ?? ''}
                onChange={(e) => setAudioIdx(e.target.value === '' ? undefined : Number(e.target.value))}
                aria-label="Faixa de áudio"
                className="max-w-36 rounded-lg bg-white/10 px-2 py-1.5 text-xs text-white outline-none"
              >
                {faixas.audio.map((f) => (
                  <option key={f.idx} value={f.idx} className="text-black">
                    {f.rotulo}
                  </option>
                ))}
              </select>
            )}

            {faixas && faixas.legendas.length > 0 && (
              <select
                value={legendaIdx ?? ''}
                onChange={(e) => setLegendaIdx(e.target.value === '' ? undefined : Number(e.target.value))}
                aria-label="Legenda"
                className="max-w-36 rounded-lg bg-white/10 px-2 py-1.5 text-xs text-white outline-none"
              >
                <option value="" className="text-black">
                  Sem legenda
                </option>
                {faixas.legendas.map((l) => (
                  <option
                    key={l.idx}
                    value={l.idx}
                    disabled={!!l.indisponivel}
                    title={l.indisponivel}
                    className="text-black"
                  >
                    {l.rotulo}
                    {l.indisponivel ? ' (imagem)' : ''}
                  </option>
                ))}
              </select>
            )}

            <select
              value={speed}
              onChange={(e) => setSpeed(Number(e.target.value))}
              aria-label="Velocidade"
              className="rounded-lg bg-white/10 px-2 py-1.5 text-xs text-white outline-none"
            >
              {SPEEDS.map((s) => (
                <option key={s} value={s} className="text-black">
                  {s}×
                </option>
              ))}
            </select>

            {next?.next && (
              <Link
                to={`/watch/${next.next}`}
                className="rounded-lg bg-white/10 px-3 py-1.5 text-xs font-medium transition hover:bg-white/20"
              >
                Próximo episódio
              </Link>
            )}

            <button
              type="button"
              onClick={() => void videoRef.current?.requestPictureInPicture?.().catch(() => {})}
              aria-label="Picture in picture"
              className="hidden h-9 w-9 place-items-center rounded-full transition hover:bg-white/10 sm:grid"
            >
              <PipIcon />
            </button>

            <button
              type="button"
              onClick={toggleFullscreen}
              aria-label="Tela cheia em paisagem"
              title="Tela cheia em paisagem"
              className="grid h-9 w-9 place-items-center rounded-full transition hover:bg-white/10"
            >
              <FullscreenIcon />
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

// Containers que nenhum navegador abre, mesmo com codecs compatíveis dentro.
const unsupportedContainers = ['.mkv', '.avi', '.wmv', '.ts', '.m2ts', '.mpg', '.mpeg']
// Codecs que o navegador realmente não decodifica.
const unsupportedVideo = ['hevc', 'h265', 'vc1', 'mpeg2video', 'vp6']
const unsupportedAudio = ['ac3', 'eac3', 'dts', 'truehd', 'pcm_bluray']

/** Explica por que o vídeo não tocou. Distinguir container de codec importa:
 *  container errado se resolve com um remux de 2 minutos, sem recodificar. */
function diagnose(file: { ext: string; vcodec?: string; acodec?: string }) {
  const ext = file.ext.toLowerCase()
  const badVideo = unsupportedVideo.includes((file.vcodec ?? '').toLowerCase())
  const badAudio = unsupportedAudio.includes((file.acodec ?? '').toLowerCase())

  if (unsupportedContainers.includes(ext) && !badVideo && !badAudio) {
    return {
      title: 'O navegador não abre este formato de arquivo',
      detail: `O vídeo em si é compatível (${file.vcodec}/${file.acodec}), mas nenhum navegador abre ${ext}. Trocar o container para .mp4 resolve sem recodificar e sem perder qualidade:`,
      fix: 'ffmpeg -i "arquivo' + ext + '" -c copy -movflags +faststart "arquivo.mp4"',
    }
  }
  if (badVideo) {
    return {
      title: 'Este navegador não decodifica o vídeo',
      detail: `O vídeo está em ${file.vcodec}, que este navegador não toca. Converter exige recodificar (demorado) — ou abra em um player local.`,
      fix: null,
    }
  }
  if (badAudio) {
    return {
      title: 'Este navegador não decodifica o áudio',
      detail: `A imagem é compatível, mas o áudio em ${file.acodec} não. Recodificar só o áudio é rápido:`,
      fix: 'ffmpeg -i "arquivo" -c:v copy -c:a aac -movflags +faststart "saida.mp4"',
    }
  }
  return {
    title: 'Não foi possível reproduzir o arquivo',
    detail: 'O navegador recusou o arquivo. Baixe e abra em um player local, como o VLC.',
    fix: null,
  }
}

/** O NAS entrega o arquivo como está; quando o navegador recusa, é melhor
 *  dizer o motivo exato do que deixar uma tela preta. */
function UnsupportedOverlay({ file, plano }: { file: PlaybackInfo; plano?: PlaybackPlan }) {
  const { title, detail, fix } = diagnose(file)
  // O servidor sabe o motivo exato (inclusive pix_fmt e perfil, que o navegador
  // não enxerga); quando ele opinou, a explicação dele vale mais.
  const motivo = plano?.motivo && plano.modo !== 'direct' ? plano.motivo : detail
  const semFerramenta = plano ? !plano.ffmpeg || !plano.transcodificacao_ativa : false

  return (
    <div className="absolute inset-0 z-10 grid place-items-center bg-black/85 px-6 text-center">
      <div className="max-w-md">
        <WarningIcon className="mx-auto text-warn" width="2em" height="2em" />
        <h2 className="mt-3 text-base font-semibold text-white">{title}</h2>
        <p className="mt-2 text-sm text-white/70">{motivo}</p>
        {semFerramenta && (
          <p className="mt-2 text-xs text-warn">
            O servidor converteria este arquivo sozinho, mas a transcodificação está
            {plano && !plano.ffmpeg ? ' sem ffmpeg' : ' desligada na configuração'}.
          </p>
        )}
        {fix && (
          <code className="mt-3 block overflow-x-auto rounded-lg bg-white/10 px-3 py-2 text-left font-mono text-[11px] text-white/80">
            {fix}
          </code>
        )}
        <a
          href={downloadUrl(file.id)}
          className="mt-5 inline-flex items-center gap-2 rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-ink transition hover:opacity-90"
        >
          <DownloadIcon /> Baixar arquivo
        </a>
      </div>
    </div>
  )
}

/**
 * O que aparece enquanto o servidor prepara o arquivo. Mostra o que está sendo
 * feito, quanto falta e a velocidade — a diferença entre "travou" e "está
 * trabalhando" é a única informação que importa aqui.
 */
function PainelDePreparo({
  plano,
  preparo,
}: {
  plano: PlaybackPlan
  preparo?: PreparoProgresso
}) {
  const rotulos: Record<string, string> = {
    remux: 'Trocando o container, sem recodificar',
    audio: 'Recodificando o áudio',
    video: 'Recodificando a imagem',
  }
  // Item só da nuvem: quem prepara é o worker na AWS, sem barra de progresso
  // fina (o evento só chega quando termina).
  const naNuvem = preparo?.receita === 'nuvem'
  const titulo = naNuvem ? 'Preparando na nuvem' : (rotulos[plano.modo] ?? 'Preparando')
  const percentual = preparo?.percentual ?? 0
  const restante = preparo?.restante_segundos ?? 0
  const erro = preparo?.estado === 'erro'

  return (
    <div className="absolute inset-0 z-10 grid place-items-center bg-black/85 px-6 text-center">
      <div className="w-full max-w-sm">
        {erro ? (
          <>
            <WarningIcon className="mx-auto text-warn" width="2em" height="2em" />
            <h2 className="mt-3 text-base font-semibold text-white">Não consegui preparar</h2>
            <p className="mt-2 text-sm text-white/70">{preparo?.erro}</p>
          </>
        ) : (
          <>
            {naNuvem && <NuvemIcon estado="processando" className="mx-auto mb-3 text-accent" width="2em" height="2em" />}
            <h2 className="text-base font-semibold text-white">{titulo}</h2>
            <p className="mt-1 text-sm text-white/60">{plano.motivo}</p>

            <div className="mt-5 h-1.5 overflow-hidden rounded-full bg-white/15">
              <div
                className={`h-full bg-accent transition-[width] duration-500 ${naNuvem ? 'animate-pulse' : ''}`}
                style={{ width: `${naNuvem ? 100 : Math.max(2, percentual)}%` }}
              />
            </div>

            <p className="mt-3 font-mono text-xs text-white/60 tabular-nums">
              {naNuvem
                ? 'o worker avisa quando terminar; pode fechar esta tela'
                : preparo?.estado === 'fila'
                ? 'na fila…'
                : `${percentual}%${restante > 0 ? ` · faltam ${clockTime(restante)}` : ''}${
                    preparo?.velocidade ? ` · ${preparo.velocidade.toFixed(0)}× tempo real` : ''
                  }`}
            </p>
            <p className="mt-4 text-xs text-white/40">
              O arquivo preparado fica em cache: na próxima vez começa na hora.
            </p>
          </>
        )}
      </div>
    </div>
  )
}
