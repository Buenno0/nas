import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, downloadUrl, soNaNuvem, type AcaoDeNuvem, type FileInfo, type TitleDetail } from '../lib/api'
import { useDisponibilidade, useHibrido, useModoNuvem } from '../lib/nuvem'
import { clockTime, gradientFor, humanDuration, humanSize, kindLabel } from '../lib/format'
import { CloudOffIcon, DownloadIcon, HeartIcon, NuvemIcon, PauseIcon, PlayIcon } from '../components/icons'
import { ErrorState, Spinner } from '../components/states'
import { PhotoGrid } from '../components/PhotoGrid'
import { ColecaoPicker } from '../components/ColecaoPicker'
import { usePlayer, type Track } from '../lib/player'

export function Title() {
  const { id } = useParams()
  const titleId = Number(id)
  const queryClient = useQueryClient()
  const player = usePlayer()
  const [matching, setMatching] = useState(false)

  // Enquanto algum arquivo deste título estiver indo ou vindo da nuvem, a
  // página volta a perguntar ao servidor: sem isso, "enviando…" só virava
  // "no Mac e na nuvem" depois de um F5. As tarefas vêm do mesmo cache que o
  // cartão de envios mantém atualizado.
  const { data: sinc } = useQuery({ queryKey: ['sincronizacao'], queryFn: api.sincronizacao, enabled: false })
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['title', titleId],
    queryFn: () => api.title(titleId),
    refetchInterval: (q) => {
      const d = q.state.data
      if (!d) return false
      const arquivos = [...d.files, ...(d.seasons ?? []).flatMap((t) => t.episodes)]
      const ids = new Set(arquivos.map((f) => f.id))
      const emTransito =
        arquivos.some((f) => f.localizacao === 'enviando' || f.localizacao === 'baixando') ||
        (sinc?.tarefas ?? []).some((t) => ids.has(t.file_id) && t.estado !== 'erro')
      return emTransito ? 2000 : false
    },
  })

  const favorite = useMutation({
    mutationFn: (on: boolean) => api.setFavorite(titleId, on),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['title', titleId] })
      void queryClient.invalidateQueries({ queryKey: ['home'] })
    },
  })

  if (isLoading) return <Spinner />
  if (isError || !data) return <ErrorState error={error} retry={() => void refetch()} />

  // Quando o mesmo título tem o original e uma versão convertida (.mkv e
  // .mp4 lado a lado), o botão de play tem que abrir a que o navegador toca.
  const playable = data.files.filter(playsInBrowser)
  const candidates = playable.length > 0 ? playable : data.files

  // Retomar = o que está pela metade; senão o primeiro ainda não assistido;
  // senão o começo de tudo (série toda vista).
  const resume =
    candidates.find((f) => (f.position ?? 0) > 0 && !f.finished) ??
    candidates.find((f) => !f.finished) ??
    candidates[0]
  const resumeAt = resume && !resume.finished ? (resume.position ?? 0) : 0
  const totalDuration = data.files.reduce((sum, f) => sum + (f.duration ?? 0), 0)

  return (
    <article>
      <header className="relative">
        <div
          className="absolute inset-0"
          style={
            data.backdrop_url
              ? {
                  backgroundImage: `url(${data.backdrop_url})`,
                  backgroundSize: 'cover',
                  backgroundPosition: 'center',
                }
              : { background: gradientFor(data.name) }
          }
        />
        <div className="absolute inset-0 bg-gradient-to-t from-bg via-bg/80 to-bg/30" />

        <div className="relative flex flex-col gap-5 px-4 pt-10 pb-6 sm:flex-row sm:items-end sm:px-6 sm:pt-16">
          <div className="w-32 shrink-0 overflow-hidden rounded-xl ring-1 ring-line sm:w-44">
            <div className="aspect-[2/3]">
              {data.poster_url ? (
                <img src={data.poster_url} alt="" className="h-full w-full object-cover" />
              ) : (
                <div className="h-full w-full" style={{ background: gradientFor(data.name) }} />
              )}
            </div>
          </div>

          <div className="min-w-0 flex-1">
            <p className="rotulo">
              {kindLabel[data.kind] ?? data.kind} · {data.library}
            </p>
            <h1 className="mt-1 text-2xl font-bold tracking-tight sm:text-4xl">{data.name}</h1>
            <p className="mt-1.5 text-sm text-muted">
              {[
                data.year || '',
                data.artist || '',
                data.files.length > 1
                  ? `${data.files.length} arquivos`
                  : humanDuration(totalDuration),
                data.rating ? `★ ${data.rating.toFixed(1)}` : '',
              ]
                .filter(Boolean)
                .join(' · ')}
            </p>

            {data.overview && (
              <p className="mt-4 max-w-2xl text-sm leading-relaxed text-muted">{data.overview}</p>
            )}

            <div className="mt-5 flex flex-wrap items-center gap-2">
              {/* Álbum toca na barra de música; foto abre pela galeria; o
                  resto vai para o player de vídeo. */}
              {data.kind === 'album' && data.files.length > 0 && (
                <button
                  type="button"
                  onClick={() => player.play(albumTracks(data), 0)}
                  className="inline-flex items-center gap-2 rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-accent-ink transition hover:opacity-90"
                >
                  <PlayIcon /> Tocar álbum
                </button>
              )}

              {data.kind !== 'album' && data.kind !== 'photos' && resume && (
                <Link
                  to={`/watch/${resume.id}`}
                  className="inline-flex items-center gap-2 rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-accent-ink transition hover:opacity-90"
                >
                  <PlayIcon />
                  {resumeAt > 0 ? `Continuar de ${clockTime(resumeAt)}` : 'Assistir'}
                </Link>
              )}
              <button
                type="button"
                onClick={() => favorite.mutate(!data.favorite)}
                aria-pressed={data.favorite}
                className={[
                  'inline-flex items-center gap-2 rounded-lg border px-3.5 py-2.5 text-sm font-medium transition',
                  data.favorite
                    ? 'border-accent bg-accent/10 text-accent'
                    : 'border-line bg-surface text-muted hover:text-ink',
                ].join(' ')}
              >
                <HeartIcon filled={data.favorite} />
                {data.favorite ? 'Nos favoritos' : 'Favoritar'}
              </button>

              <ColecaoPicker titleId={titleId} />

              {(data.kind === 'movie' || data.kind === 'tv') && (
                <button
                  type="button"
                  onClick={() => setMatching(true)}
                  className="inline-flex items-center gap-2 rounded-lg border border-line bg-surface px-3.5 py-2.5 text-sm font-medium text-muted transition hover:text-ink"
                >
                  {data.meta_state === 'matched' || data.meta_state === 'manual'
                    ? 'Trocar capa'
                    : 'Buscar capa'}
                </button>
              )}
            </div>
          </div>
        </div>
      </header>

      <section className="px-4 pb-10 sm:px-6">
        {data.kind === 'photos' ? (
          <PhotoGrid photos={data.files} />
        ) : data.kind === 'album' ? (
          <TrackList detail={data} />
        ) : data.seasons && data.seasons.length > 0 ? (
          <SeasonList detail={data} />
        ) : (
          <FileList files={data.files} />
        )}
      </section>

      {matching && (
        <MatchDialog titleId={titleId} initialQuery={data.name} onClose={() => setMatching(false)} />
      )}
    </article>
  )
}

/** Correção manual do match: mostra os candidatos do TMDB e aplica o escolhido. */
function MatchDialog({
  titleId,
  initialQuery,
  onClose,
}: {
  titleId: number
  initialQuery: string
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const [term, setTerm] = useState(initialQuery)
  const [submitted, setSubmitted] = useState(initialQuery)

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['matches', titleId, submitted],
    queryFn: () => api.matches(titleId, submitted),
    retry: false,
  })

  const apply = useMutation({
    mutationFn: (tmdbId: number) => api.applyMatch(titleId, tmdbId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['title', titleId] })
      void queryClient.invalidateQueries({ queryKey: ['home'] })
      void queryClient.invalidateQueries({ queryKey: ['titles'] })
      onClose()
    },
  })

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Corrigir metadados"
      className="fixed inset-0 z-40 grid place-items-center bg-black/60 p-4 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="flex max-h-[80vh] w-full max-w-lg flex-col overflow-hidden rounded-2xl border border-line bg-surface"
      >
        <div className="border-b border-line p-4">
          <h2 className="mb-3 text-sm font-semibold">Escolher no TMDB</h2>
          <form
            onSubmit={(e) => {
              e.preventDefault()
              setSubmitted(term)
            }}
            className="flex gap-2"
          >
            <input
              value={term}
              onChange={(e) => setTerm(e.target.value)}
              autoFocus
              className="flex-1 rounded-lg border border-line bg-bg px-3 py-2 text-sm outline-none focus:border-accent"
            />
            <button
              type="submit"
              className="rounded-lg border border-line px-3 py-2 text-sm font-medium transition hover:bg-elev"
            >
              Buscar
            </button>
          </form>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto p-2">
          {isLoading && <Spinner label="Consultando o TMDB…" />}
          {isError && (
            <p className="p-4 text-sm text-muted">
              {(error as Error).message}
              <br />
              <span className="text-xs">Confira a chave do TMDB nas configurações.</span>
            </p>
          )}
          {data?.results.length === 0 && (
            <p className="p-4 text-sm text-muted">Nenhum resultado para esse termo.</p>
          )}

          <ul className="flex flex-col gap-1">
            {data?.results.map((candidate) => (
              <li key={candidate.tmdb_id}>
                <button
                  type="button"
                  disabled={apply.isPending}
                  onClick={() => apply.mutate(candidate.tmdb_id)}
                  className="flex w-full gap-3 rounded-lg p-2 text-left transition hover:bg-elev disabled:opacity-50"
                >
                  <span className="h-20 w-14 shrink-0 overflow-hidden rounded bg-elev">
                    {candidate.poster && (
                      <img src={candidate.poster} alt="" className="h-full w-full object-cover" />
                    )}
                  </span>
                  <span className="min-w-0">
                    <span className="block text-sm font-medium">
                      {candidate.name} {candidate.year ? `(${candidate.year})` : ''}
                    </span>
                    <span className="line-clamp-3 block text-xs text-muted">
                      {candidate.overview || 'Sem sinopse.'}
                    </span>
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </div>

        <div className="flex justify-end border-t border-line p-3">
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg border border-line px-3 py-1.5 text-sm font-medium transition hover:bg-elev"
          >
            Fechar
          </button>
        </div>
      </div>
    </div>
  )
}

function SeasonList({ detail }: { detail: TitleDetail }) {
  const seasons = detail.seasons ?? []
  const [active, setActive] = useState(seasons[0]?.number ?? 0)
  const current = seasons.find((s) => s.number === active) ?? seasons[0]

  return (
    <>
      <div className="mb-4 flex flex-wrap gap-2">
        {seasons.map((season) => (
          <button
            key={season.number}
            type="button"
            onClick={() => setActive(season.number)}
            className={[
              'rounded-lg border px-3 py-1.5 text-sm font-medium transition',
              season.number === active
                ? 'border-accent bg-accent/10 text-accent'
                : 'border-line bg-surface text-muted hover:text-ink',
            ].join(' ')}
          >
            {season.number > 0 ? `Temporada ${season.number}` : 'Avulsos'}
          </button>
        ))}
      </div>
      <FileList files={current?.episodes ?? []} />
    </>
  )
}

// Containers e codecs que o navegador abre. Serve para escolher, entre os
// arquivos de um mesmo título, qual o play deve abrir.
const playableExts = ['.mp4', '.m4v', '.webm', '.mov']
const unplayableCodecs = ['hevc', 'h265', 'vc1', 'mpeg2video', 'ac3', 'eac3', 'dts', 'truehd']

function playsInBrowser(file: FileInfo): boolean {
  if (!playableExts.includes(file.ext.toLowerCase())) return false
  return ![file.vcodec, file.acodec]
    .filter(Boolean)
    .some((codec) => unplayableCodecs.includes(codec!.toLowerCase()))
}

/** Converte os arquivos do álbum na fila do player. */
function albumTracks(detail: TitleDetail): Track[] {
  return detail.files.map((file) => ({
    id: file.id,
    name: file.name,
    artist: detail.artist,
    album: detail.name,
    poster: detail.poster_url,
    duration: file.duration,
  }))
}

/** Faixas de um álbum: clicar toca no mini player e enfileira o resto. */
function TrackList({ detail }: { detail: TitleDetail }) {
  const player = usePlayer()
  const tracks = albumTracks(detail)

  return (
    <ul className="divide-y divide-line overflow-hidden rounded-xl border border-line bg-surface">
      {detail.files.map((file, i) => {
        const isCurrent = player.current?.id === file.id
        return (
          <li key={file.id}>
            <button
              type="button"
              onClick={() => player.play(tracks, i)}
              className="flex w-full items-center gap-3 px-3 py-3 text-left transition hover:bg-elev sm:px-4"
            >
              <span
                className={[
                  'grid h-9 w-9 shrink-0 place-items-center rounded-lg text-xs font-medium',
                  isCurrent ? 'bg-accent text-accent-ink' : 'bg-elev text-muted',
                ].join(' ')}
              >
                {isCurrent && player.playing ? <PauseIcon /> : (file.track ?? i + 1)}
              </span>
              <span className="min-w-0 flex-1">
                <span className={['block truncate text-sm', isCurrent ? 'text-accent' : ''].join(' ')}>
                  {file.name}
                </span>
                <span className="block text-xs text-muted">
                  {[detail.artist, humanDuration(file.duration)].filter(Boolean).join(' · ')}
                </span>
              </span>
            </button>
          </li>
        )
      })}
    </ul>
  )
}

function FileList({ files }: { files: FileInfo[] }) {
  const disp = useDisponibilidade()
  const hibrido = disp.hibrido
  if (files.length === 0) return null

  return (
    <ul className="divide-y divide-line overflow-hidden rounded-xl border border-line bg-surface">
      {files.map((file) => {
        const percent = file.duration > 0 ? ((file.position ?? 0) / file.duration) * 100 : 0
        const label =
          file.season || file.episode
            ? `${file.episode ? `E${String(file.episode).padStart(2, '0')} · ` : ''}${file.name}`
            : file.name

        return (
          <li key={file.id} className="relative">
            <div className="flex items-center gap-3 px-3 py-3 sm:px-4">
              {!disp.tocaArquivo(file.localizacao) ? (
                <span
                  title={`${disp.ondeMora}, indisponível aqui`}
                  className="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-elev text-muted"
                >
                  <CloudOffIcon />
                </span>
              ) : (
                <Link
                  to={`/watch/${file.id}`}
                  className="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-elev text-ink transition hover:bg-accent hover:text-accent-ink"
                  aria-label={`Reproduzir ${label}`}
                >
                  <PlayIcon />
                </Link>
              )}

              <div className="min-w-0 flex-1">
                <p className="line-clamp-1 text-sm font-medium">{label}</p>
                <p className="text-xs text-muted">
                  {[
                    humanDuration(file.duration),
                    file.height ? `${file.height}p` : '',
                    file.vcodec || file.acodec,
                    humanSize(file.size),
                    file.finished ? 'assistido' : '',
                    disp.naNuvem
                      ? disp.tocaArquivo(file.localizacao)
                        ? ''
                        : 'no Mac, indisponível'
                      : soNaNuvem(file.localizacao)
                        ? hibrido
                          ? 'na nuvem'
                          : 'na nuvem, indisponível'
                        : file.localizacao === 'ambos'
                          ? 'no Mac e na nuvem'
                          : '',
                  ]
                    .filter(Boolean)
                    .join(' · ')}
                  {file.media_type === 'video' && !playsInBrowser(file) && (
                    <span className="ml-1.5 text-warn">· não toca no navegador</span>
                  )}
                </p>
              </div>

              <AcoesDeNuvem file={file} />

              <a
                href={downloadUrl(file.id)}
                className="grid h-9 w-9 shrink-0 place-items-center rounded-lg text-muted transition hover:bg-elev hover:text-ink"
                aria-label={`Baixar ${label}`}
              >
                <DownloadIcon />
              </a>
            </div>

            {percent > 0 && (
              <div className="absolute inset-x-0 bottom-0 h-0.5 bg-elev">
                <div className="h-full bg-accent" style={{ width: `${Math.min(100, percent)}%` }} />
              </div>
            )}
          </li>
        )
      })}
    </ul>
  )
}

const confirmacoes: Partial<Record<AcaoDeNuvem, string>> = {
  liberar:
    'Apagar a cópia deste arquivo no Mac? Ela só é apagada depois de o servidor provar que a da nuvem é idêntica. O item continua no catálogo e toca da nuvem no modo híbrido.',
  remover:
    'Apagar a cópia deste arquivo na nuvem? A do Mac continua. O bucket guarda a versão anterior por 30 dias.',
}

/** Onde o arquivo mora e o que dá para fazer com ele. Só o administrador
 *  move arquivos, e só no híbrido: no local, nada fala com a nuvem. */
function AcoesDeNuvem({ file }: { file: FileInfo }) {
  const queryClient = useQueryClient()
  const hibrido = useHibrido()
  const { data: user } = useQuery({ queryKey: ['me'], queryFn: api.me })
  const [pedido, setPedido] = useState<AcaoDeNuvem | null>(null)
  // O pedido vale até o servidor mudar a localização do arquivo: daí em
  // diante quem diz o estado é ela (enviando, baixando, ambos…).
  useEffect(() => setPedido(null), [file.localizacao])
  const acao = useMutation({
    mutationFn: (a: AcaoDeNuvem) => api.acaoDeNuvem(file.id, a),
    onSuccess: (_, a) => {
      setPedido(a)
      // O motor trabalha em segundo plano: as tarefas passam a ser vigiadas
      // pelo cartão de envios, e a página volta a olhar o título.
      void queryClient.invalidateQueries({ queryKey: ['sincronizacao'] })
      window.setTimeout(() => void queryClient.invalidateQueries({ queryKey: ['title'] }), 1500)
    },
  })
  const estado = useModoNuvem()
  if (!user?.is_admin || !hibrido || estado?.papel === 'nuvem') return null

  const loc = file.localizacao ?? 'local'
  const opcoes: { acao: AcaoDeNuvem; rotulo: string; perigo?: boolean }[] =
    loc === 'local'
      ? [{ acao: 'enviar', rotulo: 'Enviar à nuvem' }]
      : loc === 'nuvem'
        ? [{ acao: 'fixar', rotulo: 'Disponível offline' }]
        : loc === 'ambos'
          ? [
              { acao: 'liberar', rotulo: 'Liberar espaço', perigo: true },
              { acao: 'remover', rotulo: 'Tirar da nuvem', perigo: true },
            ]
          : []

  if (opcoes.length === 0 || pedido) {
    const texto = pedido
      ? 'na fila'
      : loc === 'enviando'
        ? 'enviando…'
        : loc === 'baixando'
          ? 'baixando…'
          : ''
    if (!texto) return null
    const icone = loc === 'enviando' ? 'enviando' : loc === 'baixando' ? 'baixando' : 'processando'
    return (
      <span className="flex shrink-0 items-center gap-1.5 text-xs text-muted">
        <NuvemIcon estado={icone} className="text-accent" />
        {texto}
      </span>
    )
  }

  return (
    <div className="hidden shrink-0 items-center gap-1 sm:flex">
      {opcoes.map((o) => (
        <button
          key={o.acao}
          type="button"
          disabled={acao.isPending}
          title={acao.isError ? (acao.error as Error).message : undefined}
          onClick={() => {
            const aviso = confirmacoes[o.acao]
            if (aviso && !window.confirm(aviso)) return
            acao.mutate(o.acao)
          }}
          className={[
            'rounded-md px-2 py-1 text-xs font-medium transition disabled:opacity-50',
            o.perigo ? 'text-muted hover:bg-danger/10 hover:text-danger' : 'text-muted hover:bg-elev hover:text-ink',
          ].join(' ')}
        >
          {o.rotulo}
        </button>
      ))}
      {acao.isError && <span className="text-xs text-danger">falhou</span>}
    </div>
  )
}
