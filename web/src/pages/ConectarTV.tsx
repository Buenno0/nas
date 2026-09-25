import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import { api } from '../lib/api'
import { Mark } from '../components/Mark'

function normalizeCode(value: string) {
  const compact = value.toUpperCase().replace(/[^A-Z2-9]/g, '').slice(0, 8)
  return compact.length > 4 ? `${compact.slice(0, 4)}-${compact.slice(4)}` : compact
}

export function ConectarTV() {
  const [params] = useSearchParams()
  const [code, setCode] = useState(() => normalizeCode(params.get('codigo') ?? ''))
  const approve = useMutation({ mutationFn: () => api.approveDevice(code) })

  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (code.replace('-', '').length === 8) approve.mutate()
  }

  return (
    <main className="grid min-h-full place-items-center bg-bg px-5 py-10 text-ink">
      <section className="w-full max-w-md rounded-3xl border border-line bg-surface p-7 shadow-2xl">
        <div className="mb-7 flex items-center gap-3">
          <Mark size={44} />
          <div>
            <h1 className="text-2xl font-semibold">Conectar uma TV</h1>
            <p className="text-sm text-muted">Autorize o Ozymandias neste aparelho.</p>
          </div>
        </div>

        {approve.isSuccess ? (
          <div className="rounded-2xl border border-emerald-500/30 bg-emerald-500/10 p-5">
            <p className="font-semibold text-emerald-400">TV conectada</p>
            <p className="mt-2 text-sm text-muted">
              {approve.data.device_name} já pode entrar. Você pode fechar esta página.
            </p>
          </div>
        ) : (
          <form className="space-y-5" onSubmit={submit}>
            <label className="block">
              <span className="mb-2 block text-sm font-medium">Código mostrado na TV</span>
              <input
                autoFocus
                value={code}
                onChange={(event) => setCode(normalizeCode(event.target.value))}
                inputMode="text"
                autoComplete="one-time-code"
                placeholder="ABCD-EFGH"
                className="w-full rounded-xl border border-line bg-bg px-4 py-4 text-center font-mono text-2xl tracking-[0.18em] outline-none focus:border-accent"
              />
            </label>
            {approve.isError && (
              <p className="rounded-xl bg-red-500/10 p-3 text-sm text-red-400" role="alert">
                {(approve.error as Error).message}
              </p>
            )}
            <button
              type="submit"
              disabled={approve.isPending || code.replace('-', '').length !== 8}
              className="w-full rounded-xl bg-accent px-5 py-4 font-semibold text-black disabled:opacity-40"
            >
              {approve.isPending ? 'Conectando…' : 'Conectar TV'}
            </button>
            <p className="text-center text-xs leading-relaxed text-muted">
              Autorize apenas uma TV que esteja sob seu controle. O código vence em dez minutos.
            </p>
          </form>
        )}
      </section>
    </main>
  )
}
