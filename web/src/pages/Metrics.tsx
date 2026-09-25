import { useQuery } from '@tanstack/react-query'
import { api, type RouteSnapshot } from '../lib/api'
import { useMetrics } from '../lib/useMetrics'
import { humanSize } from '../lib/format'
import { EmptyState } from '../components/states'
import { ActivityIcon, ChipIcon, ClockIcon, DiskIcon } from '../components/icons'

/** 93784 → "1d 2h"; 7380 → "2h 03min"; 90 → "1min" */
function uptimeLegivel(segundos: number): string {
  if (segundos < 60) return `${Math.max(0, Math.round(segundos))}s`
  const d = Math.floor(segundos / 86400)
  const h = Math.floor((segundos % 86400) / 3600)
  const m = Math.floor((segundos % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${String(m).padStart(2, '0')}min`
  return `${m}min`
}

/** Milissegundos com precisão útil: 0.23ms não deve virar "0ms". */
function ms(valor: number): string {
  if (valor >= 100) return `${Math.round(valor)}ms`
  if (valor >= 10) return `${valor.toFixed(1)}ms`
  return `${valor.toFixed(2)}ms`
}

export function Metrics() {
  const { data: user } = useQuery({ queryKey: ['me'], queryFn: api.me })
  const { snap, proibido } = useMetrics()

  // A rota HTTP já recusa com 403; esta tela é cortesia, não a proteção.
  if (user && !user.is_admin) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-6 sm:px-6">
        <EmptyState
          title="Painel do administrador"
          description="A telemetria do servidor é visível apenas para quem mantém o NAS."
        />
      </div>
    )
  }

  if (proibido) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-6 sm:px-6">
        <EmptyState
          title="Sem acesso à telemetria"
          description="O servidor recusou o painel para esta conta."
        />
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-4xl space-y-6 px-4 py-6 sm:px-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold tracking-tight sm:text-2xl">
            <ActivityIcon className="text-accent" /> Métricas
          </h1>
          <p className="mt-1 text-sm text-muted">
            Saúde do servidor desde que ele subiu. Nada disso é gravado em disco: reinicie e tudo
            volta a zero.
          </p>
        </div>

        <div className="flex items-center gap-2 text-xs">
          <span
            className={[
              'rounded-full px-2.5 py-1 font-semibold',
              snap?.modo === 'tunnel'
                ? 'bg-accent/15 text-accent'
                : 'bg-ok/15 text-ok',
            ].join(' ')}
          >
            modo {snap?.modo ?? '—'}
          </span>
          <span className="inline-flex items-center gap-1.5 rounded-full border border-line px-2.5 py-1 text-muted">
            <ClockIcon width="1em" height="1em" />
            {snap ? uptimeLegivel(snap.uptime_segundos) : '—'}
          </span>
        </div>
      </div>

      {!snap && (
        <p className="rounded-xl border border-line bg-surface p-5 text-sm text-muted">
          Conectando ao fluxo de telemetria…
        </p>
      )}

      {snap && (
        <>
          <RecursosCard snap={snap} />
          <DiscoCard snap={snap} />
          <TrafegoCard trafego={snap.trafego} />
        </>
      )}
    </div>
  )
}

function Card({
  title,
  icon,
  description,
  children,
}: {
  title: string
  icon?: React.ReactNode
  description?: string
  children: React.ReactNode
}) {
  return (
    <section className="rounded-xl border border-line bg-surface p-5">
      <h2 className="flex items-center gap-2 text-sm font-semibold">
        {icon}
        {title}
      </h2>
      {description && <p className="mt-1 text-xs leading-relaxed text-muted">{description}</p>}
      <div className="mt-4">{children}</div>
    </section>
  )
}

/** Barra de uma cor com valor de 0 a 1. */
function Barra({ fracao, tom = 'bg-accent' }: { fracao: number; tom?: string }) {
  const largura = Math.min(100, Math.max(0, fracao * 100))
  return (
    <div className="h-2 overflow-hidden rounded-full bg-elev">
      <div className={`h-full rounded-full ${tom} transition-[width] duration-500`} style={{ width: `${largura}%` }} />
    </div>
  )
}

function Numero({ label, valor, nota }: { label: string; valor: string; nota?: string }) {
  return (
    <div>
      <p className="rotulo">{label}</p>
      <p className="mt-0.5 text-lg font-semibold tabular-nums">{valor}</p>
      {nota && <p className="text-[11px] text-muted">{nota}</p>}
    </div>
  )
}

function RecursosCard({ snap }: { snap: NonNullable<ReturnType<typeof useMetrics>['snap']> }) {
  const p = snap.processo
  return (
    <Card
      title="Recursos do processo"
      icon={<ChipIcon className="text-muted" />}
      description="Isto mede o processo do NAS — não o ffmpeg. Durante um preparo de vídeo a máquina sua, mas quem trabalha é um processo filho, então a CPU daqui continua baixa."
    >
      <div className="grid grid-cols-2 gap-5 sm:grid-cols-4">
        <Numero
          label="CPU"
          valor={`${p.cpu_percent.toFixed(1)}%`}
          nota={`${p.cpu_nucleos.toFixed(2)} de ${p.nucleos} núcleos`}
        />
        <Numero label="Heap agora" valor={humanSize(p.heap_bytes) || '0 B'} nota="memória Go em uso" />
        <Numero
          label="Pico de RSS"
          valor={humanSize(p.rss_pico_bytes) || '—'}
          nota="máximo desde o boot"
        />
        <Numero label="Goroutines" valor={String(p.goroutines)} nota="tarefas concorrentes" />
      </div>

      <div className="mt-4">
        <Barra
          fracao={p.cpu_percent / 100}
          tom={p.cpu_percent > 80 ? 'bg-danger' : p.cpu_percent > 45 ? 'bg-warn' : 'bg-accent'}
        />
      </div>
    </Card>
  )
}

function DiscoCard({ snap }: { snap: NonNullable<ReturnType<typeof useMetrics>['snap']> }) {
  const { disco_total_bytes: total, disco_livre_bytes: livre } = snap
  const usadoSistema = Math.max(0, total - livre)
  const fracao = (v: number) => (total > 0 ? Math.min(1, Math.max(0, v / total)) : 0)

  // A reserva é desenhada dentro do espaço livre: é livre no disco, mas o NAS
  // se proíbe de usar — é o que impede o SQLite de morrer com SQLITE_FULL.
  const reserva = Math.min(snap.disco_reserva_bytes, livre)

  const cacheFracao =
    snap.cache_limite_bytes > 0 ? snap.cache_usado_bytes / snap.cache_limite_bytes : 0

  return (
    <Card
      title="Disco"
      icon={<DiskIcon className="text-muted" />}
      description="O cache de vídeos preparados divide o volume com o banco. A reserva existe para que encher o cache nunca derrube o login."
    >
      <div className="flex h-3 overflow-hidden rounded-full bg-elev" role="img" aria-label="Ocupação do volume">
        <div className="h-full bg-muted/70" style={{ width: `${fracao(usadoSistema) * 100}%` }} />
        <div className="h-full bg-accent" style={{ width: `${fracao(snap.cache_usado_bytes) * 100}%` }} />
        <div
          className="h-full bg-warn/50"
          style={{ width: `${fracao(reserva) * 100}%` }}
        />
      </div>

      <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-[11px] text-muted">
        <Legenda tom="bg-muted/70" texto={`${humanSize(usadoSistema)} ocupados no volume`} />
        <Legenda tom="bg-accent" texto={`${humanSize(snap.cache_usado_bytes) || '0 B'} de cache`} />
        <Legenda tom="bg-warn/50" texto={`${humanSize(reserva)} de reserva intocável`} />
        <Legenda tom="bg-elev ring-1 ring-line" texto={`${humanSize(livre)} livres`} />
      </div>

      <div className="mt-5 grid grid-cols-2 gap-5 sm:grid-cols-3">
        <Numero
          label="Cache de preparo"
          valor={`${humanSize(snap.cache_usado_bytes) || '0 B'}`}
          nota={`orçamento de ${humanSize(snap.cache_limite_bytes)}`}
        />
        <Numero label="Livre no volume" valor={humanSize(livre)} nota={`de ${humanSize(total)}`} />
        <Numero
          label="Uso do orçamento"
          valor={`${Math.round(cacheFracao * 100)}%`}
          nota="acima de 100% dispara a limpeza"
        />
      </div>

      <div className="mt-3">
        <Barra
          fracao={cacheFracao}
          tom={cacheFracao > 0.9 ? 'bg-warn' : 'bg-accent'}
        />
      </div>
    </Card>
  )
}

function Legenda({ tom, texto }: { tom: string; texto: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className={`inline-block h-2 w-2 rounded-full ${tom}`} />
      {texto}
    </span>
  )
}

function TrafegoCard({ trafego }: { trafego: NonNullable<ReturnType<typeof useMetrics>['snap']>['trafego'] }) {
  const maior = trafego.rotas.reduce((m, r) => Math.max(m, r.total), 0)

  return (
    <Card
      title="Tráfego HTTP"
      icon={<ActivityIcon className="text-muted" />}
      description="Agregado pelo padrão da rota, não pelo caminho: /api/titles/7 e /api/titles/9 são a mesma linha, e GET / junta o index com todos os assets. Streaming de vídeo e SSE ficam de fora — uma resposta de duas horas destruiria qualquer noção de latência."
    >
      <div className="mb-4 grid grid-cols-3 gap-5">
        <Numero label="Requisições" valor={trafego.total.toLocaleString('pt-BR')} />
        <Numero label="Erros" valor={trafego.erros.toLocaleString('pt-BR')} />
        <Numero
          label="Taxa de erro"
          valor={`${(trafego.taxa_erro * 100).toFixed(1)}%`}
          nota={trafego.erros > 0 ? 'inclui 404 de rota do SPA' : undefined}
        />
      </div>

      {trafego.rotas.length === 0 ? (
        <p className="text-sm text-muted">Nenhuma requisição medida ainda nesta execução.</p>
      ) : (
        <ul className="divide-y divide-line overflow-hidden rounded-lg border border-line">
          {trafego.rotas.map((r) => (
            <LinhaDeRota key={r.rota} rota={r} maior={maior} />
          ))}
        </ul>
      )}

      <p className="mt-3 text-[11px] leading-relaxed text-muted">
        p50 e p95 são estimados a partir de doze faixas de latência, não da lista de amostras — a
        memória do painel não cresce com o tempo, mas o valor erra dentro da faixa. O máximo, esse,
        é medido de verdade.
      </p>

      {!!trafego.descartadas && (
        <p className="mt-3 text-[11px] text-warn">
          {trafego.descartadas} observações descartadas pelo teto de rotas distintas.
        </p>
      )}
    </Card>
  )
}

function LinhaDeRota({ rota, maior }: { rota: RouteSnapshot; maior: number }) {
  const fracao = maior > 0 ? rota.total / maior : 0
  const temErro = rota.erros_4xx + rota.erros_5xx > 0

  return (
    <li className="px-3 py-2.5">
      <div className="flex items-baseline justify-between gap-3">
        <code className="min-w-0 truncate text-xs text-ink">{rota.rota}</code>
        <span className="shrink-0 text-xs tabular-nums text-muted">
          {rota.total.toLocaleString('pt-BR')}
        </span>
      </div>

      <div className="mt-1.5">
        <Barra fracao={fracao} tom={rota.erros_5xx > 0 ? 'bg-danger' : 'bg-accent'} />
      </div>

      <div className="mt-1.5 flex flex-wrap gap-x-4 gap-y-0.5 text-[11px] text-muted tabular-nums">
        <span>p50 {ms(rota.p50_ms)}</span>
        <span>p95 {ms(rota.p95_ms)}</span>
        <span>máx {ms(rota.max_ms)}</span>
        {temErro && (
          <span className={rota.erros_5xx > 0 ? 'text-danger' : 'text-warn'}>
            {rota.erros_4xx > 0 && `${rota.erros_4xx}× 4xx`}
            {rota.erros_4xx > 0 && rota.erros_5xx > 0 && ' · '}
            {rota.erros_5xx > 0 && `${rota.erros_5xx}× 5xx`}
          </span>
        )}
      </div>
    </li>
  )
}
