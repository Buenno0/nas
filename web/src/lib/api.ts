// Cliente da API do NAS. Tudo passa por cookie de sessão; um 401 em qualquer
// chamada significa "voltar para o login".

export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: 'same-origin',
    headers: init?.body ? { 'Content-Type': 'application/json' } : undefined,
    ...init,
  })

  if (res.status === 204) return undefined as T
  if (!res.ok) {
    let message = `erro ${res.status}`
    try {
      const body = (await res.json()) as { error?: string }
      if (body.error) message = body.error
    } catch {
      // resposta sem JSON — fica a mensagem genérica
    }
    throw new ApiError(message, res.status)
  }
  return (await res.json()) as T
}

export type TitleKind = 'movie' | 'tv' | 'album' | 'photos'
export type MediaType = 'video' | 'audio' | 'photo'
export type LibraryKind = 'movie' | 'tv' | 'music' | 'photo'

export interface User {
  username: string
  must_change_password: boolean
  is_admin: boolean
}

export interface Library {
  id: number
  name: string
  /** Vazio para quem não é administrador: o caminho não é exposto. */
  path?: string
  kind: LibraryKind
  enabled: boolean
  scanned_at?: string
}

export interface TitleCard {
  id: number
  library_id: number
  kind: TitleKind
  name: string
  year?: number
  artist?: string
  rating?: number
  poster?: string
  backdrop?: string
  genres?: string
  files: number
  duration?: number
  meta_state: string
}

export interface ContinueItem {
  file_id: number
  title_id: number
  title_name: string
  kind: TitleKind
  label?: string
  poster?: string
  backdrop?: string
  position: number
  duration: number
}

export interface HomeRow {
  key: string
  title: string
  items: TitleCard[]
}

export interface HomeResponse {
  hero?: TitleCard
  continue: ContinueItem[]
  rows: HomeRow[]
}

export interface FileInfo {
  id: number
  rel_path: string
  name: string
  ext: string
  media_type: MediaType
  size: number
  duration: number
  width?: number
  height?: number
  vcodec?: string
  acodec?: string
  track?: number
  thumb?: string
  position?: number
  finished?: boolean
  season?: number
  episode?: number
  episode_name?: string
}

export interface PlaybackInfo extends FileInfo {
  title_id: number
  title_name: string
  kind: TitleKind
  poster?: string
}

export interface Season {
  number: number
  episodes: FileInfo[]
}

/** O detalhe traz os arquivos em si, não a contagem — por isso não estende
 *  TitleCard. */
export interface TitleDetail {
  id: number
  library_id: number
  kind: TitleKind
  name: string
  year?: number
  overview?: string
  rating?: number
  genres?: string
  artist?: string
  meta_state: string
  poster_url?: string
  backdrop_url?: string
  library: string
  favorite: boolean
  files: FileInfo[]
  seasons?: Season[]
}

export interface TitlesPage {
  items: TitleCard[]
  total: number
  offset: number
  limit: number
}

export interface Settings {
  port: number
  tmdb_configured: boolean
  tmdb_lang: string
  scan_every: string
  tunnel_name?: string
  ffmpeg: boolean
}

export interface MatchCandidate {
  tmdb_id: number
  name: string
  year?: number
  overview?: string
  rating?: number
  poster?: string
}

export interface ScanStatus {
  running: boolean
  stage?: string
  library?: string
  current?: number
  total?: number
  file?: string
  last_stats?: string
  last_run?: string
  last_error?: string
}

export interface TitleQuery {
  library?: number
  kind?: TitleKind
  q?: string
  sort?: 'name' | 'year' | 'recent'
  limit?: number
  offset?: number
}

function query(params: TitleQuery): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '' && value !== 0) search.set(key, String(value))
  }
  const s = search.toString()
  return s ? `?${s}` : ''
}

export const api = {
  me: () => request<User>('/api/auth/me'),
  login: (username: string, password: string) =>
    request<User>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  logout: () => request<{ ok: boolean }>('/api/auth/logout', { method: 'POST' }),
  changePassword: (current: string, next: string) =>
    request<{ ok: boolean }>('/api/auth/password', {
      method: 'POST',
      body: JSON.stringify({ current, new: next }),
    }),

  home: () => request<HomeResponse>('/api/home'),
  libraries: () => request<Library[]>('/api/libraries'),
  titles: (params: TitleQuery = {}) => request<TitlesPage>(`/api/titles${query(params)}`),
  title: (id: number) => request<TitleDetail>(`/api/titles/${id}`),
  file: (fileId: number) => request<PlaybackInfo>(`/api/files/${fileId}`),
  nextEpisode: (fileId: number) => request<{ next: number | null }>(`/api/files/${fileId}/next`),

  saveProgress: (fileId: number, position: number, duration: number) =>
    request<void>(`/api/progress/${fileId}`, {
      method: 'PUT',
      body: JSON.stringify({ position, duration }),
    }),
  setFavorite: (titleId: number, on: boolean) =>
    request<{ favorite: boolean }>(`/api/favorites/${titleId}`, {
      method: on ? 'POST' : 'DELETE',
    }),

  scan: () => request<{ started: boolean }>('/api/scan', { method: 'POST' }),
  scanStatus: () => request<ScanStatus>('/api/scan/status'),
  refreshMetadata: (all = false) =>
    request<{ started: boolean }>(`/api/metadata${all ? '?all=1' : ''}`, { method: 'POST' }),

  settings: () => request<Settings>('/api/settings'),
  saveSettings: (patch: Partial<Record<'tmdb_key' | 'tmdb_lang' | 'scan_every', string>>) =>
    request<Settings>('/api/settings', { method: 'PUT', body: JSON.stringify(patch) }),

  matches: (titleId: number, q?: string) =>
    request<{ results: MatchCandidate[] }>(
      `/api/titles/${titleId}/matches${q ? `?q=${encodeURIComponent(q)}` : ''}`,
    ),
  applyMatch: (titleId: number, tmdbId: number) =>
    request<{ ok: boolean }>(`/api/titles/${titleId}/match`, {
      method: 'POST',
      body: JSON.stringify({ tmdb_id: tmdbId }),
    }),
}

export const streamUrl = (fileId: number) => `/stream/${fileId}`
export const downloadUrl = (fileId: number) => `/stream/${fileId}?download=1`

// beaconProgress salva a posição quando a aba está fechando: fetch normal é
// cancelado no unload, sendBeacon não.
export function beaconProgress(fileId: number, position: number, duration: number) {
  const body = JSON.stringify({ position, duration })
  if (navigator.sendBeacon) {
    navigator.sendBeacon(`/api/progress/${fileId}`, new Blob([body], { type: 'text/plain' }))
    return
  }
  void fetch(`/api/progress/${fileId}`, { method: 'POST', body, keepalive: true })
}
