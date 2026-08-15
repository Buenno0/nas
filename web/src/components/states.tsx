import type { ReactNode } from 'react'
import { WarningIcon } from './icons'

export function Spinner({ label }: { label?: string }) {
  return (
    <div className="flex items-center justify-center gap-3 py-16 text-sm text-muted">
      <span className="h-4 w-4 animate-spin rounded-full border-2 border-line border-t-accent" />
      {label ?? 'Carregando…'}
    </div>
  )
}

export function EmptyState({
  title,
  description,
  action,
}: {
  title: string
  description?: string
  action?: ReactNode
}) {
  return (
    <div className="mx-auto max-w-md px-6 py-20 text-center">
      <h2 className="text-base font-semibold">{title}</h2>
      {description && <p className="mt-2 text-sm leading-relaxed text-muted">{description}</p>}
      {action && <div className="mt-5">{action}</div>}
    </div>
  )
}

export function ErrorState({ error, retry }: { error: unknown; retry?: () => void }) {
  const message = error instanceof Error ? error.message : 'algo deu errado'
  return (
    <div className="mx-auto flex max-w-md flex-col items-center gap-3 px-6 py-20 text-center">
      <WarningIcon className="text-red-400" width="1.75em" height="1.75em" />
      <p className="text-sm text-muted">{message}</p>
      {retry && (
        <button
          type="button"
          onClick={retry}
          className="rounded-lg border border-line px-3 py-1.5 text-sm font-medium transition hover:bg-elev"
        >
          Tentar de novo
        </button>
      )}
    </div>
  )
}
