import { Link } from 'react-router'
import type { TitleCard } from '../lib/api'
import { gradientFor, humanDuration, initials, kindLabel } from '../lib/format'

interface PosterProps {
  title: TitleCard
  /** 0–1: desenha a barrinha de progresso sobre a capa. */
  progress?: number
  subtitle?: string
}

export function Poster({ title, progress, subtitle }: PosterProps) {
  const meta = subtitle ?? [title.year || '', humanDuration(title.duration)].filter(Boolean).join(' · ')

  return (
    <Link
      to={`/title/${title.id}`}
      className="group block w-full focus-visible:outline-none"
      aria-label={title.name}
    >
      <div className="sala-escura relative aspect-[2/3] overflow-hidden rounded-xl bg-elev ring-1 ring-line transition duration-200 group-hover:ring-accent group-focus-visible:ring-2 group-focus-visible:ring-accent">
        {title.poster ? (
          <img
            src={title.poster}
            alt=""
            loading="lazy"
            className="h-full w-full object-cover transition duration-300 group-hover:scale-[1.04]"
          />
        ) : (
          <div
            className="flex h-full w-full flex-col items-center justify-center gap-2 p-3 text-center"
            style={{ background: gradientFor(title.name) }}
          >
            <span className="text-2xl font-semibold text-white/90">{initials(title.name)}</span>
            <span className="line-clamp-3 text-[11px] leading-tight font-medium text-white/70">
              {title.name}
            </span>
          </div>
        )}

        <span className="absolute top-2 left-2 rounded-md bg-black/55 px-1.5 py-0.5 text-[10px] font-medium tracking-wide text-white/90 backdrop-blur-sm">
          {kindLabel[title.kind] ?? title.kind}
        </span>

        {progress !== undefined && progress > 0 && (
          <div className="absolute inset-x-0 bottom-0 h-1 bg-black/50">
            <div
              className="h-full bg-accent"
              style={{ width: `${Math.min(100, Math.round(progress * 100))}%` }}
            />
          </div>
        )}
      </div>

      <div className="mt-2 px-0.5">
        <p className="line-clamp-1 text-sm font-medium text-ink">{title.name}</p>
        {meta && <p className="line-clamp-1 text-xs text-muted">{meta}</p>}
      </div>
    </Link>
  )
}

export function PosterSkeleton() {
  return (
    <div className="w-full animate-pulse">
      <div className="aspect-[2/3] rounded-xl bg-elev" />
      <div className="mt-2 h-3.5 w-3/4 rounded bg-elev" />
      <div className="mt-1.5 h-3 w-1/2 rounded bg-elev" />
    </div>
  )
}
