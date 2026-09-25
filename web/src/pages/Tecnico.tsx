// Tela técnica: o que a nuvem está fazendo, para diagnóstico. Só lê — nada
// aqui muda o bucket, as filas ou o modo.
import { useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type CustoDaNuvem, type FilaTecnica, type InspecaoDoBucket, type NotaDoDiario, type Tecnico as DadosTecnicos } from '../lib/api'
import { humanSize } from '../lib/format'
import { mbps } from '../lib/envios'
import { EmptyState } from '../components/states'
import { ChipIcon, DownloadIcon, LuaSpinner, NuvemIcon, RefreshIcon } from '../components/icons'

// --- formatos ---------------------------------------------------------------

/** "agora", "há 3 min", "há 2 h", "há 4 dias". */
function ha(quando?: string | number): string {
  if (!quando) return '—'
  const t = typeof quando === 'number' ? quando : Date.parse(quando)
  if (!Number.isFinite(t)) return '—'
  const s = Math.round((Date.now() - t) / 1000)
  if (s < 0) return em(t)
  if (s < 45) return 'agora'
  if (s < 3600) return `há ${Math.round(s / 60)} min`
  if (s < 86400) return `há ${Math.round(s / 3600)} h`
  return `há ${Math.round(s / 86400)} dias`
}

/** Futuro: "em 42 min", "em 12 dias". */
function em(t: number): string {
  const s = Math.round((t - Date.now()) / 1000)
  if (s <= 0) return 'vencido'
  if (s < 3600) return `em ${Math.max(1, Math.round(s / 60))} min`
  if (s < 86400) return `em ${Math.round(s / 3600)} h`
  return `em ${Math.round(s / 86400)} dias`
}

const hora = (ms: number) =>
  new Date(ms).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit', second: '2-digit' })

const numero = (n: number) => n.toLocaleString('pt-BR')

// --- peças ------------------------------------------------------------------

function Card({
  title,
  description,
  extra,
  children,
}: {
  title: string
  description?: string
  extra?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <section className="rounded-xl border border-line bg-surface p-5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="text-sm font-semibold">{title}</h2>
          {description && <p className="mt-1 text-xs leading-relaxed text-muted">{description}</p>}
        </div>
        {extra}
      </div>
      <div className="mt-4">{children}</div>
    </section>
  )
}

function Dado({ rotulo, valor, tom }: { rotulo: string; valor: React.ReactNode; tom?: 'ok' | 'warn' | 'danger' }) {
  const cor = tom === 'ok' ? 'text-ok' : tom === 'warn' ? 'text-warn' : tom === 'danger' ? 'text-danger' : 'text-ink'
  return (
    <div className="min-w-0 rounded-lg bg-elev/60 px-3 py-2">
      <dt className="text-[11px] tracking-wide text-muted uppercase">{rotulo}</dt>
      <dd className={`mt-0.5 truncate font-mono text-sm ${cor}`} title={typeof valor === 'string' ? valor : undefined}>
        {valor}
      </dd>
    </div>
  )
}

function Indisponivel({ motivo }: { motivo?: string }) {
  return (
    <div className="flex items-center gap-3 rounded-lg border border-dashed border-line px-4 py-3 text-sm text-muted">
      <NuvemIcon estado="local" className="shrink-0" />
      <span>
        {motivo === 'modo local'
          ? 'Modo local: nenhuma consulta à AWS. Ligue o híbrido para ver o bucket e as filas.'
          : (motivo ?? 'Indisponível.')}
      </span>
    </div>
  )
}

/** Barras da velocidade de cada parte, em ordem de chegada. */
function Sparkline({ valores }: { valores: number[] }) {
  if (valores.length < 2) return null
  const max = Math.max(...valores)
  const w = 96
  const h = 22
  const passo = w / (valores.length - 1)
  const pontos = valores.map((v, i) => `${(i * passo).toFixed(1)},${(h - (v / max) * (h - 2) - 1).toFixed(1)}`).join(' ')
  return (
    <svg width={w} height={h} viewBox={`0 0 ${w} ${h}`} className="shrink-0 text-accent" aria-hidden="true">
      <polyline points={pontos} fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
    </svg>
  )
}

// --- cards ------------------------------------------------------------------

function PulsoCard({ dados }: { dados: DadosTecnicos }) {
  const p = dados.pulso
  const cred = dados.nuvem.bucket?.credencial_expira
  const certMs = p.certificado_vence ? Date.parse(p.certificado_vence) - Date.now() : undefined
  const certTom = certMs === undefined ? undefined : certMs < 0 ? 'danger' : certMs < 30 * 86400e3 ? 'warn' : 'ok'
  const estado = p.modo === 'hibrido' ? 'hibrido' : p.modo === 'conectando' ? 'conectando' : 'local'
  return (
    <Card
      title="Pulso da nuvem"
      description="O kill switch visto por dentro. No modo local, conexões abertas tem de ser zero; cada chamada que algum trecho tentar fazer mesmo assim é recusada e contada."
    >
      <div className="flex items-center gap-4">
        <div className={`grid h-14 w-14 shrink-0 place-items-center rounded-xl bg-elev ${estado === 'hibrido' ? 'text-accent' : 'text-muted'}`}>
          <NuvemIcon key={estado} estado={estado} className="h-8 w-8" />
        </div>
        <div className="min-w-0">
          <p className="text-lg font-semibold">{p.modo === 'hibrido' ? 'Híbrido' : p.modo === 'conectando' ? 'Conectando…' : 'Local'}</p>
          <p className="text-xs text-muted">
            desde {new Date(p.desde).toLocaleString('pt-BR')} · {ha(p.desde)}
            {p.travado && ' · travado (--sem-nuvem)'}
          </p>
          {p.erro && <p className="mt-1 text-xs text-danger">{p.erro}</p>}
        </div>
      </div>
      <dl className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-3">
        <Dado
          rotulo="Conexões abertas"
          valor={numero(p.conexoes)}
          tom={p.modo === 'local' ? (p.conexoes === 0 ? 'ok' : 'danger') : undefined}
        />
        <Dado rotulo="Chamadas bloqueadas" valor={numero(p.nuvem_bloqueadas_total)} />
        <Dado rotulo="Outbox pendente" valor={`${numero(p.eventos_pendentes)} eventos`} tom={p.eventos_pendentes > 0 ? 'warn' : undefined} />
        <Dado rotulo="Credencial (1 h)" valor={cred ? `vence ${em(Date.parse(cred))}` : p.modo === 'hibrido' ? '—' : 'descartada'} />
        <Dado
          rotulo={`Certificado${p.certificado_cn ? ` · ${p.certificado_cn}` : ''}`}
          valor={p.certificado_vence ? `vence ${em(Date.parse(p.certificado_vence))}` : 'não encontrado'}
          tom={certTom}
        />
        <Dado rotulo="Última reconciliação" valor={ha(p.ultima_reconciliacao)} />
      </dl>
      {certTom === 'warn' && (
        <p className="mt-3 rounded-lg bg-warn/10 px-3 py-2 text-xs text-warn">
          O certificado do Mac vence em menos de 30 dias. Traga o ca.key de volta e rode{' '}
          <code className="font-mono">sh scripts/roles-anywhere.sh --renovar</code>.
        </p>
      )}
    </Card>
  )
}

function BucketCard({ dados }: { dados: DadosTecnicos }) {
  const b = dados.nuvem.bucket
  return (
    <Card
      title="Raio-X do bucket"
      description="Tamanho por prefixo, classes do Intelligent-Tiering, regras e envios que ficaram pela metade. Consultado a cada minuto no máximo."
    >
      {dados.nuvem.indisponivel || !b ? <Indisponivel motivo={dados.nuvem.motivo} /> : <Bucket b={b} />}
    </Card>
  )
}

function Bucket({ b }: { b: InspecaoDoBucket }) {
  const total = b.prefixos.reduce((n, p) => n + p.bytes, 0)
  const objetos = b.prefixos.reduce((n, p) => n + p.objetos, 0)
  const erros = Object.entries(b.erros ?? {})
  return (
    <div className="space-y-4">
      <Dado rotulo="Bucket" valor={b.bucket} />
      <dl className="grid grid-cols-3 gap-2">
        <Dado rotulo="Região" valor={b.regiao} />
        <Dado rotulo="Total" valor={humanSize(total) || '0 B'} />
        <Dado rotulo="Versionamento" valor={b.versionamento || '—'} tom={b.versionamento === 'Enabled' ? 'ok' : undefined} />
      </dl>

      <div>
        <p className="mb-2 text-xs text-muted">{numero(objetos)} objetos</p>
        <ul className="space-y-2.5">
          {b.prefixos.map((p) => (
            <li key={p.prefixo}>
              <div className="flex items-baseline justify-between gap-3 text-sm">
                <span className="font-mono">{p.prefixo}</span>
                <span className="shrink-0 font-mono text-xs text-muted">
                  {humanSize(p.bytes) || '0 B'} · {numero(p.objetos)}
                  {p.truncado && '+'} obj
                </span>
              </div>
              <div className="mt-1 h-1.5 overflow-hidden rounded bg-elev">
                <div className="h-full rounded bg-accent" style={{ width: `${total ? Math.max(1, (p.bytes / total) * 100) : 0}%` }} />
              </div>
              <p className="mt-1 font-mono text-[11px] text-muted">
                {Object.entries(p.classes)
                  .map(([c, n]) => `${c || 'STANDARD'} ${numero(n)}`)
                  .join(' · ')}
              </p>
            </li>
          ))}
        </ul>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div>
          <h3 className="mb-1.5 text-xs font-semibold tracking-wide text-muted uppercase">Lifecycle</h3>
          <ul className="space-y-1.5 text-xs">
            {b.lifecycle.length === 0 && <li className="text-muted">nenhuma regra</li>}
            {b.lifecycle.map((r) => (
              <li key={r.id} className={r.ativa ? '' : 'opacity-50'}>
                <span className="font-mono">{r.id}</span>
                {r.prefixo && <span className="font-mono text-muted"> · {r.prefixo}</span>}
                <p className="text-muted">{r.resumo}</p>
              </li>
            ))}
          </ul>
        </div>
        <div>
          <h3 className="mb-1.5 text-xs font-semibold tracking-wide text-muted uppercase">CORS</h3>
          <ul className="space-y-0.5 font-mono text-xs">
            {b.cors.length === 0 && <li className="text-muted">sem regras</li>}
            {b.cors.map((o) => (
              <li key={o} className="truncate">
                {o}
              </li>
            ))}
          </ul>
        </div>
      </div>

      <div>
        <h3 className="mb-1.5 text-xs font-semibold tracking-wide text-muted uppercase">
          Multiparts pendentes · {b.multiparts_pendentes.length}
        </h3>
        {b.multiparts_pendentes.length === 0 ? (
          <p className="text-xs text-muted">nenhum</p>
        ) : (
          <>
            <ul className="divide-y divide-line rounded-lg border border-line text-xs">
              {b.multiparts_pendentes.slice(0, 8).map((m) => (
                <li key={m.key + m.iniciado} className="flex justify-between gap-3 px-3 py-1.5">
                  <span className="truncate font-mono">{m.key}</span>
                  <span className="shrink-0 text-muted">{ha(m.iniciado)}</span>
                </li>
              ))}
            </ul>
            <p className="mt-1.5 text-[11px] text-muted">
              Envios pausados ficam aqui até retomar. Os abandonados o lifecycle limpa em 7 dias.
            </p>
          </>
        )}
      </div>

      {erros.length > 0 && (
        <ul className="space-y-1 rounded-lg bg-danger/10 px-3 py-2 text-xs text-danger">
          {erros.map(([parte, erro]) => (
            <li key={parte}>
              <span className="font-semibold">{parte}:</span> <span className="break-words">{erro}</span>
            </li>
          ))}
          <li className="text-muted">Falta de permissão? Aplique o infra/identidade.tf (InspecionarBucket).</li>
        </ul>
      )}
    </div>
  )
}

function FilasCard({ dados, jobs }: { dados: DadosTecnicos; jobs: NotaDoDiario[] }) {
  const visto = dados.pulso.worker_visto
  return (
    <Card
      title="Filas e workers"
      description="Pedidos de processamento para os workers e o que eles respondem. Mensagem na DLQ é job que falhou cinco vezes."
    >
      {dados.nuvem.indisponivel ? (
        <Indisponivel motivo={dados.nuvem.motivo} />
      ) : dados.nuvem.filas.length === 0 ? (
        <p className="text-sm text-muted">Nenhuma fila configurada (nuvem.fila_jobs, nuvem.fila_eventos).</p>
      ) : (
        <ul className="divide-y divide-line rounded-lg border border-line">
          {dados.nuvem.filas.map((f) => (
            <Fila key={f.papel} f={f} />
          ))}
        </ul>
      )}

      <div className="mt-4 flex items-center gap-2 text-sm">
        <NuvemIcon estado={visto ? 'processando' : 'local'} className={visto ? 'text-accent' : 'text-muted'} />
        {visto ? (
          <span>
            Worker visto <span className="font-mono">{ha(visto)}</span>
          </span>
        ) : (
          <span className="text-muted">Nenhum worker respondeu ainda: o Mac prepara ele mesmo.</span>
        )}
      </div>

      {jobs.length > 0 && (
        <ul className="mt-3 space-y-1.5 text-xs">
          {jobs.slice(0, 8).map((j) => {
            const d = j.dados as { ms?: number; derivados?: string[]; erro?: string }
            const falhou = j.tipo === 'job.falhou' || j.tipo === 'job.erro'
            return (
              <li key={j.id} className="flex items-baseline justify-between gap-3">
                <span className="flex min-w-0 items-baseline gap-2">
                  <span className={`shrink-0 font-mono ${falhou ? 'text-danger' : j.tipo === 'job.concluido' ? 'text-ok' : 'text-muted'}`}>
                    {j.tipo.replace('job.', '')}
                  </span>
                  <span className="truncate">{j.nome}</span>
                </span>
                <span className="shrink-0 font-mono text-muted">
                  {d.derivados?.length ? `${d.derivados.join(', ')} · ` : ''}
                  {d.ms ? `${Math.round(d.ms / 60000) || '<1'} min · ` : ''}
                  {ha(j.em)}
                </span>
              </li>
            )
          })}
        </ul>
      )}
    </Card>
  )
}

function Fila({ f }: { f: FilaTecnica }) {
  const dlq = f.papel.startsWith('dlq')
  return (
    <li className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
      <span className="min-w-0">
        <span className="font-medium">{f.papel}</span>
        <span className="block truncate font-mono text-[11px] text-muted">{f.nome}</span>
      </span>
      {f.erro ? (
        <span className="line-clamp-2 max-w-[60%] text-right text-xs text-danger">{f.erro}</span>
      ) : (
        <span className="flex shrink-0 gap-3 font-mono text-xs">
          <span className={dlq && f.visiveis > 0 ? 'text-danger' : ''}>{numero(f.visiveis)} na fila</span>
          <span className="text-muted">{numero(f.em_voo)} em voo</span>
        </span>
      )}
    </li>
  )
}

// --- custo ------------------------------------------------------------------

/** Nomes do Cost Explorer são longos; estes são os que o Ozymandias usa. */
const SERVICOS: Record<string, string> = {
  'Amazon Simple Storage Service': 'S3 (bucket)',
  'Amazon CloudFront': 'CloudFront',
  'Amazon Elastic Container Service': 'ECS (workers e instância)',
  'Amazon EC2 Container Registry (ECR)': 'ECR (imagem)',
  'Amazon Simple Queue Service': 'SQS (filas)',
  'Amazon Simple Notification Service': 'SNS',
  'AWS CodeBuild': 'CodeBuild (build da imagem)',
  AmazonCloudWatch: 'CloudWatch',
  'Amazon Virtual Private Cloud': 'VPC (IPv4 público)',
  'AWS Key Management Service': 'KMS',
  'AWS CloudTrail': 'CloudTrail',
  'AWS Systems Manager': 'SSM',
  Tax: 'Impostos',
}

const dolar = (v: number, moeda = 'USD') =>
  v.toLocaleString('pt-BR', { style: 'currency', currency: moeda, minimumFractionDigits: 2, maximumFractionDigits: v < 1 ? 3 : 2 })

function CustoCard() {
  const queryClient = useQueryClient()
  const [forcando, setForcando] = useState(false)
  const { data } = useQuery({ queryKey: ['custo'], queryFn: () => api.custo(), staleTime: 10 * 60e3 })
  const atualizar = async () => {
    setForcando(true)
    try {
      queryClient.setQueryData(['custo'], await api.custo(true))
    } finally {
      setForcando(false)
    }
  }
  const c = data?.custo
  return (
    <Card
      title="Custo da nuvem"
      description="Gasto do mês na conta AWS, por serviço, contra o orçamento do alerta. O Cost Explorer atualiza algumas vezes por dia e cobra US$ 0,01 por consulta: o painel guarda o resultado por 6 h."
      extra={
        <button
          type="button"
          onClick={atualizar}
          disabled={forcando || !data || (data.indisponivel && data.motivo === 'modo local') || data.do_cache}
          className="inline-flex shrink-0 items-center gap-1.5 rounded-lg border border-line px-2.5 py-1.5 text-xs transition hover:bg-elev disabled:opacity-50"
          title="No máximo uma consulta a cada 10 min"
        >
          {forcando ? <LuaSpinner className="text-ink" /> : <RefreshIcon />} Atualizar
        </button>
      }
    >
      {!data ? (
        <div className="flex justify-center py-6 text-muted">
          <LuaSpinner />
        </div>
      ) : !c ? (
        <Indisponivel motivo={data.motivo} />
      ) : (
        <Custo c={c} doCache={!!data.do_cache} />
      )}
    </Card>
  )
}

function Custo({ c, doCache }: { c: CustoDaNuvem; doCache: boolean }) {
  const teto = Math.max(c.limite ?? 0, c.previsao ?? 0, c.total)
  const pct = (v: number) => `${teto > 0 ? Math.min(100, (v / teto) * 100) : 0}%`
  const passou = c.limite !== undefined && (c.previsao ?? c.total) > c.limite
  const perto = !passou && c.limite !== undefined && (c.previsao ?? c.total) > c.limite * 0.8
  const maxDia = Math.max(...c.por_dia.map((d) => d.valor), 0.0001)
  const [ano, mes] = c.mes.split('-').map(Number)
  const nomeDoMes = new Date(ano, mes - 1, 1).toLocaleDateString('pt-BR', { month: 'long', year: 'numeric' })
  const erros = Object.entries(c.erros ?? {})

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="text-[11px] tracking-wide text-muted uppercase">{nomeDoMes} até agora</p>
          <p className="font-mono text-3xl font-semibold">{dolar(c.total, c.moeda)}</p>
        </div>
        <div className="text-right text-xs text-muted">
          {c.previsao !== undefined && (
            <p>
              previsão do mês <span className={`font-mono ${passou ? 'text-danger' : perto ? 'text-warn' : 'text-ink'}`}>{dolar(c.previsao, c.moeda)}</span>
            </p>
          )}
          {c.limite !== undefined && (
            <p>
              orçamento <span className="font-mono text-ink">{dolar(c.limite, c.moeda)}</span>
            </p>
          )}
          <p>
            consultado {ha(c.em)}
            {doCache && ' · nuvem desligada, último valor guardado'}
          </p>
        </div>
      </div>

      {teto > 0 && (
        <div className="relative h-2.5 overflow-hidden rounded-full bg-elev" aria-hidden="true">
          {c.previsao !== undefined && <div className="absolute inset-y-0 left-0 rounded-full bg-accent/30" style={{ width: pct(c.previsao) }} />}
          <div className={`absolute inset-y-0 left-0 rounded-full ${passou ? 'bg-danger' : 'bg-accent'}`} style={{ width: pct(c.total) }} />
          {c.limite !== undefined && <div className="absolute inset-y-0 w-0.5 bg-ink/70" style={{ left: `calc(${pct(c.limite)} - 1px)` }} />}
        </div>
      )}
      {passou && (
        <p className="rounded-lg bg-danger/10 px-3 py-2 text-xs text-danger">
          A previsão passa do orçamento. Veja abaixo qual serviço puxa o gasto.
        </p>
      )}

      {c.por_dia.length > 1 && (
        <div>
          <div className="flex h-16 items-end gap-[3px]">
            {c.por_dia.map((d) => (
              <div
                key={d.dia}
                className="flex-1 rounded-t bg-accent/70 transition-colors hover:bg-accent"
                style={{ height: `${Math.max(2, (d.valor / maxDia) * 100)}%` }}
                title={`${d.dia.slice(8)}/${d.dia.slice(5, 7)} · ${dolar(d.valor, c.moeda)}`}
              />
            ))}
          </div>
          <div className="mt-1 flex justify-between font-mono text-[10px] text-muted">
            <span>{c.por_dia[0].dia.slice(8)}</span>
            <span>por dia</span>
            <span>{c.por_dia[c.por_dia.length - 1].dia.slice(8)}</span>
          </div>
        </div>
      )}

      <ul className="space-y-1.5">
        {c.por_servico.filter((s) => s.valor >= 0.0005).length === 0 && <li className="text-xs text-muted">Nenhum gasto registrado ainda.</li>}
        {c.por_servico
          .filter((s) => s.valor >= 0.0005)
          .map((s) => (
            <li key={s.servico} className="text-sm">
              <div className="flex items-baseline justify-between gap-3">
                <span className="truncate" title={s.servico}>
                  {SERVICOS[s.servico] ?? s.servico}
                </span>
                <span className="shrink-0 font-mono text-xs">{dolar(s.valor, c.moeda)}</span>
              </div>
              <div className="mt-1 h-1 overflow-hidden rounded bg-elev">
                <div className="h-full rounded bg-accent/80" style={{ width: `${c.total > 0 ? (s.valor / c.total) * 100 : 0}%` }} />
              </div>
            </li>
          ))}
      </ul>

      {erros.length > 0 && (
        <ul className="space-y-1 rounded-lg bg-danger/10 px-3 py-2 text-xs text-danger">
          {erros.map(([parte, erro]) => (
            <li key={parte}>
              <span className="font-semibold">{parte}:</span> <span className="break-words">{erro}</span>
            </li>
          ))}
          <li className="text-muted">
            Falta de permissão? Aplique o infra/identidade.tf (Mac) ou o infra/nuvem.tf (instância cloud): LerCustos e LerOrcamento. Conta nova: abra o Cost Explorer uma vez no console para ativá-lo.
          </li>
        </ul>
      )}
    </div>
  )
}

// --- diário -----------------------------------------------------------------

const FILTROS = [
  { rotulo: 'Tudo', tipo: '' },
  { rotulo: 'Envios', tipo: 'envio.' },
  { rotulo: 'Jobs', tipo: 'job.' },
  { rotulo: 'Kill switch', tipo: 'modo.' },
  { rotulo: 'Federação', tipo: 'evento.' },
]

interface Grupo {
  chave: string
  upload?: number
  nome: string
  notas: NotaDoDiario[] // mais nova primeiro
}

/** Envios viram um grupo por upload; o resto fica uma linha cada. */
function agrupa(notas: NotaDoDiario[]): Grupo[] {
  const grupos: Grupo[] = []
  const porUpload = new Map<number, Grupo>()
  for (const n of notas) {
    if (n.upload_id && n.tipo.startsWith('envio.')) {
      let g = porUpload.get(n.upload_id)
      if (!g) {
        g = { chave: `u${n.upload_id}`, upload: n.upload_id, nome: n.nome ?? '', notas: [] }
        porUpload.set(n.upload_id, g)
        grupos.push(g)
      }
      g.notas.push(n)
    } else {
      grupos.push({ chave: `n${n.id}`, nome: n.nome ?? '', notas: [n] })
    }
  }
  return grupos
}

const TOM_DO_TIPO: Record<string, string> = {
  'envio.concluido': 'text-ok',
  'envio.erro': 'text-danger',
  'envio.pausa': 'text-warn',
  'envio.abortado': 'text-danger',
  'job.concluido': 'text-ok',
  'job.falhou': 'text-danger',
  'job.erro': 'text-danger',
  'modo.local': 'text-warn',
  'modo.hibrido': 'text-ok',
  'modo.bloqueadas': 'text-warn',
}

function descreve(n: NotaDoDiario): string {
  const d = n.dados as Record<string, unknown>
  switch (n.tipo) {
    case 'envio.inicio':
      return `${d.origem === 'mac' ? 'do Mac' : 'do navegador'} · ${humanSize(Number(d.tamanho))} em partes de ${humanSize(Number(d.parte_tamanho))}`
    case 'envio.parte':
      return `parte ${d.n} · ${humanSize(Number(d.tamanho))} em ${(Number(d.ms) / 1000).toFixed(1)} s`
    case 'envio.urls':
      return `assinou ${(d.partes as number[])?.length ?? 0} URLs`
    case 'envio.concluido':
      return `${d.partes} partes fechadas${d.etag ? ` · ETag ${String(d.etag).slice(0, 12)}…` : ''}`
    case 'envio.pausa':
      return 'pausado pelo kill switch'
    case 'envio.retomada':
      return 'retomado do que o bucket já tinha'
    case 'modo.local':
      return `nuvem cortada · ${d.conexoes ?? 0} conexões no instante do corte${d.erro ? ` · ${d.erro}` : ''}`
    case 'modo.hibrido':
      return 'híbrido ligado'
    case 'modo.conectando':
      return 'conectando…'
    case 'modo.bloqueadas':
      return `${d.n} chamadas recusadas pelo guard (total ${d.total})`
    case 'evento.recebido':
    case 'evento.repetido':
      return `${n.nome} · de ${d.origem}`
    case 'diario.poda':
      return `${d.apagadas} notas com mais de 30 dias apagadas`
    default:
      return d.erro ? String(d.erro) : d.key ? String(d.key) : ''
  }
}

function Linha({ n }: { n: NotaDoDiario }) {
  return (
    <div className="flex items-baseline gap-3 text-xs">
      <span className="shrink-0 font-mono text-muted">{hora(n.em)}</span>
      <span className={`w-28 shrink-0 font-mono ${TOM_DO_TIPO[n.tipo] ?? 'text-ink'}`}>{n.tipo}</span>
      <span className="min-w-0 break-words text-muted">
        {n.nome && !n.tipo.startsWith('envio.') && !n.tipo.startsWith('evento.') && <span className="text-ink">{n.nome} · </span>}
        {descreve(n)}
      </span>
    </div>
  )
}

function GrupoDeEnvio({ g }: { g: Grupo }) {
  const [aberto, setAberto] = useState(false)
  const partes = g.notas.filter((n) => n.tipo === 'envio.parte').reverse()
  const velocidades = partes.map((p) => {
    const d = p.dados as { tamanho: number; ms: number }
    return d.ms > 0 ? (d.tamanho / d.ms) * 1000 : 0
  })
  const media = velocidades.length ? velocidades.reduce((a, b) => a + b, 0) / velocidades.length : 0
  const ultimo = g.notas[0]
  const inicio = g.notas.find((n) => n.tipo === 'envio.inicio')
  const tamanho = Number((inicio?.dados as { tamanho?: number })?.tamanho ?? 0)
  return (
    <li className="px-3 py-2.5">
      <button type="button" onClick={() => setAberto(!aberto)} className="flex w-full items-center gap-3 text-left" aria-expanded={aberto}>
        <NuvemIcon
          estado={ultimo.tipo === 'envio.concluido' ? 'concluido' : ultimo.tipo === 'envio.erro' ? 'erro' : ultimo.tipo === 'envio.pausa' ? 'local' : 'enviando'}
          className={`shrink-0 ${TOM_DO_TIPO[ultimo.tipo] ?? 'text-accent'}`}
        />
        <span className="min-w-0 flex-1">
          <span className="line-clamp-1 text-sm font-medium">{g.nome}</span>
          <span className="block font-mono text-[11px] text-muted">
            #{g.upload} · {ultimo.tipo.replace('envio.', '')} · {partes.length} partes
            {tamanho ? ` · ${humanSize(tamanho)}` : ''}
            {media ? ` · média ${mbps(media)}` : ''} · {ha(ultimo.em)}
          </span>
        </span>
        <Sparkline valores={velocidades} />
        <span className={`shrink-0 text-muted transition ${aberto ? 'rotate-90' : ''}`} aria-hidden="true">
          ›
        </span>
      </button>
      {aberto && (
        <div className="mt-2 max-h-72 space-y-1 overflow-y-auto border-l border-line pl-3">
          {g.notas.map((n) => (
            <div key={n.id}>
              <Linha n={n} />
              {n.tipo === 'envio.parte' && (n.dados as { etag?: string }).etag && (
                <p className="pl-[5.5rem] font-mono text-[10px] break-all text-muted/70">{String((n.dados as { etag?: string }).etag)}</p>
              )}
            </div>
          ))}
        </div>
      )}
    </li>
  )
}

function DiarioCard({ notas, ao_vivo }: { notas: NotaDoDiario[]; ao_vivo: boolean }) {
  const [filtro, setFiltro] = useState('')
  const [busca, setBusca] = useState('')
  const grupos = useMemo(() => {
    const termo = busca.trim().toLowerCase()
    const filtradas = notas.filter(
      (n) => n.tipo.startsWith(filtro) && (!termo || (n.nome ?? '').toLowerCase().includes(termo) || n.tipo.includes(termo)),
    )
    return agrupa(filtradas)
  }, [notas, filtro, busca])

  const exporta = () => {
    const blob = new Blob([JSON.stringify(notas, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `ozymandias-diario-${new Date().toISOString().slice(0, 19).replace(/:/g, '')}.json`
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <Card
      title="Diário de envios"
      description="Cada envio parte a parte (com tempo e ETag), pausas do kill switch, retomadas, jobs e eventos da federação. Guardado por 30 dias."
      extra={
        <span className={`inline-flex shrink-0 items-center gap-1.5 text-[11px] ${ao_vivo ? 'text-ok' : 'text-muted'}`}>
          <span className={`h-1.5 w-1.5 rounded-full ${ao_vivo ? 'animate-pulse bg-ok' : 'bg-muted'}`} />
          {ao_vivo ? 'ao vivo' : 'parado'}
        </span>
      }
    >
      <div className="flex flex-wrap items-center gap-2">
        {FILTROS.map((f) => (
          <button
            key={f.rotulo}
            type="button"
            onClick={() => setFiltro(f.tipo)}
            className={`rounded-full px-3 py-1 text-xs transition ${filtro === f.tipo ? 'bg-accent text-accent-ink' : 'bg-elev text-muted hover:text-ink'}`}
          >
            {f.rotulo}
          </button>
        ))}
        <input
          type="search"
          value={busca}
          onChange={(e) => setBusca(e.target.value)}
          placeholder="arquivo ou tipo…"
          className="ml-auto w-full min-w-0 rounded-lg border border-line bg-bg px-3 py-1.5 text-xs sm:w-48"
        />
        <button
          type="button"
          onClick={exporta}
          disabled={notas.length === 0}
          className="inline-flex items-center gap-1.5 rounded-lg border border-line px-2.5 py-1.5 text-xs transition hover:bg-elev disabled:opacity-50"
        >
          <DownloadIcon /> JSON
        </button>
      </div>

      {grupos.length === 0 ? (
        <p className="mt-4 text-sm text-muted">Nada por aqui ainda. Envie algo para a nuvem e acompanhe parte a parte.</p>
      ) : (
        <ul className="mt-3 divide-y divide-line rounded-lg border border-line">
          {grupos.slice(0, 150).map((g) =>
            g.upload ? (
              <GrupoDeEnvio key={g.chave} g={g} />
            ) : (
              <li key={g.chave} className="px-3 py-2">
                <Linha n={g.notas[0]} />
              </li>
            ),
          )}
        </ul>
      )}
    </Card>
  )
}

/** O diário: as últimas 500 notas e, por SSE, as novas no topo. */
function useDiario() {
  const queryClient = useQueryClient()
  const { data } = useQuery({ queryKey: ['diario'], queryFn: () => api.diario({ limite: 500 }) })
  const [novas, setNovas] = useState<NotaDoDiario[]>([])
  const [aoVivo, setAoVivo] = useState(false)
  useEffect(() => {
    if (!('EventSource' in window)) return
    const fonte = new EventSource('/api/tecnico/eventos')
    fonte.onopen = () => setAoVivo(true)
    fonte.onerror = () => setAoVivo(false)
    fonte.onmessage = (e) => {
      const n = JSON.parse(e.data) as NotaDoDiario
      setNovas((l) => [n, ...l].slice(0, 2000))
      // Uma virada de modo ou um job muda os outros cards também.
      if (n.tipo.startsWith('modo.') || n.tipo.startsWith('job.')) void queryClient.invalidateQueries({ queryKey: ['tecnico'] })
    }
    return () => fonte.close()
  }, [queryClient])
  const notas = useMemo(() => {
    const vistos = new Set<number>()
    return [...novas, ...(data ?? [])].filter((n) => (vistos.has(n.id) ? false : (vistos.add(n.id), true)))
  }, [novas, data])
  return { notas, aoVivo }
}

// --- página -----------------------------------------------------------------

export function Tecnico() {
  const queryClient = useQueryClient()
  const { data: user } = useQuery({ queryKey: ['me'], queryFn: api.me })
  const [forcando, setForcando] = useState(false)
  const { data, error } = useQuery({
    queryKey: ['tecnico'],
    queryFn: () => api.tecnico(),
    refetchInterval: 15000,
    enabled: !!user?.is_admin,
  })
  const { notas, aoVivo } = useDiario()
  const jobs = useMemo(() => notas.filter((n) => n.tipo.startsWith('job.')), [notas])

  if (user && !user.is_admin) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-6 sm:px-6">
        <EmptyState title="Painel do administrador" description="A tela técnica é visível apenas para quem mantém o NAS." />
      </div>
    )
  }

  const atualizar = async () => {
    setForcando(true)
    try {
      queryClient.setQueryData(['tecnico'], await api.tecnico(true))
    } finally {
      setForcando(false)
    }
  }

  return (
    <div className="mx-auto max-w-5xl space-y-6 px-4 py-6 sm:px-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold tracking-tight sm:text-2xl">
            <ChipIcon className="text-accent" /> Técnico
          </h1>
          <p className="mt-1 text-sm text-muted">
            O que a nuvem está fazendo, por dentro. Só leitura.
            {data && !data.nuvem.indisponivel && ` Bucket consultado ${ha(data.nuvem.em)}.`}
          </p>
        </div>
        <button
          type="button"
          onClick={atualizar}
          disabled={forcando || !data || data.nuvem.indisponivel}
          className="inline-flex items-center gap-2 rounded-lg border border-line px-3.5 py-2 text-sm font-medium transition hover:bg-elev disabled:opacity-50"
        >
          {forcando ? <LuaSpinner className="text-ink" /> : <RefreshIcon />} Consultar a AWS agora
        </button>
      </div>

      {error && <p className="rounded-lg bg-danger/10 px-3 py-2 text-sm text-danger">{(error as Error).message}</p>}
      {!data ? (
        <div className="flex justify-center py-12 text-muted">
          <LuaSpinner />
        </div>
      ) : (
        <>
          <PulsoCard dados={data} />
          <CustoCard />
          <div className="grid gap-6 lg:grid-cols-2">
            <BucketCard dados={data} />
            <FilasCard dados={data} jobs={jobs} />
          </div>
        </>
      )}
      <DiarioCard notas={notas} ao_vivo={aoVivo} />
    </div>
  )
}
