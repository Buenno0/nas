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
  /** No híbrido, todo arquivo local dela ganha cópia na nuvem. */
  espelhada?: boolean
}

export type AcaoDeNuvem = 'enviar' | 'fixar' | 'liberar' | 'remover'

export interface TarefaDeSincronizacao {
  file_id: number
  tipo: AcaoDeNuvem
  nome: string
  feitos: number
  total: number
  estado: 'fila' | 'trabalhando' | 'pausado' | 'erro'
  erro?: string
}

export interface EstadoSincronizacao {
  tarefas: TarefaDeSincronizacao[]
  eventos_pendentes: number
  ultima_reconciliacao?: string
  reconciliando: boolean
  erro?: string
  /** Bateria e temperatura do Mac: decidem o bursting para os workers. */
  energia?: { na_bateria: boolean; carga?: number; quente: boolean; motivo?: string }
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
  /** Nenhum arquivo do título tem cópia no Mac. */
  so_na_nuvem?: boolean
  /** Nenhum arquivo tem cópia no bucket: na instância cloud, indisponível. */
  so_no_mac?: boolean
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

export interface FotoDoDia extends FileInfo {
  title_id: number
}

export interface ArtistaCard {
  nome: string
  albuns: number
  faixas: number
  duracao: number
  poster?: string
}

export interface ArtistaDetalhe {
  nome: string
  albuns: TitleCard[]
  faixas: FileInfo[]
  duracao: number
}

export interface Colecao {
  id: number
  nome: string
  itens: number
  poster?: string
  criada_em: number
  atualizada_em: number
}

export interface HomeRow {
  key: string
  title: string
  items: TitleCard[]
}

export interface HomeResponse {
  hero?: TitleCard
  continue: ContinueItem[]
  /** Começado há mais de 30 dias e nunca terminado. */
  esquecidos?: ContinueItem[]
  /** Fotos deste mesmo dia, em anos anteriores. */
  no_dia?: FotoDoDia[]
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
  /** Foto: data de captura (EXIF), com o mtime do arquivo como reserva. */
  quando?: number
  localizacao?: Localizacao
  /** O worker gerando a versão que toca em qualquer navegador. */
  preparo_nuvem?: 'preparando' | 'pronto' | 'falhou'
}

/** Onde o arquivo mora. "nuvem" e "baixando" não têm cópia no Mac. */
export type Localizacao = 'local' | 'enviando' | 'ambos' | 'baixando' | 'nuvem'
export const soNaNuvem = (l?: Localizacao) => l === 'nuvem' || l === 'baixando'

export type ModoNuvem = 'local' | 'conectando' | 'hibrido'

export interface EstadoNuvem {
  modo: ModoNuvem
  erro?: string
  desde: string
  /** --sem-nuvem: o híbrido está travado nesta execução. */
  travado: boolean
  configurada: boolean
  suporte: boolean
  nuvem_bloqueadas_total: number
  /** "nuvem" na instância cloud (nas serve --nuvem). */
  papel?: 'mac' | 'nuvem'
}

export interface Upload {
  id: number
  library_id: number
  key: string
  nome: string
  tamanho: number
  parte_tamanho: number
  content_type: string
  estado: 'enviando' | 'concluido' | 'abortado'
  created_at: number
}

export interface ParteEnviada {
  n: number
  etag: string
}

export type PlaybackMode = 'direct' | 'remux' | 'audio' | 'video'

export interface PreparoProgresso {
  estado: 'ausente' | 'fila' | 'trabalhando' | 'pronto' | 'erro'
  receita?: string
  segundos_prontos: number
  segundos_total: number
  percentual: number
  velocidade: number
  restante_segundos: number
  erro?: string
}

export interface PlaybackPlan {
  modo: PlaybackMode
  motivo: string
  /** Vazio enquanto o preparo não terminou. */
  url: string
  url_direta: string
  preparo?: PreparoProgresso
  ffmpeg: boolean
  transcodificacao_ativa: boolean
  localizacao?: Localizacao
  /** Só na nuvem, com o modo local: existe, mas não toca. */
  indisponivel?: boolean
}

export interface Faixa {
  idx: number
  codec?: string
  lang?: string
  rotulo: string
  canais?: number
  padrao?: boolean
  forcada?: boolean
  externa?: boolean
  /** Só legenda: a URL do WebVTT já convertido. */
  url?: string
  /** Preenchido quando a faixa existe mas não dá para usar (legenda de imagem). */
  indisponivel?: string
}

export interface Faixas {
  audio: Faixa[]
  legendas: Faixa[]
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

export interface ProcessSample {
  cpu_percent: number
  cpu_nucleos: number
  heap_bytes: number
  rss_pico_bytes: number
  goroutines: number
  nucleos: number
}

export interface RouteSnapshot {
  rota: string
  total: number
  erros_4xx: number
  erros_5xx: number
  taxa_erro: number
  media_ms: number
  max_ms: number
  p50_ms: number
  p95_ms: number
}

export interface TrafficSummary {
  total: number
  erros: number
  taxa_erro: number
  descartadas?: number
  rotas: RouteSnapshot[]
}

export interface MetricsSnapshot {
  uptime_segundos: number
  modo: 'local' | 'tunnel'
  processo: ProcessSample
  disco_livre_bytes: number
  disco_total_bytes: number
  disco_reserva_bytes: number
  cache_usado_bytes: number
  cache_limite_bytes: number
  trafego: TrafficSummary
  modo_nuvem?: ModoNuvem
  nuvem_bloqueadas_total?: number
}

export interface TitleQuery {
  library?: number
  kind?: TitleKind
  q?: string
  sort?: 'name' | 'year' | 'recent'
  limit?: number
  offset?: number
}

function query(params: TitleQuery | Record<string, string | number | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '' && value !== 0) search.set(key, String(value))
  }
  const s = search.toString()
  return s ? `?${s}` : ''
}

// --- Tela técnica -----------------------------------------------------------

export interface NotaDoDiario {
  id: number
  /** ms desde a época */
  em: number
  tipo: string
  upload_id?: number
  file_id?: number
  nome?: string
  dados: Record<string, unknown>
}

export interface UsoDoPrefixo {
  prefixo: string
  bytes: number
  objetos: number
  classes: Record<string, number>
  truncado: boolean
}

export interface InspecaoDoBucket {
  bucket: string
  regiao: string
  versionamento: string
  lifecycle: { id: string; prefixo: string; ativa: boolean; resumo: string }[]
  cors: string[]
  prefixos: UsoDoPrefixo[]
  multiparts_pendentes: { key: string; iniciado: string }[]
  credencial_expira?: string
  erros?: Record<string, string>
}

export interface FilaTecnica {
  papel: string
  nome: string
  visiveis: number
  em_voo: number
  atrasadas: number
  erro?: string
}

export interface Tecnico {
  pulso: EstadoNuvem & {
    conexoes: number
    certificado_vence?: string
    certificado_cn?: string
    eventos_pendentes: number
    ultima_reconciliacao?: string
    worker_visto?: string
  }
  nuvem: {
    indisponivel: boolean
    motivo?: string
    bucket?: InspecaoDoBucket
    filas: FilaTecnica[]
    em: string
  }
}

export interface CustoDaNuvem {
  mes: string
  moeda: string
  total: number
  por_servico: { servico: string; valor: number }[]
  por_dia: { dia: string; valor: number }[]
  previsao?: number
  limite?: number
  em: string
  erros?: Record<string, string>
}

export interface RespostaCusto {
  indisponivel: boolean
  motivo?: string
  custo?: CustoDaNuvem
  do_cache?: boolean
}

export interface Armazenamento {
  por_localizacao: { localizacao: string; bytes: number; arquivos: number }[]
  por_biblioteca: { id: number; nome: string; kind: 'movie' | 'tv' | 'music' | 'photo'; bytes: number; bytes_nuvem: number }[]
  derivados: { arquivos: number; total: number }
  disco?: { livre: number; total: number; reserva: number }
}

export const api = {
  me: () => request<User>('/api/auth/me'),
  login: (username: string, password: string, remember = true) =>
    request<User>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password, remember }),
    }),
  logout: () => request<{ ok: boolean }>('/api/auth/logout', { method: 'POST' }),
  changePassword: (current: string, next: string) =>
    request<{ ok: boolean }>('/api/auth/password', {
      method: 'POST',
      body: JSON.stringify({ current, new: next }),
    }),
  approveDevice: (userCode: string) =>
    request<{ ok: boolean; device_name: string }>('/api/auth/device/approve', {
      method: 'POST',
      body: JSON.stringify({ user_code: userCode }),
    }),

  home: () => request<HomeResponse>('/api/home'),
  libraries: () => request<Library[]>('/api/libraries'),
  colecoes: () => request<Colecao[]>('/api/colecoes'),
  colecao: (id: number) =>
    request<{ colecao: Colecao; itens: TitleCard[] }>(`/api/colecoes/${id}`),
  criarColecao: (nome: string) =>
    request<Colecao>('/api/colecoes', { method: 'POST', body: JSON.stringify({ nome }) }),
  renomearColecao: (id: number, nome: string) =>
    request<{ ok: boolean }>(`/api/colecoes/${id}`, { method: 'PUT', body: JSON.stringify({ nome }) }),
  apagarColecao: (id: number) =>
    request<{ ok: boolean }>(`/api/colecoes/${id}`, { method: 'DELETE' }),
  adicionarNaColecao: (id: number, titleId: number) =>
    request<{ ok: boolean }>(`/api/colecoes/${id}/itens/${titleId}`, { method: 'POST' }),
  removerDaColecao: (id: number, titleId: number) =>
    request<{ ok: boolean }>(`/api/colecoes/${id}/itens/${titleId}`, { method: 'DELETE' }),
  colecoesDoTitulo: (titleId: number) =>
    request<{ colecoes: number[] }>(`/api/titles/${titleId}/colecoes`),

  artistas: () => request<ArtistaCard[]>('/api/artistas'),
  artista: (nome: string) => request<ArtistaDetalhe>(`/api/artistas/${encodeURIComponent(nome)}`),
  titles: (params: TitleQuery = {}) => request<TitlesPage>(`/api/titles${query(params)}`),
  title: (id: number) => request<TitleDetail>(`/api/titles/${id}`),
  file: (fileId: number) => request<PlaybackInfo>(`/api/files/${fileId}`),

  /** O que tocar e em que estado está o preparo. Não gasta CPU: só consulta. */
  playback: (fileId: number, audio?: number) =>
    request<PlaybackPlan>(`/api/files/${fileId}/playback${planoQuery(audio)}`),
  /** Faixas de áudio e legendas disponíveis. */
  faixas: (fileId: number) => request<Faixas>(`/api/files/${fileId}/faixas`),
  /** Manda o servidor preparar o arquivo (idempotente: pedidos repetidos
   *  compartilham um único ffmpeg). */
  prepare: (fileId: number, audio?: number) =>
    request<PreparoProgresso>(`/api/files/${fileId}/prepare${planoQuery(audio)}`, { method: 'POST' }),
  prepareEventsUrl: (fileId: number, audio?: number) =>
    `/api/files/${fileId}/prepare/events${planoQuery(audio)}`,
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

  metrics: () => request<MetricsSnapshot>('/api/metrics/status'),
  metricsEventsUrl: () => '/api/metrics/events',

  modo: () => request<EstadoNuvem>('/api/modo'),
  /** "local" é o kill switch: corta a nuvem na hora. */
  setModo: (modo: 'local' | 'hibrido') =>
    request<EstadoNuvem>('/api/modo', { method: 'PUT', body: JSON.stringify({ modo }) }),
  modoEventsUrl: () => '/api/modo/events',

  sincronizacao: () => request<EstadoSincronizacao>('/api/sincronizacao'),
  armazenamento: () => request<Armazenamento>('/api/armazenamento'),
  tecnico: (atualizar = false) => request<Tecnico>(`/api/tecnico${atualizar ? '?atualizar=1' : ''}`),
  custo: (atualizar = false) => request<RespostaCusto>(`/api/tecnico/custo${atualizar ? '?atualizar=1' : ''}`),
  diario: (f: { tipo?: string; upload?: number; antes?: number; limite?: number } = {}) =>
    request<NotaDoDiario[]>(`/api/tecnico/diario${query(f)}`),
  /** Fogo e esquece: o diário nunca pode atrapalhar um envio. */
  anotarEnvio: (id: number, nota: { tipo: 'parte' | 'pausa' | 'retomada' | 'erro'; n?: number; tamanho?: number; ms?: number; etag?: string; erro?: string }) =>
    void fetch(`/api/uploads/${id}/diario`, {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(nota),
      keepalive: true,
    }).catch(() => {}),
  reconciliar: () => request<{ iniciado: boolean }>('/api/sincronizacao', { method: 'POST' }),
  acaoDeNuvem: (fileId: number, acao: AcaoDeNuvem) =>
    request<{ na_fila: boolean }>(`/api/files/${fileId}/nuvem/${acao}`, { method: 'POST' }),
  setEspelhada: (libId: number, espelhada: boolean) =>
    request<{ espelhada: boolean }>(`/api/libraries/${libId}/espelhada`, {
      method: 'PUT',
      body: JSON.stringify({ espelhada }),
    }),

  uploads: () => request<Upload[]>('/api/uploads'),
  criarUpload: (libraryId: number, nome: string, tamanho: number, contentType: string) =>
    request<{ upload: Upload; partes: number }>('/api/uploads', {
      method: 'POST',
      body: JSON.stringify({ library_id: libraryId, nome, tamanho, content_type: contentType }),
    }),
  urlsDoUpload: (id: number, partes: number[]) =>
    request<{ urls: Record<string, string> }>(`/api/uploads/${id}/urls`, {
      method: 'POST',
      body: JSON.stringify({ partes }),
    }),
  partesDoUpload: (id: number) =>
    request<{ upload: Upload; enviadas: ParteEnviada[] }>(`/api/uploads/${id}/partes`),
  concluirUpload: (id: number, partes: ParteEnviada[]) =>
    request<{ file_id: number }>(`/api/uploads/${id}/concluir`, {
      method: 'POST',
      body: JSON.stringify({ partes }),
    }),
  abortarUpload: (id: number) => request<void>(`/api/uploads/${id}`, { method: 'DELETE' }),

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

/**
 * O que ESTE navegador sabe decodificar, perguntado a ele em vez de deduzido do
 * User-Agent. Safari toca HEVC por hardware; sem essa negociação, o servidor
 * recodificaria de graça e queimaria CPU do Mac.
 */
function capacidades(): string[] {
  const suportado = (tipo: string) =>
    typeof MediaSource !== 'undefined' && MediaSource.isTypeSupported
      ? MediaSource.isTypeSupported(tipo)
      : document.createElement('video').canPlayType(tipo) !== ''

  const lista: string[] = []
  if (suportado('video/mp4; codecs="hvc1.1.6.L93.B0"')) lista.push('hevc')
  if (suportado('video/webm; codecs="vp9"')) lista.push('vp9')
  if (suportado('video/mp4; codecs="av01.0.05M.08"')) lista.push('av1')
  return lista
}

/**
 * Os parâmetros que participam da escolha da receita no servidor — e portanto
 * da chave do cache de preparo. Precisam ser IDÊNTICOS entre consultar o plano,
 * pedir o preparo, acompanhar o progresso e buscar o arquivo pronto: divergir
 * em um deles faria o player esperar por um preparo e pedir outro.
 */
function planoQuery(audio?: number): string {
  const q = new URLSearchParams()
  const caps = capacidades()
  if (caps.length > 0) q.set('can', caps.join(','))
  if (audio !== undefined) q.set('audio', String(audio))
  const s = q.toString()
  return s ? `?${s}` : ''
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
