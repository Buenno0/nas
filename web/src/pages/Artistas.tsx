import { Link, useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api, type FileInfo } from '../lib/api'
import { usePlayer, type Track } from '../lib/player'
import { humanDuration, gradientFor, initials } from '../lib/format'
import { Poster } from '../components/Poster'
import { EmptyState, ErrorState, Spinner } from '../components/states'
import { PlayIcon, MusicIcon, ShuffleIcon } from '../components/icons'

export function Artistas() {
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['artistas'],
    queryFn: api.artistas,
  })

  if (isLoading) return <Spinner label="Carregando artistas…" />
  if (isError) return <ErrorState error={error} retry={() => void refetch()} />

  if (!data || data.length === 0) {
    return (
      <EmptyState
        title="Nenhum artista no acervo"
        description="Artistas aparecem aqui a partir das tags dos arquivos de música. Adicione uma biblioteca do tipo música e rode um scan."
      />
    )
  }

  return (
    <div className="px-4 py-6 sm:px-6">
      <h1 className="mb-1 text-xl font-semibold tracking-tight sm:text-2xl">Artistas</h1>
      <p className="mb-6 text-sm text-muted">
        {data.length} {data.length === 1 ? 'artista' : 'artistas'} no acervo.
      </p>

      <ul className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-6">
        {data.map((a) => (
          <li key={a.nome}>
            <Link to={`/artista/${encodeURIComponent(a.nome)}`} className="group block">
              {a.poster ? (
                <img
                  src={a.poster}
                  alt=""
                  loading="lazy"
                  className="aspect-square w-full rounded-full object-cover ring-1 ring-line transition group-hover:ring-accent"
                />
              ) : (
                <div
                  className="grid aspect-square w-full place-items-center rounded-full text-2xl font-semibold text-white/90 ring-1 ring-line"
                  style={{ background: gradientFor(a.nome) }}
                >
                  {initials(a.nome)}
                </div>
              )}
              <p className="mt-2 line-clamp-1 text-center text-sm font-medium">{a.nome}</p>
              <p className="text-center text-xs text-muted">
                {a.albuns} {a.albuns === 1 ? 'álbum' : 'álbuns'} · {a.faixas} faixas
              </p>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  )
}

/** Converte o que veio da API no formato da fila do player. */
function paraFila(faixas: FileInfo[], artista: string): Track[] {
  return faixas.map((f) => ({
    id: f.id,
    name: f.name,
    artist: artista,
    duration: f.duration,
    poster: f.thumb,
  }))
}

/** Embaralho de Fisher-Yates sobre uma cópia: a lista da tela não se mexe. */
function embaralhar<T>(itens: T[]): T[] {
  const copia = [...itens]
  for (let i = copia.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1))
    ;[copia[i], copia[j]] = [copia[j], copia[i]]
  }
  return copia
}

export function Artista() {
  const { nome: bruto } = useParams()
  const nome = decodeURIComponent(bruto ?? '')
  const player = usePlayer()

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['artista', nome],
    queryFn: () => api.artista(nome),
    enabled: nome !== '',
  })

  if (isLoading) return <Spinner label="Carregando…" />
  if (isError) return <ErrorState error={error} retry={() => void refetch()} />
  if (!data) return null

  const fila = paraFila(data.faixas, data.nome)

  return (
    <div className="px-4 py-6 sm:px-6">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">{data.nome}</h1>
        <p className="mt-1 text-sm text-muted">
          {[
            `${data.albuns.length} ${data.albuns.length === 1 ? 'álbum' : 'álbuns'}`,
            `${data.faixas.length} faixas`,
            humanDuration(data.duracao),
          ]
            .filter(Boolean)
            .join(' · ')}
        </p>

        {fila.length > 0 && (
          <div className="mt-4 flex flex-wrap gap-2">
            <button
              type="button"
              onClick={() => player.play(fila, 0)}
              className="inline-flex items-center gap-2 rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-ink transition hover:opacity-90"
            >
              <PlayIcon /> Tocar tudo
            </button>
            <button
              type="button"
              onClick={() => player.play(embaralhar(fila), 0)}
              className="inline-flex items-center gap-2 rounded-lg border border-line px-4 py-2 text-sm font-medium transition hover:bg-elev"
            >
              <ShuffleIcon /> Aleatório
            </button>
          </div>
        )}
      </header>

      {data.albuns.length > 0 && (
        <section className="mb-8">
          <h2 className="mb-3 text-sm font-semibold">Discografia</h2>
          <ul className="grid grid-cols-3 gap-4 sm:grid-cols-4 md:grid-cols-5 xl:grid-cols-7">
            {data.albuns.map((album) => (
              <li key={album.id}>
                <Poster title={album} />
              </li>
            ))}
          </ul>
        </section>
      )}

      {data.faixas.length > 0 && (
        <section>
          <h2 className="mb-3 text-sm font-semibold">Todas as faixas</h2>
          <ul className="divide-y divide-line overflow-hidden rounded-xl border border-line">
            {data.faixas.map((faixa, i) => (
              <li key={faixa.id}>
                <button
                  type="button"
                  onClick={() => player.play(fila, i)}
                  className="flex w-full items-center gap-3 px-3 py-2.5 text-left transition hover:bg-elev"
                >
                  <MusicIcon className="shrink-0 text-muted" />
                  <span className="min-w-0 flex-1 truncate text-sm">{faixa.name}</span>
                  <span className="shrink-0 text-xs text-muted tabular-nums">
                    {humanDuration(faixa.duration)}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  )
}
