import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../lib/api'
import { useScanStatus } from '../lib/useScanStatus'
import { useTheme } from '../lib/theme'
import { kindLabel } from '../lib/format'
import { MoonIcon, RefreshIcon, SunIcon } from '../components/icons'

export function Settings() {
  const { data: user } = useQuery({ queryKey: ['me'], queryFn: api.me })
  const admin = user?.is_admin ?? false

  return (
    <div className="mx-auto max-w-3xl space-y-6 px-4 py-6 sm:px-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">
          {admin ? 'Configurações' : 'Minha conta'}
        </h1>
        <p className="mt-1 text-sm text-muted">
          {admin
            ? 'Bibliotecas, metadados e aparência.'
            : `Conectado como ${user?.username ?? ''}. Bibliotecas e metadados são administrados por quem mantém o servidor.`}
        </p>
      </div>

      {/* As seções de servidor só existem para o admin — e a API recusa
          essas rotas para os demais, então esconder aqui é só cortesia. */}
      {admin && <LibrariesCard />}
      {admin && <MetadataCard />}
      <AppearanceCard />
      <PasswordCard />
    </div>
  )
}

function Card({ title, description, children }: { title: string; description?: string; children: React.ReactNode }) {
  return (
    <section className="rounded-xl border border-line bg-surface p-5">
      <h2 className="text-sm font-semibold">{title}</h2>
      {description && <p className="mt-1 text-xs leading-relaxed text-muted">{description}</p>}
      <div className="mt-4">{children}</div>
    </section>
  )
}

function LibrariesCard() {
  const queryClient = useQueryClient()
  const { data: libraries } = useQuery({ queryKey: ['libraries'], queryFn: api.libraries })

  const status = useScanStatus()

  const scan = useMutation({
    mutationFn: api.scan,
    // Quando o scan termina, o acervo mudou: recarrega as listas.
    onSuccess: () => setTimeout(() => void queryClient.invalidateQueries(), 1500),
  })

  return (
    <Card
      title="Bibliotecas"
      description="As pastas indexadas. Adicione ou remova pelo terminal: nas lib add ~/Media/Filmes --kind movie"
    >
      <ul className="divide-y divide-line overflow-hidden rounded-lg border border-line">
        {(libraries ?? []).map((lib) => (
          <li key={lib.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
            <div className="min-w-0">
              <p className="text-sm font-medium">{lib.name}</p>
              {lib.path && <p className="line-clamp-1 text-xs text-muted">{lib.path}</p>}
            </div>
            <span className="shrink-0 rounded-md bg-elev px-2 py-0.5 text-[11px] text-muted">
              {kindLabel[lib.kind] ?? lib.kind}
            </span>
          </li>
        ))}
        {(libraries ?? []).length === 0 && (
          <li className="px-3 py-4 text-sm text-muted">Nenhuma biblioteca cadastrada.</li>
        )}
      </ul>

      <div className="mt-4 flex flex-wrap items-center gap-3">
        <button
          type="button"
          onClick={() => scan.mutate()}
          disabled={status?.running || scan.isPending}
          className="inline-flex items-center gap-2 rounded-lg bg-accent px-3.5 py-2 text-sm font-semibold text-accent-ink transition hover:opacity-90 disabled:opacity-60"
        >
          <RefreshIcon className={status?.running ? 'animate-spin' : undefined} />
          {status?.running ? 'Trabalhando…' : 'Escanear agora'}
        </button>

        <p className="text-xs text-muted">
          {status?.running
            ? `${status.stage ?? ''} · ${status.file ?? ''} ${status.current ?? 0}/${status.total ?? 0}`
            : (status?.last_stats ?? 'nenhum scan nesta sessão')}
        </p>
      </div>

      {status?.last_error && (
        <p className="mt-3 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-400">
          {status.last_error}
        </p>
      )}
    </Card>
  )
}

function MetadataCard() {
  const queryClient = useQueryClient()
  const { data: settings } = useQuery({ queryKey: ['settings'], queryFn: api.settings })
  const [key, setKey] = useState('')

  const save = useMutation({
    mutationFn: () => api.saveSettings({ tmdb_key: key.trim() }),
    onSuccess: () => {
      setKey('')
      void queryClient.invalidateQueries({ queryKey: ['settings'] })
    },
  })

  const refresh = useMutation({
    mutationFn: () => api.refreshMetadata(true),
    onSuccess: () => setTimeout(() => void queryClient.invalidateQueries(), 2000),
  })

  return (
    <Card
      title="Capas e metadados (TMDB)"
      description="Com uma chave gratuita do TMDB, o NAS busca pôster, sinopse, nota e nomes de episódio. Sem ela, a capa é um quadro do próprio vídeo."
    >
      <p className="mb-3 text-xs">
        {settings?.tmdb_configured ? (
          <span className="text-emerald-400">Chave configurada.</span>
        ) : (
          <span className="text-muted">
            Nenhuma chave. Crie a sua em themoviedb.org → Configurações → API.
          </span>
        )}
        {settings && !settings.ffmpeg && (
          <span className="ml-2 text-amber-400">ffmpeg ausente: sem capas geradas localmente.</span>
        )}
      </p>

      <div className="flex flex-col gap-3 sm:max-w-md">
        <input
          type="password"
          value={key}
          onChange={(e) => setKey(e.target.value)}
          placeholder={settings?.tmdb_configured ? 'Substituir a chave' : 'Chave da API do TMDB'}
          autoComplete="off"
          className="rounded-lg border border-line bg-bg px-3 py-2 text-sm outline-none focus:border-accent"
        />

        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            onClick={() => save.mutate()}
            disabled={!key.trim() || save.isPending}
            className="rounded-lg border border-line px-3.5 py-2 text-sm font-medium transition hover:bg-elev disabled:opacity-50"
          >
            {save.isPending ? 'Salvando…' : 'Salvar chave'}
          </button>
          <button
            type="button"
            onClick={() => refresh.mutate()}
            disabled={refresh.isPending}
            className="rounded-lg border border-line px-3.5 py-2 text-sm font-medium transition hover:bg-elev disabled:opacity-50"
          >
            Buscar metadados de tudo
          </button>
        </div>

        {save.isError && (
          <p role="alert" className="text-xs text-red-400">
            {(save.error as Error).message}
          </p>
        )}
      </div>
    </Card>
  )
}

function AppearanceCard() {
  const { theme, toggle } = useTheme()
  return (
    <Card title="Aparência" description="O tema escuro é o padrão; sua escolha fica salva neste navegador.">
      <button
        type="button"
        onClick={(e) => toggle(e)}
        className="inline-flex items-center gap-2 rounded-lg border border-line px-3.5 py-2 text-sm font-medium transition hover:bg-elev"
      >
        {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
        {theme === 'dark' ? 'Usar tema claro' : 'Usar tema escuro'}
      </button>
    </Card>
  )
}

function PasswordCard() {
  const queryClient = useQueryClient()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')

  const change = useMutation({
    mutationFn: () => api.changePassword(current, next),
    onSuccess: () => {
      setCurrent('')
      setNext('')
      // Trocar a senha derruba todas as sessões: volta para o login.
      void queryClient.invalidateQueries()
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    change.mutate()
  }

  return (
    <Card title="Senha" description="Ao trocar a senha, todas as sessões são encerradas — inclusive esta.">
      <form onSubmit={submit} className="flex flex-col gap-3 sm:max-w-sm">
        <input
          type="password"
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
          placeholder="Senha atual"
          autoComplete="current-password"
          required
          className="rounded-lg border border-line bg-bg px-3 py-2 text-sm outline-none focus:border-accent"
        />
        <input
          type="password"
          value={next}
          onChange={(e) => setNext(e.target.value)}
          placeholder="Nova senha (mínimo 8 caracteres)"
          autoComplete="new-password"
          minLength={8}
          required
          className="rounded-lg border border-line bg-bg px-3 py-2 text-sm outline-none focus:border-accent"
        />
        {change.isError && (
          <p role="alert" className="text-xs text-red-400">
            {(change.error as Error).message}
          </p>
        )}
        <button
          type="submit"
          disabled={change.isPending}
          className="w-fit rounded-lg border border-line px-3.5 py-2 text-sm font-medium transition hover:bg-elev disabled:opacity-60"
        >
          {change.isPending ? 'Trocando…' : 'Trocar senha'}
        </button>
      </form>
    </Card>
  )
}
