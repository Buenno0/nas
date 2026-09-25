import { useState, type FormEvent, type ReactNode } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Library } from '../lib/api'
import { useTheme } from '../lib/theme'
import { useModoAoVivo, useModoNuvem } from '../lib/nuvem'
import { MiniPlayer } from './MiniPlayer'
import { CartaoDeEnvios } from './CartaoDeEnvios'
import { Mark } from './Mark'
import {
  ActivityIcon,
  ChevronRight,
  NuvemIcon,
  FilmIcon,
  HomeIcon,
  ListIcon,
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
  useModoAoVivo()

  return (
    <div className="min-h-full bg-bg">
      <Sidebar libraries={libraries ?? []} admin={user?.is_admin ?? false} />

      <div className="lg:pl-60">
        <TopBar key={location.pathname} />
        {/* Espaço extra embaixo para a barra de música e a nav do celular. */}
        <main className="pb-40 lg:pb-28">{children}</main>
      </div>

      <MiniPlayer />
      {/* Em Configurações o cartão completo já está na página. */}
      {location.pathname !== '/settings' && <CartaoDeEnvios />}
      <MobileNav libraries={libraries ?? []} />
    </div>
  )
}

/** Selo do modo de nuvem. Só aparece quando há nuvem para falar: num
 *  Ozymandias sem bucket configurado, o modo local é o único que existe. */
function SeloDoModo() {
  const estado = useModoNuvem()
  if (!estado || (!estado.configurada && estado.modo === 'local' && estado.papel !== 'nuvem')) return null
  const hibrido = estado.modo === 'hibrido'
  const naNuvem = estado.papel === 'nuvem'
  const rotulo = naNuvem ? 'Nuvem' : estado.modo === 'conectando' ? 'Conectando' : hibrido ? 'Híbrido' : 'Local'
  return (
    <Link
      to="/settings"
      title={hibrido ? 'Modo híbrido: o bucket está ligado' : 'Modo local: nenhuma chamada à nuvem'}
      className={[
        'hidden shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium transition sm:inline-flex',
        hibrido ? 'border-accent/40 bg-accent/10 text-accent' : 'border-line bg-surface text-muted hover:text-ink',
        estado.modo === 'conectando' ? 'animate-pulse' : '',
      ].join(' ')}
    >
      <NuvemIcon key={estado.modo} estado={naNuvem || hibrido ? 'hibrido' : estado.modo === 'conectando' ? 'conectando' : 'local'} />
      {rotulo}
    </Link>
  )
}

function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <Link to="/" className="flex items-center gap-2.5 px-3 py-1" aria-label="Ozymandias">
      <Mark size={30} />
      {/* No celular a barra é disputada com a busca: fica só a marca. */}
      {!compact && (
        <span className="text-[15px] leading-tight font-semibold tracking-tight">Ozymandias</span>
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
        {/* Só faz sentido com música indexada: um link para uma lista vazia
            é pior que link nenhum. */}
        {libraries.some((l) => l.kind === 'music') && (
          <NavLink to="/artistas" className={navClass}>
            <MusicIcon /> Artistas
          </NavLink>
        )}
      </nav>

      <div className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto">
        <p className="rotulo px-3 pt-2 pb-1">
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

        {/* As ruínas ficam junto do acervo, logo abaixo das bibliotecas. */}
        <RuinasMenu />
      </div>

      <div className="flex flex-col gap-1">
        <NavLink to="/colecoes" className={navClass}>
          <ListIcon /> Coleções
        </NavLink>
        {/* Telemetria é tela de admin; a rota HTTP já recusa as outras contas. */}
        {admin && (
          <NavLink to="/metricas" className={navClass}>
            <ActivityIcon /> Métricas
          </NavLink>
        )}
        <NavLink to="/settings" className={navClass}>
          <SettingsIcon /> {admin ? 'Configurações' : 'Minha conta'}
        </NavLink>
      </div>
    </aside>
  )
}

function RuinasMenu() {
  const [open, setOpen] = useState(() => window.location.pathname === '/ruinas')

  return (
    // Os links são <a> comuns porque cada destino precisa sair do SPA e
    // receber do servidor seu status HTTP real.
    <div
      className={[
        'mt-1 overflow-hidden rounded-xl border transition',
        open ? 'border-line bg-elev/55' : 'border-transparent',
      ].join(' ')}
    >
      <button
        type="button"
        onClick={() => setOpen((current) => !current)}
        aria-expanded={open}
        aria-controls="menu-ruinas-sidebar"
        className="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-muted transition hover:bg-elev/60 hover:text-ink"
      >
        <WarningIcon className="shrink-0 text-warn" />
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-semibold text-ink">Ruínas</span>
          <span className="rotulo block truncate text-accent">
            Escolha como tudo termina
          </span>
        </span>
        <ChevronRight
          className={`shrink-0 transition-transform duration-200 ${open ? 'rotate-90' : ''}`}
        />
      </button>

      <div
        id="menu-ruinas-sidebar"
        className={`grid transition-all duration-200 ${
          open ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0'
        }`}
      >
        <nav className="min-h-0 overflow-hidden" aria-label="Telas de erro">
          <div className="mx-3 mb-3 ml-6 border-l border-line pl-3">
            <a
              href="/ruinas/401"
              className="flex items-center gap-2 rounded-md px-2 py-1.5 text-xs text-muted transition hover:bg-surface hover:text-ink"
            >
              <span className="font-mono font-semibold text-accent">401</span>
              <span>Entrada proibida</span>
            </a>
            <a
              href="/ruinas/404"
              className="flex items-center gap-2 rounded-md px-2 py-1.5 text-xs text-muted transition hover:bg-surface hover:text-ink"
            >
              <span className="font-mono font-semibold text-warn">404</span>
              <span>Caminho perdido</span>
            </a>
            <a
              href="/ruinas/500"
              className="flex items-center gap-2 rounded-md px-2 py-1.5 text-xs text-muted transition hover:bg-surface hover:text-ink"
            >
              <span className="font-mono font-semibold text-danger">500</span>
              <span>Colapso final</span>
            </a>
            <a
              href="/ruinas/502"
              className="flex items-center gap-2 rounded-md px-2 py-1.5 text-xs text-muted transition hover:bg-surface hover:text-ink"
            >
              <span className="font-mono font-semibold text-danger">502</span>
              <span>Névoa no horizonte</span>
            </a>
            <a
              href="/ruinas/503"
              className="flex items-center gap-2 rounded-md px-2 py-1.5 text-xs text-muted transition hover:bg-surface hover:text-ink"
            >
              <span className="font-mono font-semibold text-ink">503</span>
              <span>O céu está fechado</span>
            </a>
            <a
              href="/ruinas/no-mac"
              className="flex items-center gap-2 rounded-md px-2 py-1.5 text-xs text-muted transition hover:bg-surface hover:text-ink"
            >
              <span className="font-mono font-semibold text-accent">503</span>
              <span>Isto dorme no Mac</span>
            </a>
          </div>
        </nav>
      </div>
    </div>
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

        <SeloDoModo />

        <button
          type="button"
          onClick={(e) => toggle(e)}
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
      <div className="grid grid-cols-5">
        <MobileLink to="/" label="Início" icon={<HomeIcon />} />
        <MobileLink to="/search" label="Buscar" icon={<SearchIcon />} />
        <MobileLink
          to={first ? `/library/${first.id}` : '/settings'}
          label={first ? 'Acervo' : 'Config'}
          icon={first ? <FilmIcon /> : <SettingsIcon />}
        />
        <MobileLink to="/ruinas" label="Ruínas" icon={<WarningIcon />} />
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
