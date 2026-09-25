import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '../lib/api'
import { ListIcon } from './icons'

/**
 * Botão de "adicionar a uma coleção" com o painel de escolha.
 *
 * As caixas já vêm marcadas com as listas em que o título está: sem isso a
 * pessoa teria de lembrar de cor onde já pôs cada coisa.
 */
export function ColecaoPicker({ titleId }: { titleId: number }) {
  const [aberto, setAberto] = useState(false)
  const [nova, setNova] = useState('')
  const queryClient = useQueryClient()

  const { data: colecoes } = useQuery({
    queryKey: ['colecoes'],
    queryFn: api.colecoes,
    enabled: aberto,
  })
  const { data: pertence } = useQuery({
    queryKey: ['colecoes-do-titulo', titleId],
    queryFn: () => api.colecoesDoTitulo(titleId),
    enabled: aberto,
  })

  const atualizar = () => {
    void queryClient.invalidateQueries({ queryKey: ['colecoes'] })
    void queryClient.invalidateQueries({ queryKey: ['colecoes-do-titulo', titleId] })
  }

  const alternar = useMutation({
    mutationFn: ({ id, dentro }: { id: number; dentro: boolean }) =>
      dentro ? api.removerDaColecao(id, titleId) : api.adicionarNaColecao(id, titleId),
    onSuccess: atualizar,
  })

  const criar = useMutation({
    mutationFn: async () => {
      const col = await api.criarColecao(nova.trim())
      await api.adicionarNaColecao(col.id, titleId)
    },
    onSuccess: () => {
      setNova('')
      atualizar()
    },
  })

  const dentroDe = new Set(pertence?.colecoes ?? [])

  const submeter = (e: FormEvent) => {
    e.preventDefault()
    if (nova.trim()) criar.mutate()
  }

  return (
    <div className="relative">
      <button
        type="button"
        onClick={() => setAberto((v) => !v)}
        aria-expanded={aberto}
        className="inline-flex items-center gap-2 rounded-lg border border-line bg-surface px-3.5 py-2.5 text-sm font-medium text-muted transition hover:text-ink"
      >
        <ListIcon /> Coleções
      </button>

      {aberto && (
        <>
          {/* Clique fora fecha. Um painel que só fecha pelo próprio botão é o
              tipo de coisa que faz a pessoa achar que a página travou. */}
          <button
            type="button"
            aria-label="Fechar"
            onClick={() => setAberto(false)}
            className="fixed inset-0 z-30 cursor-default"
          />
          <div className="absolute z-40 mt-2 w-72 rounded-xl border border-line bg-surface p-3 shadow-xl">
            <p className="mb-2 text-[11px] font-semibold tracking-wider text-muted uppercase">
              Salvar em
            </p>

            {colecoes && colecoes.length > 0 ? (
              <ul className="mb-3 max-h-56 space-y-0.5 overflow-y-auto">
                {colecoes.map((c) => {
                  const dentro = dentroDe.has(c.id)
                  return (
                    <li key={c.id}>
                      <button
                        type="button"
                        onClick={() => alternar.mutate({ id: c.id, dentro })}
                        className="flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-left text-sm transition hover:bg-elev"
                      >
                        <span
                          aria-hidden
                          className={[
                            'grid h-4 w-4 shrink-0 place-items-center rounded border text-[10px]',
                            dentro ? 'border-accent bg-accent text-accent-ink' : 'border-line',
                          ].join(' ')}
                        >
                          {dentro ? '✓' : ''}
                        </span>
                        <span className="min-w-0 flex-1 truncate">{c.nome}</span>
                        <span className="shrink-0 text-xs text-muted">{c.itens}</span>
                      </button>
                    </li>
                  )
                })}
              </ul>
            ) : (
              <p className="mb-3 text-xs leading-relaxed text-muted">
                Nenhuma coleção ainda. Crie a primeira abaixo.
              </p>
            )}

            <form onSubmit={submeter} className="flex gap-2">
              <input
                value={nova}
                onChange={(e) => setNova(e.target.value)}
                placeholder="Nova coleção"
                className="min-w-0 flex-1 rounded-lg border border-line bg-bg px-2.5 py-1.5 text-sm outline-none focus:border-accent"
              />
              <button
                type="submit"
                disabled={!nova.trim() || criar.isPending}
                className="shrink-0 rounded-lg bg-accent px-3 py-1.5 text-sm font-semibold text-accent-ink disabled:opacity-50"
              >
                Criar
              </button>
            </form>

            {criar.isError && (
              <p role="alert" className="mt-2 text-xs text-red-400">
                {criar.error instanceof ApiError ? criar.error.message : 'não foi possível criar'}
              </p>
            )}
          </div>
        </>
      )}
    </div>
  )
}
