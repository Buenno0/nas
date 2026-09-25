import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '../lib/api'
import { gradientFor } from '../lib/format'
import { Poster } from '../components/Poster'
import { EmptyState, ErrorState, Spinner } from '../components/states'

export function Colecoes() {
  const queryClient = useQueryClient()
  const [nova, setNova] = useState('')

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['colecoes'],
    queryFn: api.colecoes,
  })

  const criar = useMutation({
    mutationFn: () => api.criarColecao(nova.trim()),
    onSuccess: () => {
      setNova('')
      void queryClient.invalidateQueries({ queryKey: ['colecoes'] })
    },
  })

  if (isLoading) return <Spinner label="Carregando coleções…" />
  if (isError) return <ErrorState error={error} retry={() => void refetch()} />

  const submeter = (e: FormEvent) => {
    e.preventDefault()
    if (nova.trim()) criar.mutate()
  }

  return (
    <div className="mx-auto max-w-4xl px-4 py-6 sm:px-6">
      <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">Coleções</h1>
      <p className="mt-1 text-sm text-muted">
        Listas montadas por você — uma trilogia, uma maratona, o que assistir com alguém. São suas:
        ninguém mais vê.
      </p>

      <form onSubmit={submeter} className="mt-5 mb-8 flex max-w-md gap-2">
        <input
          value={nova}
          onChange={(e) => setNova(e.target.value)}
          placeholder="Nome da coleção"
          className="min-w-0 flex-1 rounded-lg border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-accent"
        />
        <button
          type="submit"
          disabled={!nova.trim() || criar.isPending}
          className="shrink-0 rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-ink transition hover:opacity-90 disabled:opacity-50"
        >
          Criar
        </button>
      </form>

      {criar.isError && (
        <p role="alert" className="-mt-5 mb-6 text-xs text-red-400">
          {criar.error instanceof ApiError ? criar.error.message : 'não foi possível criar'}
        </p>
      )}

      {!data || data.length === 0 ? (
        <EmptyState
          title="Nenhuma coleção ainda"
          description="Crie uma acima, ou abra um título e use o botão Coleções."
        />
      ) : (
        <ul className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4">
          {data.map((c) => (
            <li key={c.id}>
              <Link to={`/colecao/${c.id}`} className="group block">
                {c.poster ? (
                  <img
                    src={c.poster}
                    alt=""
                    loading="lazy"
                    className="aspect-[2/3] w-full rounded-xl object-cover ring-1 ring-line transition group-hover:ring-accent"
                  />
                ) : (
                  <div
                    className="aspect-[2/3] w-full rounded-xl ring-1 ring-line"
                    style={{ background: gradientFor(c.nome) }}
                  />
                )}
                <p className="mt-2 line-clamp-1 text-sm font-medium">{c.nome}</p>
                <p className="text-xs text-muted">
                  {c.itens} {c.itens === 1 ? 'título' : 'títulos'}
                </p>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

export function Colecao() {
  const { id } = useParams()
  const colecaoId = Number(id)
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [renomeando, setRenomeando] = useState(false)
  const [nome, setNome] = useState('')

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['colecao', colecaoId],
    queryFn: () => api.colecao(colecaoId),
    enabled: Number.isFinite(colecaoId),
  })

  const renomear = useMutation({
    mutationFn: () => api.renomearColecao(colecaoId, nome.trim()),
    onSuccess: () => {
      setRenomeando(false)
      void queryClient.invalidateQueries({ queryKey: ['colecao', colecaoId] })
      void queryClient.invalidateQueries({ queryKey: ['colecoes'] })
    },
  })

  const apagar = useMutation({
    mutationFn: () => api.apagarColecao(colecaoId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['colecoes'] })
      navigate('/colecoes')
    },
  })

  const remover = useMutation({
    mutationFn: (titleId: number) => api.removerDaColecao(colecaoId, titleId),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['colecao', colecaoId] }),
  })

  if (isLoading) return <Spinner />
  if (isError || !data) return <ErrorState error={error} retry={() => void refetch()} />

  return (
    <div className="mx-auto max-w-5xl px-4 py-6 sm:px-6">
      <div className="mb-6 flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          {renomeando ? (
            <form
              onSubmit={(e) => {
                e.preventDefault()
                if (nome.trim()) renomear.mutate()
              }}
              className="flex gap-2"
            >
              <input
                autoFocus
                value={nome}
                onChange={(e) => setNome(e.target.value)}
                className="rounded-lg border border-line bg-surface px-3 py-1.5 text-lg font-semibold outline-none focus:border-accent"
              />
              <button type="submit" className="rounded-lg bg-accent px-3 py-1.5 text-sm font-semibold text-accent-ink">
                Salvar
              </button>
              <button
                type="button"
                onClick={() => setRenomeando(false)}
                className="rounded-lg border border-line px-3 py-1.5 text-sm"
              >
                Cancelar
              </button>
            </form>
          ) : (
            <h1 className="text-2xl font-semibold tracking-tight">{data.colecao.nome}</h1>
          )}
          <p className="mt-1 text-sm text-muted">
            {data.itens.length} {data.itens.length === 1 ? 'título' : 'títulos'}
          </p>
        </div>

        {!renomeando && (
          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => {
                setNome(data.colecao.nome)
                setRenomeando(true)
              }}
              className="rounded-lg border border-line px-3 py-1.5 text-sm font-medium transition hover:bg-elev"
            >
              Renomear
            </button>
            <button
              type="button"
              onClick={() => {
                // Apagar uma lista curada não pode ser um clique só sem volta.
                if (confirm(`Apagar a coleção "${data.colecao.nome}"? Os títulos continuam no acervo.`)) {
                  apagar.mutate()
                }
              }}
              className="rounded-lg border border-line px-3 py-1.5 text-sm font-medium text-muted transition hover:border-red-500/40 hover:text-red-400"
            >
              Apagar
            </button>
          </div>
        )}
      </div>

      {data.itens.length === 0 ? (
        <EmptyState
          title="Coleção vazia"
          description="Abra um título no acervo e use o botão Coleções para colocá-lo aqui."
        />
      ) : (
        <ul className="grid grid-cols-3 gap-4 sm:grid-cols-4 md:grid-cols-5 xl:grid-cols-6">
          {data.itens.map((item) => (
            <li key={item.id} className="group/item relative">
              <Poster title={item} />
              <button
                type="button"
                onClick={() => remover.mutate(item.id)}
                aria-label={`Remover ${item.name} da coleção`}
                className="absolute top-1.5 right-1.5 grid h-7 w-7 place-items-center rounded-full bg-black/70 text-sm text-white opacity-0 transition group-hover/item:opacity-100 focus:opacity-100"
              >
                ×
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
