import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../lib/api'
import { useScanStatus } from '../lib/useScanStatus'
import { useTheme } from '../lib/theme'
import { kindLabel } from '../lib/format'
import { CloudIcon, LuaSpinner, MoonIcon, NuvemIcon, RefreshIcon, SunIcon, UploadIcon, type EstadoNuvem } from '../components/icons'
import { NuvemDeEnvio } from '../components/NuvemDeEnvio'
import { chaveModo, useModoNuvem } from '../lib/nuvem'
import { criarEnvio, enviar, PausadoPeloModo } from '../lib/upload'
import { humanSize } from '../lib/format'

export function Settings() {
  const { data: user } = useQuery({ queryKey: ['me'], queryFn: api.me })
  const admin = user?.is_admin ?? false
  const naNuvem = useModoNuvem()?.papel === 'nuvem'

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
      {naNuvem && <InstanciaCloudCard />}
      {admin && !naNuvem && <NuvemCard />}
      {admin && <UploadCard />}
      {admin && <SincronizacaoCard />}
      {admin && !naNuvem && <LibrariesCard />}
      {admin && !naNuvem && <MetadataCard />}
      <AppearanceCard />
      {naNuvem ? (
        <Card title="Senha" description="Contas e senhas são do Mac: troque por lá, e a mudança chega aqui no próximo sincronismo." >
          <p className="text-xs text-muted">Esta é a instância cloud.</p>
        </Card>
      ) : (
        <PasswordCard />
      )}
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

function InstanciaCloudCard() {
  return (
    <Card
      title="Instância cloud"
      description="Você está no Ozymandias da nuvem, que fica no ar com o Mac dormindo. Ele mostra o acervo inteiro, toca o que tem cópia no bucket e recebe envios; o que só existe no Mac aparece como indisponível. Bibliotecas, contas e metadados vêm do Mac."
    >
      <p className="inline-flex items-center gap-2 rounded-lg bg-accent/15 px-3 py-1.5 text-sm font-semibold text-accent">
        <CloudIcon /> Nuvem
      </p>
    </Card>
  )
}

const ESTRATO = 'M0 40 C60 18 140 22 210 30 C260 10 330 12 382 30 C432 24 472 34 500 44 C420 54 300 52 200 54 C120 56 50 52 0 40 Z'

/** O corte, dito com a cena: os dois bancos de nuvem se abrem e o céu fica
 *  limpo. Dura o tempo de ler a frase; o estado real já é o selo. */
function NuvemCortada() {
  return (
    <div role="status" className="relative mt-4 overflow-hidden rounded-lg border border-line bg-bg px-4 py-3">
      <svg viewBox="0 0 600 70" className="pointer-events-none absolute inset-0 h-full w-full" preserveAspectRatio="xMidYMid slice" aria-hidden="true">
        <g className="nv-dispersa-e">
          <path d={ESTRATO} transform="translate(-20 4) scale(0.9)" className="fill-elev" />
        </g>
        <g className="nv-dispersa-d">
          <path d={ESTRATO} transform="translate(240 10) scale(0.8)" className="fill-elev" />
        </g>
      </svg>
      <p className="relative text-sm font-semibold text-ok">A nuvem foi cortada</p>
      <p className="relative text-xs text-muted">Nenhuma chamada sai do Mac. Envios e downloads continuam quando o híbrido voltar.</p>
    </div>
  )
}

function NuvemCard() {
  const queryClient = useQueryClient()
  const estado = useModoNuvem()
  const [cortou, setCortou] = useState(0)
  const alternar = useMutation({
    mutationFn: api.setModo,
    onSuccess: (_, modo) => {
      if (modo === 'local') setCortou(Date.now())
    },
    onSettled: () => void queryClient.invalidateQueries({ queryKey: chaveModo }),
  })
  useEffect(() => {
    if (!cortou) return
    const t = window.setTimeout(() => setCortou(0), 3500)
    return () => window.clearTimeout(t)
  }, [cortou])

  const modo = estado?.modo ?? 'local'
  const hibrido = modo === 'hibrido'
  const bloqueio = !estado?.suporte
    ? 'Este binário foi compilado sem suporte a nuvem.'
    : estado.travado
      ? 'Servidor iniciado com --sem-nuvem: o híbrido está travado nesta execução.'
      : !estado.configurada
        ? 'Configure o bucket no terminal: nas config set nuvem.bucket … e nuvem.regiao …'
        : ''

  return (
    <Card
      title="Modo de nuvem"
      description="Local é o modo base: o Mac não faz nenhuma chamada à AWS, e itens que moram só no bucket aparecem como indisponíveis. Híbrido liga o bucket. Alternar não perde nada."
    >
      <div className="flex flex-wrap items-center gap-3">
        <span
          className={[
            'inline-flex items-center gap-2 rounded-lg px-3 py-1.5 text-sm font-semibold',
            hibrido ? 'bg-accent/15 text-accent' : 'bg-elev text-ink',
          ].join(' ')}
        >
          <NuvemIcon key={modo} estado={modo === 'conectando' ? 'conectando' : hibrido ? 'hibrido' : 'local'} />
          {modo === 'conectando' ? 'Conectando…' : hibrido ? 'Híbrido' : 'Local'}
        </span>

        {hibrido || modo === 'conectando' ? (
          <button
            type="button"
            onClick={() => alternar.mutate('local')}
            className="rounded-lg bg-danger px-3.5 py-2 text-sm font-semibold text-white transition hover:opacity-90"
          >
            Cortar a nuvem agora
          </button>
        ) : (
          <button
            type="button"
            onClick={() => alternar.mutate('hibrido')}
            disabled={!!bloqueio || alternar.isPending}
            className="rounded-lg bg-accent px-3.5 py-2 text-sm font-semibold text-accent-ink transition hover:opacity-90 disabled:opacity-50"
          >
            {alternar.isPending ? 'Conectando…' : 'Ligar o híbrido'}
          </button>
        )}
      </div>

      {cortou > 0 && <NuvemCortada key={cortou} />}
      {bloqueio && !hibrido && <p className="mt-3 text-xs text-muted">{bloqueio}</p>}
      {estado?.erro && (
        <p role="alert" className="mt-3 rounded-lg border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger">
          {estado.erro}
        </p>
      )}
      <p className="mt-3 text-xs text-muted">
        Chamadas à nuvem bloqueadas nesta execução:{' '}
        <span className="font-mono text-ink">{estado?.nuvem_bloqueadas_total ?? 0}</span>
      </p>
    </Card>
  )
}

interface Envio {
  chave: string
  arquivo: File
  uploadId?: number
  enviados: number
  estado: 'enviando' | 'pausado' | 'pronto' | 'erro'
  erro?: string
  /** Envios soltos juntos (ou enquanto outros ainda sobem) formam um lote:
   *  é a unidade da nuvem que cresce e do raio no fim. */
  lote: number
}

/** Um envio na lista, venha do navegador (upload direto) ou do Mac (o motor
 *  subindo um arquivo do disco, pedido na página do título). */
interface Linha {
  chave: string
  nome: string
  total: number
  feitos: number
  estado: Envio['estado']
  erro?: string
  lote: number
  doMac: boolean
  tentar?: () => void
}

/** "~3 min", "menos de 1 min", "~1 h 20 min". */
function restante(segundos: number): string {
  if (!isFinite(segundos) || segundos <= 0) return ''
  if (segundos < 60) return 'menos de 1 min'
  const min = Math.round(segundos / 60)
  if (min < 60) return `~${min} min`
  return `~${Math.floor(min / 60)} h${min % 60 ? ` ${min % 60} min` : ''}`
}

const mbps = (bps: number) => `${Math.max(1, Math.round((bps * 8) / 1e6))} Mbps`

/**
 * Velocidade de cada envio em bytes/s, suavizada (média móvel exponencial):
 * as partes chegam aos saltos de 16 MiB, e a velocidade crua faria a
 * previsão pular de "1 min" para "9 min" a cada parte. As primeiras leituras
 * esperam 3 s de amostra antes de prever qualquer coisa.
 */
function useVelocidades(linhas: Linha[]): Map<string, number> {
  const amostras = useRef(new Map<string, { t: number; feitos: number; inicio: number; bps: number }>())
  const agora = performance.now()
  const vel = new Map<string, number>()
  for (const l of linhas) {
    if (l.estado !== 'enviando') {
      amostras.current.delete(l.chave)
      continue
    }
    const a = amostras.current.get(l.chave)
    if (!a) {
      amostras.current.set(l.chave, { t: agora, feitos: l.feitos, inicio: agora, bps: 0 })
      continue
    }
    const dt = (agora - a.t) / 1000
    if (l.feitos > a.feitos && dt > 0.25) {
      const inst = (l.feitos - a.feitos) / dt
      a.bps = a.bps ? a.bps * 0.7 + inst * 0.3 : inst
      a.t = agora
      a.feitos = l.feitos
    }
    if (a.bps > 0 && agora - a.inicio > 3000) vel.set(l.chave, a.bps)
  }
  return vel
}

/** O lote mais recente numa nuvem só: o progresso somado das linhas dele que
 *  não falharam. O raio cai quando a última termina. */
function LoteDeEnvio({ linhas, velocidades }: { linhas: Linha[]; velocidades: Map<string, number> }) {
  const ultimo = linhas.reduce((n, e) => Math.max(n, e.lote), 0)
  const lote = linhas.filter((e) => e.lote === ultimo && e.estado !== 'erro')
  const total = lote.reduce((n, e) => n + e.total, 0)
  const feito = lote.reduce((n, e) => n + (e.estado === 'pronto' ? e.total : e.feitos), 0)
  const ativo = lote.some((e) => e.estado === 'enviando')
  const progresso = total > 0 ? feito / total : 0
  // Previsão do lote: o que falta sobre a soma das velocidades dos envios
  // em curso (eles sobem em paralelo e dividem o link).
  const bps = lote.reduce((n, e) => n + (velocidades.get(e.chave) ?? 0), 0)
  const falta = restante(bps > 0 ? (total - feito) / bps : 0)
  if (lote.length === 0) return null
  return (
    <div className="mt-4 flex flex-col items-center gap-1">
      <NuvemDeEnvio progresso={progresso} ativo={ativo} />
      <p className="font-mono text-xs text-muted" aria-live="polite">
        {!ativo
          ? progresso >= 1
            ? 'tudo na nuvem'
            : ''
          : `${Math.round(progresso * 100)}% de ${humanSize(total)}${falta ? ` · faltam ${falta} · ${mbps(bps)}` : ' · calculando…'}`}
      </p>
    </div>
  )
}

interface EnvioDoMac {
  nome: string
  total: number
  feitos: number
  estado: Envio['estado']
  erro?: string
  lote: number
}

function UploadCard() {
  const queryClient = useQueryClient()
  const { data: libraries } = useQuery({ queryKey: ['libraries'], queryFn: api.libraries })
  const estado = useModoNuvem()
  const hibrido = estado?.modo === 'hibrido'
  const [libId, setLibId] = useState(0)
  const [envios, setEnvios] = useState<Envio[]>([])
  const [arrastando, setArrastando] = useState(false)
  const [doMac, setDoMac] = useState<Record<number, EnvioDoMac>>({})

  // Envios que o motor faz a partir do disco ("Enviar à nuvem" no título). A
  // mesma consulta do painel de sincronização; rápida só enquanto há envio.
  const { data: sinc } = useQuery({
    queryKey: ['sincronizacao'],
    queryFn: api.sincronizacao,
    enabled: hibrido,
    refetchInterval: (q) => (q.state.data?.tarefas.some((t) => t.tipo === 'enviar' && t.estado !== 'erro') ? 1000 : 5000),
  })

  // O uploader consulta isto a cada 200 ms: o kill switch chega pelo SSE e
  // aborta as partes em voo.
  const hibridoRef = useRef(hibrido)
  hibridoRef.current = hibrido

  const destino = libId || libraries?.[0]?.id || 0
  const muda = (chave: string, patch: Partial<Envio>) =>
    setEnvios((lista) => lista.map((e) => (e.chave === chave ? { ...e, ...patch } : e)))

  const roda = async (envio: Envio) => {
    muda(envio.chave, { estado: 'enviando', erro: undefined })
    try {
      let uploadId = envio.uploadId
      let upload
      if (uploadId === undefined) {
        upload = await criarEnvio(envio.arquivo, destino)
        uploadId = upload.id
        muda(envio.chave, { uploadId })
      } else {
        upload = (await api.partesDoUpload(uploadId)).upload
      }
      await enviar(envio.arquivo, upload, () => hibridoRef.current, (p) =>
        muda(envio.chave, { enviados: p.enviados }),
      )
      muda(envio.chave, { estado: 'pronto', enviados: envio.arquivo.size })
      void queryClient.invalidateQueries()
    } catch (e) {
      if (e instanceof PausadoPeloModo || !hibridoRef.current) muda(envio.chave, { estado: 'pausado' })
      else muda(envio.chave, { estado: 'erro', erro: (e as Error).message })
    }
  }

  // Voltou o híbrido: retoma o que o kill switch pausou.
  const enviosRef = useRef(envios)
  enviosRef.current = envios
  useEffect(() => {
    if (!hibrido) return
    for (const e of enviosRef.current) if (e.estado === 'pausado') void roda(e)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hibrido])

  // Nada subindo = lote novo; com algo em curso, os novos entram no mesmo.
  // Vale para os dois lados: um envio do Mac no meio de um do navegador vai
  // para a mesma nuvem.
  const doMacRef = useRef(doMac)
  doMacRef.current = doMac
  const proximoLote = () => {
    const todos = [...enviosRef.current, ...Object.values(doMacRef.current)]
    const ultimo = todos.reduce((n, e) => Math.max(n, e.lote), 0)
    const emCurso = todos.some((e) => e.estado === 'enviando' || e.estado === 'pausado')
    return emCurso ? ultimo : ultimo + 1
  }

  useEffect(() => {
    if (!sinc) return
    const tarefas = sinc.tarefas.filter((t) => t.tipo === 'enviar')
    setDoMac((atual) => {
      const novo = { ...atual }
      let mudou = false
      for (const t of tarefas) {
        const estado: Envio['estado'] = t.estado === 'erro' ? 'erro' : t.estado === 'pausado' ? 'pausado' : 'enviando'
        const antes = novo[t.file_id]
        const reabriu = antes && (antes.estado === 'pronto' || antes.estado === 'erro') && estado === 'enviando'
        const lote = antes && !reabriu ? antes.lote : proximoLote()
        novo[t.file_id] = { nome: t.nome, total: t.total, feitos: t.feitos, estado, erro: t.erro, lote }
        mudou = true
      }
      // Sumiu da lista do motor sem erro: terminou.
      for (const [id, e] of Object.entries(novo)) {
        if ((e.estado === 'enviando' || e.estado === 'pausado') && !tarefas.some((t) => t.file_id === Number(id))) {
          novo[Number(id)] = { ...e, estado: 'pronto', feitos: e.total }
          mudou = true
          void queryClient.invalidateQueries({ queryKey: ['title'] })
        }
      }
      return mudou ? novo : atual
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sinc])

  const linhas: Linha[] = [
    ...Object.entries(doMac).map(([id, e]) => ({
      chave: `mac-${id}`,
      nome: e.nome,
      total: e.total,
      feitos: e.feitos,
      estado: e.estado,
      erro: e.erro,
      lote: e.lote,
      doMac: true,
      tentar: () => void api.acaoDeNuvem(Number(id), 'enviar').then(() => queryClient.invalidateQueries({ queryKey: ['sincronizacao'] })),
    })),
    ...envios.map((e) => ({
      chave: e.chave,
      nome: e.arquivo.name,
      total: e.arquivo.size,
      feitos: e.enviados,
      estado: e.estado,
      erro: e.erro,
      lote: e.lote,
      doMac: false,
      tentar: () => void roda(e),
    })),
  ].sort((a, b) => b.lote - a.lote)
  const velocidades = useVelocidades(linhas)

  const adiciona = (arquivos: FileList | null) => {
    if (!arquivos || !destino) return
    const lote = proximoLote()
    const novos = [...arquivos].map<Envio>((arquivo) => ({
      chave: `${arquivo.name}-${arquivo.size}-${Math.random()}`,
      arquivo,
      enviados: 0,
      estado: hibridoRef.current ? 'enviando' : 'pausado',
      lote,
    }))
    setEnvios((lista) => [...novos, ...lista])
    if (hibridoRef.current) for (const e of novos) void roda(e)
  }

  return (
    <Card
      title="Enviar para a nuvem"
      description="O navegador manda o arquivo direto ao bucket, sem passar pelo Mac, e o item entra no catálogo como “na nuvem”. Os envios pedidos na página de um título (“Enviar à nuvem”) também aparecem aqui. Se o modo voltar a local, tudo pausa e retoma quando o híbrido voltar."
    >
      <label className="mb-3 flex items-center gap-2 text-xs text-muted">
        Biblioteca
        <select
          value={destino}
          onChange={(e) => setLibId(Number(e.target.value))}
          className="rounded-lg border border-line bg-bg px-2 py-1.5 text-sm text-ink outline-none focus:border-accent"
        >
          {(libraries ?? []).map((lib) => (
            <option key={lib.id} value={lib.id}>
              {lib.name}
            </option>
          ))}
        </select>
      </label>

      <label
        onDragOver={(e) => {
          e.preventDefault()
          setArrastando(true)
        }}
        onDragLeave={() => setArrastando(false)}
        onDrop={(e) => {
          e.preventDefault()
          setArrastando(false)
          if (hibrido) adiciona(e.dataTransfer.files)
        }}
        className={[
          'flex flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed px-4 py-8 text-center text-sm transition',
          hibrido ? 'cursor-pointer' : 'cursor-not-allowed opacity-60',
          arrastando ? 'border-accent bg-accent/10' : 'border-line hover:border-accent/60',
        ].join(' ')}
      >
        <UploadIcon className="text-accent" width="1.75em" height="1.75em" />
        <span className="font-medium">
          {hibrido ? 'Solte arquivos aqui ou clique para escolher' : 'Ligue o híbrido para enviar'}
        </span>
        <span className="text-xs text-muted">Filmes, episódios, músicas e fotos</span>
        <input
          type="file"
          multiple
          disabled={!hibrido || !destino}
          className="sr-only"
          onChange={(e) => {
            adiciona(e.target.files)
            e.target.value = ''
          }}
        />
      </label>

      {linhas.length > 0 && <LoteDeEnvio linhas={linhas} velocidades={velocidades} />}

      {linhas.length > 0 && (
        <ul className="mt-4 divide-y divide-line overflow-hidden rounded-lg border border-line">
          {linhas.map((e) => {
            const pct = e.total > 0 ? Math.round((e.feitos / e.total) * 100) : 0
            const bps = velocidades.get(e.chave) ?? 0
            const falta = restante(bps > 0 ? (e.total - e.feitos) / bps : 0)
            return (
              <li key={e.chave} className="px-3 py-2.5">
                <div className="flex items-center justify-between gap-3 text-sm">
                  <span className="flex min-w-0 items-center gap-2">
                    <NuvemIcon
                      key={e.estado}
                      estado={e.estado === 'pronto' ? 'concluido' : e.estado === 'erro' ? 'erro' : e.estado === 'pausado' ? 'local' : 'enviando'}
                      className={e.estado === 'erro' ? 'shrink-0 text-danger' : e.estado === 'pronto' ? 'shrink-0 text-ok' : 'shrink-0 text-accent'}
                    />
                    <span className="line-clamp-1 font-medium">{e.nome}</span>
                    {e.doMac && (
                      <span className="shrink-0 rounded bg-elev px-1.5 py-0.5 font-mono text-[10px] tracking-wide text-muted uppercase">
                        do Mac
                      </span>
                    )}
                  </span>
                  <span
                    className={[
                      'shrink-0 text-xs',
                      e.estado === 'erro' ? 'text-danger' : e.estado === 'pronto' ? 'text-ok' : 'text-muted',
                    ].join(' ')}
                  >
                    {e.estado === 'pronto'
                      ? e.doMac
                        ? 'no Mac e na nuvem'
                        : 'na nuvem'
                      : e.estado === 'pausado'
                        ? `pausado · ${pct}%`
                        : e.estado === 'erro'
                          ? 'falhou'
                          : `${pct}% de ${humanSize(e.total)}${falta ? ` · faltam ${falta} · ${mbps(bps)}` : ''}`}
                  </span>
                </div>
                <div className="mt-1.5 h-1 overflow-hidden rounded bg-elev">
                  <div
                    className={`h-full transition-[width] ${e.estado === 'pronto' ? 'bg-ok' : 'bg-accent'}`}
                    style={{ width: `${e.estado === 'pronto' ? 100 : pct}%` }}
                  />
                </div>
                {e.erro && (
                  <div className="mt-1.5 flex items-center justify-between gap-2 text-xs text-danger">
                    <span>{e.erro}</span>
                    {e.tentar && (
                      <button type="button" onClick={e.tentar} className="underline">
                        tentar de novo
                      </button>
                    )}
                  </div>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </Card>
  )
}

function iconeDaTarefa(tipo: string, estado: string): EstadoNuvem {
  if (estado === 'erro') return 'erro'
  if (estado === 'pausado') return 'local'
  if (estado === 'fila') return 'processando'
  return tipo === 'fixar' ? 'baixando' : tipo === 'enviar' ? 'enviando' : 'sincronizando'
}

const rotuloDaTarefa = { enviar: 'enviando', fixar: 'baixando', liberar: 'liberando', remover: 'removendo' }

function SincronizacaoCard() {
  const queryClient = useQueryClient()
  const nuvem = useModoNuvem()
  const { data } = useQuery({
    queryKey: ['sincronizacao'],
    queryFn: api.sincronizacao,
    enabled: !!nuvem?.configurada,
    // Só pergunta rápido enquanto há trabalho; parado, o painel é estático.
    refetchInterval: (q) =>
      q.state.data?.reconciliando || q.state.data?.tarefas.some((t) => t.estado !== 'erro') ? 1500 : 15000,
  })
  const reconciliar = useMutation({
    mutationFn: api.reconciliar,
    onSettled: () => void queryClient.invalidateQueries({ queryKey: ['sincronizacao'] }),
  })
  if (!nuvem?.configurada) return null
  const hibrido = nuvem.modo === 'hibrido'

  return (
    <Card
      title="Sincronização"
      description="Ao entrar no híbrido, o servidor manda para o bucket o que mudou no modo local, traz ao catálogo o que está no bucket e não aparece aqui, e retoma envios e downloads interrompidos. Repete a cada 15 minutos."
    >
      <div className="flex flex-wrap items-center gap-3">
        <button
          type="button"
          onClick={() => reconciliar.mutate()}
          disabled={!hibrido || data?.reconciliando || reconciliar.isPending}
          className="inline-flex items-center gap-2 rounded-lg border border-line px-3.5 py-2 text-sm font-medium transition hover:bg-elev disabled:opacity-50"
        >
          {data?.reconciliando ? <LuaSpinner className="text-ink" /> : <RefreshIcon />}
          {data?.reconciliando ? 'Sincronizando…' : 'Sincronizar agora'}
        </button>
        <p className="text-xs text-muted">
          {data?.eventos_pendentes
            ? `${data.eventos_pendentes} mudanças esperando o híbrido · `
            : ''}
          {data?.ultima_reconciliacao
            ? `última: ${new Date(data.ultima_reconciliacao).toLocaleString('pt-BR')}`
            : 'nunca sincronizado'}
        </p>
      </div>

      {data?.erro && (
        <p className="mt-3 rounded-lg border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger">
          {data.erro}
        </p>
      )}

      {data?.energia && !nuvem.papel?.startsWith('nuvem') && (
        <p className="mt-3 text-xs text-muted">
          {data.energia.motivo
            ? `Mac ${data.energia.motivo}${data.energia.carga ? ` (${data.energia.carga}%)` : ''}: preparos de arquivos que também estão no bucket vão para os workers da nuvem.`
            : 'Mac na tomada e frio: preparos rodam aqui, com o VideoToolbox.'}
        </p>
      )}

      {data && data.tarefas.length > 0 && (
        <ul className="mt-4 divide-y divide-line overflow-hidden rounded-lg border border-line">
          {data.tarefas.map((t) => {
            const pct = t.total > 0 ? Math.round((t.feitos / t.total) * 100) : 0
            return (
              <li key={`${t.tipo}-${t.file_id}`} className="px-3 py-2.5">
                <div className="flex items-center justify-between gap-3 text-sm">
                  <span className="flex min-w-0 items-center gap-2">
                    <NuvemIcon key={t.estado} estado={iconeDaTarefa(t.tipo, t.estado)} className={t.estado === 'erro' ? 'shrink-0 text-danger' : 'shrink-0 text-accent'} />
                    <span className="line-clamp-1 font-medium">{t.nome}</span>
                  </span>
                  <span className={`shrink-0 text-xs ${t.estado === 'erro' ? 'text-danger' : 'text-muted'}`}>
                    {t.estado === 'erro'
                      ? 'falhou'
                      : t.estado === 'pausado'
                        ? 'pausado'
                        : t.estado === 'fila'
                          ? 'na fila'
                          : `${rotuloDaTarefa[t.tipo]} ${t.total ? `${pct}% de ${humanSize(t.total)}` : ''}`}
                  </span>
                </div>
                {t.total > 0 && t.estado !== 'erro' && (
                  <div className="mt-1.5 h-1 overflow-hidden rounded bg-elev">
                    <div className="h-full bg-accent transition-[width]" style={{ width: `${pct}%` }} />
                  </div>
                )}
                {t.erro && <p className="mt-1 text-xs text-danger">{t.erro}</p>}
              </li>
            )
          })}
        </ul>
      )}
    </Card>
  )
}

function LibrariesCard() {
  const queryClient = useQueryClient()
  const { data: libraries } = useQuery({ queryKey: ['libraries'], queryFn: api.libraries })
  const nuvem = useModoNuvem()
  const espelho = useMutation({
    mutationFn: ({ id, on }: { id: number; on: boolean }) => api.setEspelhada(id, on),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: ['libraries'] }),
  })

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
            <div className="flex shrink-0 items-center gap-3">
              {nuvem?.configurada && (
                <label
                  className="flex items-center gap-1.5 text-[11px] text-muted"
                  title="No híbrido, todo arquivo desta biblioteca ganha uma cópia na nuvem"
                >
                  <input
                    type="checkbox"
                    checked={!!lib.espelhada}
                    disabled={espelho.isPending}
                    onChange={(e) => espelho.mutate({ id: lib.id, on: e.target.checked })}
                    className="accent-[var(--color-accent)]"
                  />
                  espelhar
                </label>
              )}
              <span className="rounded-md bg-elev px-2 py-0.5 text-[11px] text-muted">
                {kindLabel[lib.kind] ?? lib.kind}
              </span>
            </div>
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
        <p className="mt-3 rounded-lg border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger">
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
          <span className="text-ok">Chave configurada.</span>
        ) : (
          <span className="text-muted">
            Nenhuma chave. Crie a sua em themoviedb.org → Configurações → API.
          </span>
        )}
        {settings && !settings.ffmpeg && (
          <span className="ml-2 text-warn">ffmpeg ausente: sem capas geradas localmente.</span>
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
          <p role="alert" className="text-xs text-danger">
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
          <p role="alert" className="text-xs text-danger">
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
