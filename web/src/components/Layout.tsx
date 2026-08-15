import { useState, type FormEvent, type ReactNode } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Library } from '../lib/api'
import { useTheme } from '../lib/theme'
import { MiniPlayer } from './MiniPlayer'
import { Mark } from './Mark'
import {
  FilmIcon,
  HomeIcon,
  LogoutIcon,
  MoonIcon,
  MusicIcon,
  PhotoIcon,
  SearchIcon,
  SettingsIcon,
  SunIcon,
  TvIcon,
  WarningIcon,
} from './icons'

const libraryIcon = {
  movie: FilmIcon,
  tv: TvIcon,
  music: MusicIcon,
  photo: PhotoIcon,
}

function navClass({ isActive }: { isActive: boolean }) {
  return [
    'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition',
    isActive ? 'bg-elev text-ink' : 'text-muted hover:bg-elev/60 hover:text-ink',
  ].join(' ')
}

export function Layout({ children }: { children: ReactNode }) {
  const { data: libraries } = useQuery({ queryKey: ['libraries'], queryFn: api.libraries })
  const { data: user } = useQuery({ queryKey: ['me'], queryFn: api.me })
  const location = useLocation()

  return (
    <div className="min-h-full bg-bg">
      <Sidebar libraries={libraries ?? []} admin={user?.is_admin ?? false} />

      <div className="lg:pl-60">
        <TopBar key={location.pathname} />
        {/* Espaço extra embaixo para a barra de música e a nav do celular. */}
        <main className="pb-40 lg:pb-28">{children}</main>
      </div>

      <MiniPlayer />
      <MobileNav libraries={libraries ?? []} />
    </div>
  )
}

function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <Link to="/" className="flex items-center gap-2.5 px-3 py-1" aria-label="NAS Ozymandias">
      <Mark size={30} />
      {/* No celular a barra é disputada com a busca: fica só a marca. */}
      {!compact && (
        <span className="text-[15px] leading-tight font-semibold tracking-tight">
          NAS <span className="text-muted">Ozymandias</span>
        </span>
      )}
    </Link>
  )
}

function Sidebar({ libraries, admin }: { libraries: Library[]; admin: boolean }) {
  return (
    <aside className="fixed inset-y-0 left-0 z-30 hidden w-60 flex-col gap-6 border-r border-line bg-surface px-3 py-4 lg:flex">
      <Brand />

      <nav className="flex flex-col gap-1">
        <NavLink to="/" end className={navClass}>
          <HomeIcon /> Início
        </NavLink>
        <NavLink to="/search" className={navClass}>
          <SearchIcon /> Buscar
        </NavLink>
      </nav>

      <div className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto">
        <p className="px-3 pt-2 pb-1 text-[11px] font-semibold tracking-wider text-muted uppercase">
          Bibliotecas
        </p>
        {libraries.length === 0 && (
          <p className="px-3 py-2 text-xs leading-relaxed text-muted">
            Nenhuma ainda. Adicione com <code className="text-ink">nas lib add</code>.
          </p>
        )}
        {libraries.map((lib) => {
          const Icon = libraryIcon[lib.kind] ?? FilmIcon
          return (
            <NavLink key={lib.id} to={`/library/${lib.id}`} className={navClass}>
              <Icon /> <span className="line-clamp-1">{lib.name}</span>
            </NavLink>
          )
        })}
      </div>

      <div className="flex flex-col gap-1">
        <NavLink to="/settings" className={navClass}>
          <SettingsIcon /> {admin ? 'Configurações' : 'Minha conta'}
        </NavLink>
        {/* Atalho para as três telas de erro intencionais. */}
        <NavLink to="/ruinas" className={navClass}>
          <WarningIcon /> Ruínas
        </NavLink>
      </div>
    </aside>
  )
}

function TopBar() {
  const navigate = useNavigate()
  const location = useLocation()
  const queryClient = useQueryClient()
  const { theme, toggle } = useTheme()
  const [term, setTerm] = useState(() => new URLSearchParams(location.search).get('q') ?? '')

  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: () => queryClient.invalidateQueries(),
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    navigate(`/search?q=${encodeURIComponent(term)}`)
  }

  return (
    <header className="sticky top-0 z-20 border-b border-line/70 bg-bg/80 backdrop-blur-md">
      <div className="flex items-center gap-3 px-4 py-3 sm:px-6">
        <div className="lg:hidden">
          <Brand compact />
        </div>

        <form onSubmit={submit} className="ml-auto flex-1 lg:ml-0 lg:max-w-md">
          <label className="flex items-center gap-2 rounded-full border border-line bg-surface px-3 py-1.5 focus-within:border-accent">
            <SearchIcon className="shrink-0 text-muted" />
            <input
              type="search"
              value={term}
              onChange={(e) => setTerm(e.target.value)}
              placeholder="Buscar no acervo"
              className="w-full bg-transparent text-sm outline-none placeholder:text-muted"
            />
          </label>
        </form>

        <button
          type="button"
          onClick={toggle}
          aria-label={theme === 'dark' ? 'Mudar para o tema claro' : 'Mudar para o tema escuro'}
          className="grid h-9 w-9 shrink-0 place-items-center rounded-full border border-line bg-surface text-muted transition hover:text-ink"
        >
          {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
        </button>

        <button
          type="button"
          onClick={() => logout.mutate()}
          aria-label="Sair"
          className="grid h-9 w-9 shrink-0 place-items-center rounded-full border border-line bg-surface text-muted transition hover:text-ink"
        >
          <LogoutIcon />
        </button>
      </div>
    </header>
  )
}

function MobileNav({ libraries }: { libraries: Library[] }) {
  const first = libraries[0]
  return (
    <nav className="fixed inset-x-0 bottom-0 z-30 border-t border-line bg-surface/95 pb-[env(safe-area-inset-bottom)] backdrop-blur-md lg:hidden">
      <div className="grid grid-cols-4">
        <MobileLink to="/" label="Início" icon={<HomeIcon />} />
        <MobileLink to="/search" label="Buscar" icon={<SearchIcon />} />
        <MobileLink
          to={first ? `/library/${first.id}` : '/settings'}
          label={first ? 'Acervo' : 'Config'}
          icon={first ? <FilmIcon /> : <SettingsIcon />}
        />
        <MobileLink to="/settings" label="Ajustes" icon={<SettingsIcon />} />
      </div>
    </nav>
  )
}

function MobileLink({ to, label, icon }: { to: string; label: string; icon: ReactNode }) {
  return (
    <NavLink
      to={to}
      end={to === '/'}
      className={({ isActive }) =>
        [
          'flex flex-col items-center gap-1 py-2.5 text-[11px] font-medium transition',
          isActive ? 'text-accent' : 'text-muted',
        ].join(' ')
      }
    >
      {icon}
      {label}
    </NavLink>
  )
}
