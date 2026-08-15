import { useState } from 'react'
import { useParams, useSearchParams } from 'react-router'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { api, type TitleQuery } from '../lib/api'
import { Poster, PosterSkeleton } from '../components/Poster'
import { EmptyState, ErrorState } from '../components/states'

const PAGE = 60

/** Grade de uma biblioteca. A mesma tela serve à busca global (sem library). */
export function Library() {
  const { id } = useParams()
  const libraryId = id ? Number(id) : undefined
  const [sort, setSort] = useState<'name' | 'year' | 'recent'>('name')
  const [limit, setLimit] = useState(PAGE)

  const { data: libraries } = useQuery({ queryKey: ['libraries'], queryFn: api.libraries })
  const library = libraries?.find((l) => l.id === libraryId)

  const params: TitleQuery = { library: libraryId, sort, limit }
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['titles', params],
    queryFn: () => api.titles(params),
    placeholderData: keepPreviousData,
  })

  return (
    <Grid
      heading={library?.name ?? 'Acervo'}
      subtitle={data ? `${data.total} ${data.total === 1 ? 'título' : 'títulos'}` : undefined}
      toolbar={
        <label className="flex items-center gap-2 text-xs text-muted">
          Ordenar
          <select
            value={sort}
            onChange={(e) => setSort(e.target.value as typeof sort)}
            className="rounded-lg border border-line bg-surface px-2 py-1.5 text-sm text-ink outline-none focus:border-accent"
          >
            <option value="name">Nome</option>
            <option value="year">Ano</option>
            <option value="recent">Adicionados</option>
          </select>
        </label>
      }
      isLoading={isLoading}
      isError={isError}
      error={error}
      retry={() => void refetch()}
      items={data?.items ?? []}
      total={data?.total ?? 0}
      onMore={() => setLimit((l) => l + PAGE)}
      emptyTitle="Nada por aqui ainda"
      emptyDescription="Rode um scan nas configurações para indexar os arquivos desta pasta."
    />
  )
}

/** Busca global — mesma grade, filtrada pelo termo da URL. */
export function Search() {
  const [searchParams] = useSearchParams()
  const term = searchParams.get('q') ?? ''
  const [limit, setLimit] = useState(PAGE)

  const params: TitleQuery = { q: term, limit }
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['titles', params],
    queryFn: () => api.titles(params),
    enabled: term.trim().length > 0,
    placeholderData: keepPreviousData,
  })

  if (!term.trim()) {
    return <EmptyState title="Buscar" description="Digite algo na barra acima para procurar no acervo." />
  }

  return (
    <Grid
      heading={`Resultados para “${term}”`}
      subtitle={data ? `${data.total} ${data.total === 1 ? 'título' : 'títulos'}` : undefined}
      isLoading={isLoading}
      isError={isError}
      error={error}
      retry={() => void refetch()}
      items={data?.items ?? []}
      total={data?.total ?? 0}
      onMore={() => setLimit((l) => l + PAGE)}
      emptyTitle="Nenhum resultado"
      emptyDescription="Tente outro termo ou verifique se a biblioteca já foi indexada."
    />
  )
}

interface GridProps {
  heading: string
  subtitle?: string
  toolbar?: React.ReactNode
  isLoading: boolean
  isError: boolean
  error: unknown
  retry: () => void
  items: import('../lib/api').TitleCard[]
  total: number
  onMore: () => void
  emptyTitle: string
  emptyDescription: string
}

function Grid(props: GridProps) {
  const { items, total } = props

  return (
    <div className="px-4 py-6 sm:px-6">
      <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">{props.heading}</h1>
          {props.subtitle && <p className="text-sm text-muted">{props.subtitle}</p>}
        </div>
        {props.toolbar}
      </div>

      {props.isError ? (
        <ErrorState error={props.error} retry={props.retry} />
      ) : props.isLoading ? (
        <div className="grid grid-cols-3 gap-3 sm:grid-cols-4 sm:gap-4 md:grid-cols-5 xl:grid-cols-7">
          {Array.from({ length: 14 }).map((_, i) => (
            <PosterSkeleton key={i} />
          ))}
        </div>
      ) : items.length === 0 ? (
        <EmptyState title={props.emptyTitle} description={props.emptyDescription} />
      ) : (
        <>
          <div className="grid grid-cols-3 gap-3 sm:grid-cols-4 sm:gap-4 md:grid-cols-5 xl:grid-cols-7">
            {items.map((title) => (
              <Poster key={title.id} title={title} />
            ))}
          </div>

          {items.length < total && (
            <div className="mt-8 flex justify-center">
              <button
                type="button"
                onClick={props.onMore}
                className="rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium transition hover:bg-elev"
              >
                Carregar mais ({items.length} de {total})
              </button>
            </div>
          )}
        </>
      )}
    </div>
  )
}
