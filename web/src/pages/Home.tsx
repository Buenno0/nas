import { Link } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api, type ContinueItem, type TitleCard } from '../lib/api'
import { clockTime, gradientFor, humanDuration, kindLabel } from '../lib/format'
import { Poster, PosterSkeleton } from '../components/Poster'
import { Row, RowItem } from '../components/Row'
import { EmptyState, ErrorState } from '../components/states'
import { PlayIcon } from '../components/icons'

export function Home() {
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['home'],
    queryFn: api.home,
  })

  if (isError) return <ErrorState error={error} retry={() => void refetch()} />

  if (isLoading) {
    return (
      <div className="space-y-8 py-6">
        <div className="mx-4 h-64 animate-pulse rounded-2xl bg-elev sm:mx-6 sm:h-80" />
        <div className="flex gap-4 px-4 sm:px-6">
          {Array.from({ length: 6 }).map((_, i) => (
            <div key={i} className="w-32 shrink-0 sm:w-36 md:w-40">
              <PosterSkeleton />
            </div>
          ))}
        </div>
      </div>
    )
  }

  const empty = !data?.hero && (data?.rows.length ?? 0) === 0 && (data?.continue.length ?? 0) === 0
  if (empty) {
    return (
      <EmptyState
        title="Seu acervo está vazio"
        description="Adicione uma pasta como biblioteca e rode um scan para o NAS indexar seus filmes, séries, músicas e fotos."
        action={
          <Link
            to="/settings"
            className="inline-block rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-ink"
          >
            Ir para configurações
          </Link>
        }
      />
    )
  }

  return (
    <div className="space-y-10 py-6">
      {data?.hero && <Hero title={data.hero} />}

      {data && data.continue.length > 0 && (
        <Row title="Continuar assistindo">
          {data.continue.map((item) => (
            <div key={item.file_id} className="w-56 shrink-0 snap-start sm:w-64">
              <ContinueCard item={item} />
            </div>
          ))}
        </Row>
      )}

      {data?.rows.map((row) => (
        <Row key={row.key} title={row.title}>
          {row.items.map((item) => (
            <RowItem key={item.id}>
              <Poster title={item} />
            </RowItem>
          ))}
        </Row>
      ))}
    </div>
  )
}

function Hero({ title }: { title: TitleCard }) {
  return (
    <section className="px-4 sm:px-6">
      <div className="relative overflow-hidden rounded-2xl ring-1 ring-line">
        <div
          className="absolute inset-0"
          style={
            title.backdrop
              ? { backgroundImage: `url(${title.backdrop})`, backgroundSize: 'cover', backgroundPosition: 'center' }
              : { background: gradientFor(title.name) }
          }
        />
        <div className="absolute inset-0 bg-gradient-to-t from-black/85 via-black/45 to-transparent" />

        <div className="relative flex min-h-[16rem] flex-col justify-end gap-3 p-5 sm:min-h-[20rem] sm:p-8">
          <span className="w-fit rounded-md bg-white/15 px-2 py-0.5 text-[11px] font-medium text-white backdrop-blur-sm">
            {kindLabel[title.kind] ?? title.kind}
          </span>
          <h1 className="max-w-2xl text-2xl leading-tight font-bold text-white sm:text-4xl">
            {title.name}
          </h1>
          <p className="text-sm text-white/70">
            {[title.year, humanDuration(title.duration), title.files > 1 ? `${title.files} arquivos` : '']
              .filter(Boolean)
              .join(' · ')}
          </p>
          <div className="mt-1">
            <Link
              to={`/title/${title.id}`}
              className="inline-flex items-center gap-2 rounded-lg bg-white px-4 py-2 text-sm font-semibold text-black transition hover:bg-white/90"
            >
              <PlayIcon /> Abrir
            </Link>
          </div>
        </div>
      </div>
    </section>
  )
}

function ContinueCard({ item }: { item: ContinueItem }) {
  const percent = item.duration > 0 ? Math.min(100, (item.position / item.duration) * 100) : 0
  const remaining = Math.max(0, item.duration - item.position)

  return (
    <Link
      to={`/watch/${item.file_id}`}
      className="group block overflow-hidden rounded-xl ring-1 ring-line transition hover:ring-accent"
    >
      <div className="relative aspect-video">
        <div
          className="absolute inset-0"
          style={
            item.backdrop || item.poster
              ? {
                  backgroundImage: `url(${item.backdrop || item.poster})`,
                  backgroundSize: 'cover',
                  backgroundPosition: 'center',
                }
              : { background: gradientFor(item.title_name) }
          }
        />
        <div className="absolute inset-0 bg-black/35 transition group-hover:bg-black/20" />
        <span className="absolute inset-0 grid place-items-center text-white">
          <span className="grid h-11 w-11 place-items-center rounded-full bg-black/55 backdrop-blur-sm">
            <PlayIcon />
          </span>
        </span>
        <div className="absolute inset-x-0 bottom-0 h-1 bg-black/50">
          <div className="h-full bg-accent" style={{ width: `${percent}%` }} />
        </div>
      </div>

      <div className="bg-surface px-3 py-2">
        <p className="line-clamp-1 text-sm font-medium">{item.title_name}</p>
        <p className="text-xs text-muted">
          {[item.label, remaining > 0 ? `faltam ${clockTime(remaining)}` : ''].filter(Boolean).join(' · ')}
        </p>
      </div>
    </Link>
  )
}
