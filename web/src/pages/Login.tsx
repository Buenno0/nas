import { useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../lib/api'
import { useTheme } from '../lib/theme'
import { MoonIcon, SunIcon } from '../components/icons'
import { Mark } from '../components/Mark'

export function Login() {
  const queryClient = useQueryClient()
  const { theme, toggle } = useTheme()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')

  const login = useMutation({
    mutationFn: () => api.login(username, password),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['me'] }),
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    login.mutate()
  }

  return (
    <div className="relative grid min-h-full place-items-center overflow-hidden px-4 py-12">
      {/* Brilho de fundo — decorativo, some para leitores de tela. */}
      <div
        aria-hidden
        className="pointer-events-none absolute -top-40 left-1/2 h-96 w-[36rem] -translate-x-1/2 rounded-full opacity-30 blur-3xl"
        style={{ background: 'radial-gradient(circle, var(--accent), transparent 70%)' }}
      />

      <button
        type="button"
        onClick={toggle}
        aria-label={theme === 'dark' ? 'Mudar para o tema claro' : 'Mudar para o tema escuro'}
        className="absolute top-4 right-4 grid h-9 w-9 place-items-center rounded-full border border-line bg-surface text-muted transition hover:text-ink"
      >
        {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
      </button>

      <form
        onSubmit={submit}
        className="relative w-full max-w-sm rounded-2xl border border-line bg-surface p-6 shadow-2xl shadow-black/20"
      >
        <div className="mb-6 flex items-center gap-3">
          <Mark size={42} />
          <div>
            <h1 className="text-lg font-semibold tracking-tight">
              NAS <span className="text-muted">Ozymandias</span>
            </h1>
            <p className="text-xs text-muted">Seu acervo, na sua rede</p>
          </div>
        </div>

        <label className="mb-3 block">
          <span className="mb-1.5 block text-xs font-medium text-muted">Usuário</span>
          <input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
            autoFocus
            required
            className="w-full rounded-lg border border-line bg-bg px-3 py-2 text-sm outline-none focus:border-accent"
          />
        </label>

        <label className="mb-5 block">
          <span className="mb-1.5 block text-xs font-medium text-muted">Senha</span>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            required
            className="w-full rounded-lg border border-line bg-bg px-3 py-2 text-sm outline-none focus:border-accent"
          />
        </label>

        {login.isError && (
          <p
            role="alert"
            className="mb-4 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-400"
          >
            {(login.error as Error).message}
          </p>
        )}

        <button
          type="submit"
          disabled={login.isPending}
          className="w-full rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-accent-ink transition hover:opacity-90 disabled:opacity-60"
        >
          {login.isPending ? 'Entrando…' : 'Entrar'}
        </button>

        {/* Isca para quem chegou aqui sem conta: as ruínas são públicas. */}
        <Link
          to="/ruinas"
          className="mt-5 block text-center text-xs text-muted transition hover:text-accent"
        >
          Sem conta? Veja o que há do outro lado da porta
        </Link>
      </form>
    </div>
  )
}
