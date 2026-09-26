// Armazenamento (canvas "Armazenamento · Mac e nuvem"): o que mora no Mac, o
// que mora na nuvem e quanto isso custa. Só lê: o acervo vem do banco, o
// bucket da tela técnica (em cache de 1 min) e o disco do próprio Mac.
import { useId } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import { useEnvios } from '../lib/envios'
import { useModoNuvem } from '../lib/nuvem'
import { humanSize } from '../lib/format'
import { EmptyState, Spinner } from '../components/states'
import { FilmIcon, MusicIcon, NuvemIcon, PhotoIcon, TvIcon } from '../components/icons'

// Preço de referência do S3 em us-east-1 (Intelligent-Tiering, camada
// frequente); o que desce de camada fica mais barato, então é um teto.
const USD_POR_GB = 0.023
const CUMULO = 'M70 170 C30 170 20 130 50 118 C44 84 84 66 110 82 C120 46 170 36 196 64 C214 30 280 30 292 78 C330 70 364 96 352 130 C380 140 376 172 344 172 Z'
const ONDA = 'M0 0 C25 -8 75 -8 100 0 C125 8 175 8 200 0 C225 -8 275 -8 300 0 C325 8 375 8 400 0 C425 -8 475 -8 500 0 C525 8 575 8 600 0 L600 300 L0 300 Z'

const iconeDaBiblioteca = { movie: FilmIcon, tv: TvIcon, music: MusicIcon, photo: PhotoIcon }

function Card({ titulo, descricao, children, className = '' }: { titulo: string; descricao: string; children: React.ReactNode; className?: string }) {
  return (
    <section className={`flex flex-col rounded-xl border border-line bg-surface p-5 ${className}`}>
      <h2 className="text-sm font-semibold">{titulo}</h2>
      <p className="mt-1 text-xs leading-relaxed text-muted">{descricao}</p>
      <div className="mt-4 flex flex-1 flex-col">{children}</div>
    </section>
  )
}

/** A nuvem com o nível do que já está no bucket, ondulando. */
function NuvemCheia({ nivel }: { nivel: number }) {
  const id = useId().replace(/:/g, '')
  const n = Math.max(0.04, Math.min(1, nivel))
  // O líquido ocupa a faixa y = 172 (vazio) … 40 (cheio) do cúmulo.
  const topo = 172 - 132 * n
  return (
    <svg viewBox="0 0 400 200" className="h-auto w-full max-w-[260px]" aria-hidden="true">
      <defs>
        <clipPath id={`arm-${id}`}>
          <path d={CUMULO} />
        </clipPath>
      </defs>
      <path d={CUMULO} fill="var(--bg)" />
      <g clipPath={`url(#arm-${id})`}>
        <g className="arm-sobe" style={{ transform: `translateY(${topo.toFixed(1)}px)` }}>
          <path className="arm-onda-2" d={ONDA} fill="var(--accent)" fillOpacity={0.22} />
          <path className="arm-onda" d={ONDA} fill="var(--accent)" fillOpacity={0.35} transform="translate(0 4)" />
        </g>
      </g>
      <path d={CUMULO} fill="none" stroke="var(--ink)" strokeOpacity={0.75} strokeWidth={3} strokeLinejoin="round" />
    </svg>
  )
}

export function Armazenamento() {
  const { data: user } = useQuery({ queryKey: ['me'], queryFn: api.me })
  const modo = useModoNuvem()
  const { lote } = useEnvios()
  const { data, isLoading } = useQuery({ queryKey: ['armazenamento'], queryFn: api.armazenamento, enabled: !!user?.is_admin, refetchInterval: 30000 })
  const { data: tec } = useQuery({ queryKey: ['tecnico'], queryFn: () => api.tecnico(), enabled: !!user?.is_admin, refetchInterval: 60000 })

  if (user && !user.is_admin) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-6 sm:px-6">
        <EmptyState title="Armazenamento" description="Visível apenas para quem mantém o NAS." />
      </div>
    )
  }
  if (isLoading || !data) {
    return (
      <div className="flex justify-center py-20">
        <Spinner />
      </div>
    )
  }

  const por = (loc: string) => data.por_localizacao.find((l) => l.localizacao === loc) ?? { bytes: 0, arquivos: 0 }
  const soMac = por('local').bytes + por('enviando').bytes
  const ambos = por('ambos').bytes
  const soNuvem = por('nuvem').bytes + por('baixando').bytes
  const acervo = soMac + ambos + soNuvem
  const pct = (v: number) => (acervo > 0 ? (v / acervo) * 100 : 0)

  // Bucket: o raio-X da tela técnica, quando o híbrido está ligado; senão a
  // soma do catálogo (sem os derivados).
  const bucket = tec?.nuvem.bucket
  const prefixos = bucket?.prefixos ?? []
  const bytesBucket = bucket ? prefixos.reduce((n, p) => n + p.bytes, 0) : ambos + soNuvem
  const derivados = prefixos.find((p) => p.prefixo === 'derivados/')
  const custo = (bytesBucket / 1e9) * USD_POR_GB
  const nivel = acervo > 0 ? (ambos + soNuvem) / acervo : 0

  // Classes do Intelligent-Tiering, pela contagem de objetos.
  const classes: Record<string, number> = {}
  for (const p of prefixos) for (const [c, n] of Object.entries(p.classes)) classes[c || 'STANDARD'] = (classes[c || 'STANDARD'] ?? 0) + n
  const totalObjetos = Object.values(classes).reduce((a, b) => a + b, 0)
  const camadas = [
    { nome: 'Frequente', n: (classes.STANDARD ?? 0) + (classes.INTELLIGENT_TIERING ?? 0), cor: 'bg-accent' },
    { nome: 'Infrequente', n: (classes.STANDARD_IA ?? 0) + (classes.ONEZONE_IA ?? 0), cor: 'bg-accent/60' },
    { nome: 'Arquivo', n: (classes.GLACIER_IR ?? 0) + (classes.GLACIER ?? 0) + (classes.DEEP_ARCHIVE ?? 0), cor: 'bg-muted/50' },
  ]

  const disco = data.disco
  const usado = disco ? disco.total - disco.livre : 0
  const hibrido = modo?.modo === 'hibrido'

  return (
    <div className="mx-auto max-w-6xl space-y-6 px-4 py-6 sm:px-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">Armazenamento</h1>
          <p className="mt-1 text-sm text-muted">O que está no Mac, o que está na nuvem e quanto isso custa.</p>
        </div>
        <span
          className={`inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs font-semibold ${hibrido ? 'border-accent/40 bg-accent/10 text-accent' : 'border-line text-muted'}`}
        >
          <NuvemIcon estado={hibrido ? 'hibrido' : 'local'} /> {hibrido ? 'Híbrido' : 'Local'}
        </span>
      </div>

      <div className={`grid gap-6 ${disco ? 'lg:grid-cols-2' : ''}`}>
        <Card titulo="No bucket" descricao="Tudo o que o S3 guarda do Ozymandias, derivados incluídos.">
          <div className="flex flex-1 flex-wrap items-center justify-center gap-8">
            <NuvemCheia nivel={nivel} />
            <div className="flex flex-col gap-5">
              <div>
                <p className="text-3xl font-semibold tracking-tight">{humanSize(bytesBucket) || '0 B'}</p>
                <p className="text-xs text-muted">{bucket ? 'originais e derivados' : 'originais (sem o raio-X do bucket)'}</p>
              </div>
              <div>
                <p className="text-3xl font-semibold tracking-tight">
                  {custo.toLocaleString('pt-BR', { style: 'currency', currency: 'USD', maximumFractionDigits: custo < 1 ? 3 : 2 })}
                  <span className="text-lg text-muted">/mês</span>
                </p>
                <p className="max-w-[14rem] text-xs text-muted">estimativa pela tabela da AWS; o custo real está na tela Técnico</p>
              </div>
            </div>
          </div>
        </Card>

        {disco && (
          <Card titulo="Disco do Mac" descricao="O que sobra no SSD. Fixar respeita a reserva, e liberar espaço devolve aqui.">
            <p className="text-3xl font-semibold tracking-tight">{humanSize(disco.livre)} livres</p>
            <p className="text-xs text-muted">de {humanSize(disco.total)}</p>
            <div className="relative mt-5 h-2.5 rounded-full bg-elev">
              <div className="arm-barra h-full rounded-full bg-muted/60" style={{ width: `${disco.total ? (usado / disco.total) * 100 : 0}%` }} />
              {disco.reserva > 0 && (
                <div
                  className="absolute -top-1.5 h-5.5 w-0.5 rounded bg-accent"
                  style={{ left: `${Math.min(100, ((disco.total - disco.reserva) / disco.total) * 100)}%` }}
                  title={`reserva de ${humanSize(disco.reserva)}`}
                />
              )}
            </div>
            <div className="mt-2 flex justify-between text-xs">
              <span className="text-muted">usado · {humanSize(usado)}</span>
              {disco.reserva > 0 && <span className="text-accent">reserva de {humanSize(disco.reserva)}</span>}
            </div>
          </Card>
        )}
      </div>

      <Card titulo="Onde mora o acervo" descricao="Cada arquivo tem uma localização. A barra é o tamanho somado de cada uma.">
        <div className="flex h-3 gap-0.5 overflow-hidden rounded-full bg-elev" role="img" aria-label={`Só no Mac ${humanSize(soMac)}, no Mac e na nuvem ${humanSize(ambos)}, só na nuvem ${humanSize(soNuvem)}`}>
          <div className="arm-barra h-full bg-muted/60" style={{ width: `${pct(soMac)}%` }} />
          <div className="arm-barra h-full bg-accent" style={{ width: `${pct(ambos)}%` }} />
          <div className="arm-barra h-full bg-ink/80" style={{ width: `${pct(soNuvem)}%` }} />
        </div>
        <div className="mt-3 flex flex-wrap gap-x-6 gap-y-2 text-xs text-muted">
          {[
            ['bg-muted/60', 'Só no Mac', soMac, por('local').arquivos],
            ['bg-accent', 'No Mac e na nuvem', ambos, por('ambos').arquivos],
            ['bg-ink/80', 'Só na nuvem', soNuvem, por('nuvem').arquivos],
          ].map(([cor, nome, bytes, n]) => (
            <span key={nome as string} className="inline-flex items-center gap-2">
              <span className={`h-2.5 w-2.5 rounded-sm ${cor}`} />
              {nome}
              <span className="font-mono text-ink">{humanSize(bytes as number) || '0 B'}</span>
              <span className="font-mono">· {n as number}</span>
            </span>
          ))}
        </div>
        {lote?.ativo && (
          <div className="mt-4 flex items-center gap-3 overflow-hidden rounded-lg border border-line bg-bg px-4 py-3 text-sm">
            <NuvemIcon estado="enviando" className="shrink-0 text-accent" />
            <span className="font-medium">
              Enviando {lote.linhas.length} {lote.linhas.length === 1 ? 'arquivo' : 'arquivos'}
            </span>
            <span className="font-mono text-xs text-muted">
              {humanSize(lote.feito)} de {humanSize(lote.total)}
            </span>
            <span className="ml-auto h-1 w-40 overflow-hidden rounded bg-elev">
              <span className="arm-barra block h-full bg-accent" style={{ width: `${Math.round(lote.progresso * 100)}%` }} />
            </span>
          </div>
        )}
      </Card>

      <div className="grid gap-6 lg:grid-cols-3">
        <Card titulo="Por biblioteca" descricao="Quanto de cada biblioteca já está no bucket.">
          <ul className="flex flex-col gap-3">
            {data.por_biblioteca.map((b) => {
              const Icone = iconeDaBiblioteca[b.kind] ?? FilmIcon
              const fracao = b.bytes > 0 ? b.bytes_nuvem / b.bytes : 0
              return (
                <li key={b.id} className="grid grid-cols-[1.25rem_minmax(0,1fr)_4rem_5rem] items-center gap-3 text-sm">
                  <Icone className="text-muted" />
                  <span className="truncate">{b.nome}</span>
                  <span className="h-1.5 overflow-hidden rounded bg-elev">
                    <span className="arm-barra block h-full bg-accent" style={{ width: `${Math.max(fracao > 0 ? 4 : 0, fracao * 100)}%` }} />
                  </span>
                  <span className="text-right font-mono text-xs text-muted" title={`${humanSize(b.bytes_nuvem)} de ${humanSize(b.bytes)}`}>
                    {humanSize(b.bytes_nuvem) || '0 B'}
                  </span>
                </li>
              )
            })}
          </ul>
        </Card>

        <Card titulo="Classes de armazenamento" descricao="O Intelligent-Tiering desce o que ninguém toca há 30 e 90 dias, sem custo para buscar de volta.">
          {totalObjetos === 0 ? (
            <p className="text-sm text-muted">{hibrido ? 'Consultando o bucket…' : 'Ligue o híbrido para ver as classes.'}</p>
          ) : (
            <>
              <div className="flex h-2.5 gap-0.5 overflow-hidden rounded-full bg-elev">
                {camadas.map((c) => (
                  <div key={c.nome} className={`arm-barra h-full ${c.cor}`} style={{ width: `${(c.n / totalObjetos) * 100}%` }} />
                ))}
              </div>
              <div className="mt-3 flex flex-wrap gap-x-4 gap-y-2 text-xs text-muted">
                {camadas.map((c) => (
                  <span key={c.nome} className="inline-flex items-center gap-2">
                    <span className={`h-2.5 w-2.5 rounded-sm ${c.cor}`} />
                    {c.nome} <span className="font-mono">{c.n}</span>
                  </span>
                ))}
              </div>
            </>
          )}
        </Card>

        <Card titulo="Derivados do worker" descricao="MP4 compatível, miniaturas e legendas gerados na nuvem.">
          <div className="flex flex-1 items-center gap-4">
            <NuvemIcon estado="processando" className="h-9 w-9 shrink-0 text-accent" />
            <div>
              <p className="text-3xl font-semibold tracking-tight">{derivados ? humanSize(derivados.bytes) : `${data.derivados.total}`}</p>
              <p className="text-xs text-muted">
                {derivados ? `${data.derivados.total} derivados · ` : 'derivados · '}em {data.derivados.arquivos} {data.derivados.arquivos === 1 ? 'arquivo' : 'arquivos'}
              </p>
            </div>
          </div>
        </Card>
      </div>
    </div>
  )
}
