import { usePlayer } from '../lib/player'
import { clockTime, gradientFor } from '../lib/format'
import { ChevronLeft, ChevronRight, PauseIcon, PlayIcon } from './icons'

/** Barra fixa de música: continua tocando enquanto se navega pelo acervo. */
export function MiniPlayer() {
  const { current, playing, position, duration, toggle, next, previous, seek, close, queue, index } =
    usePlayer()

  if (!current) return null

  const percent = duration > 0 ? (position / duration) * 100 : 0

  return (
    <div className="fixed inset-x-0 bottom-14 z-30 border-t border-line bg-surface/95 backdrop-blur-md lg:bottom-0 lg:left-60">
      <div className="h-0.5 w-full bg-elev">
        <div className="h-full bg-accent transition-[width]" style={{ width: `${percent}%` }} />
      </div>

      <div className="flex items-center gap-3 px-3 py-2 sm:px-5">
        <div
          className="h-11 w-11 shrink-0 overflow-hidden rounded-lg"
          style={current.poster ? undefined : { background: gradientFor(current.name) }}
        >
          {current.poster && <img src={current.poster} alt="" className="h-full w-full object-cover" />}
        </div>

        <div className="min-w-0 flex-1">
          <p className="line-clamp-1 text-sm font-medium">{current.name}</p>
          <p className="line-clamp-1 text-xs text-muted">
            {[current.artist, current.album].filter(Boolean).join(' · ') || 'Tocando agora'}
          </p>
        </div>

        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={previous}
            aria-label="Faixa anterior"
            className="grid h-9 w-9 place-items-center rounded-full text-muted transition hover:bg-elev hover:text-ink"
          >
            <ChevronLeft />
          </button>
          <button
            type="button"
            onClick={toggle}
            aria-label={playing ? 'Pausar' : 'Tocar'}
            className="grid h-10 w-10 place-items-center rounded-full bg-accent text-accent-ink transition hover:opacity-90"
          >
            {playing ? <PauseIcon /> : <PlayIcon />}
          </button>
          <button
            type="button"
            onClick={next}
            disabled={index + 1 >= queue.length}
            aria-label="Próxima faixa"
            className="grid h-9 w-9 place-items-center rounded-full text-muted transition hover:bg-elev hover:text-ink disabled:opacity-40"
          >
            <ChevronRight />
          </button>
        </div>

        <div className="hidden items-center gap-3 sm:flex">
          <span className="font-mono text-xs text-muted tabular-nums">
            {clockTime(position)} / {clockTime(duration || current.duration)}
          </span>
          <input
            type="range"
            min={0}
            max={duration || current.duration || 0}
            step={1}
            value={position}
            onChange={(e) => seek(Number(e.target.value))}
            aria-label="Posição da faixa"
            className="h-1 w-32 cursor-pointer appearance-none rounded-full bg-elev
                       [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:w-3
                       [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full
                       [&::-webkit-slider-thumb]:bg-accent"
          />
        </div>

        <button
          type="button"
          onClick={close}
          aria-label="Fechar player"
          className="grid h-9 w-9 shrink-0 place-items-center rounded-full text-muted transition hover:bg-elev hover:text-ink"
        >
          <svg viewBox="0 0 24 24" width="1.1em" height="1.1em" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round">
            <path d="m6 6 12 12M18 6 6 18" />
          </svg>
        </button>
      </div>
    </div>
  )
}
