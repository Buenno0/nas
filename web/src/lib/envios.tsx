// Estado dos envios para a nuvem, acima das rotas: um envio que começa em
// Configurações continua visível (e controlável) em qualquer tela, no cartão
// flutuante. Junta os dois lados:
//
//   - envios do navegador, direto ao bucket (lib/upload.ts);
//   - envios do Mac, que o motor faz a partir do disco quando alguém pede
//     "Enviar à nuvem" na página de um título (tarefas de /api/sincronizacao).
import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './api'
import { useModoNuvem } from './nuvem'
import { criarEnvio, enviar, PausadoPeloModo } from './upload'

export type EstadoDoEnvio = 'enviando' | 'pausado' | 'pronto' | 'erro'

interface Envio {
  chave: string
  arquivo: File
  libraryId: number
  uploadId?: number
  enviados: number
  estado: EstadoDoEnvio
  erro?: string
  lote: number
}

interface EnvioDoMac {
  nome: string
  total: number
  feitos: number
  estado: EstadoDoEnvio
  erro?: string
  lote: number
}

/** Um envio na lista, venha do navegador ou do Mac. */
export interface Linha {
  chave: string
  nome: string
  total: number
  feitos: number
  estado: EstadoDoEnvio
  erro?: string
  /** Envios soltos juntos (ou enquanto outros ainda sobem) formam um lote:
   *  é a unidade da nuvem que cresce e do raio no fim. */
  lote: number
  doMac: boolean
  tentar?: () => void
}

export interface ResumoDoLote {
  numero: number
  linhas: Linha[]
  total: number
  feito: number
  progresso: number
  ativo: boolean
  bps: number
  falta: string
}

interface ContextoDeEnvios {
  linhas: Linha[]
  velocidades: Map<string, number>
  lote: ResumoDoLote | null
  adiciona: (arquivos: FileList | File[], libraryId: number) => void
}

const Contexto = createContext<ContextoDeEnvios | null>(null)

export function useEnvios(): ContextoDeEnvios {
  const c = useContext(Contexto)
  if (!c) throw new Error('useEnvios fora do EnviosProvider')
  return c
}

/** "~3 min", "menos de 1 min", "~1 h 20 min". */
export function restante(segundos: number): string {
  if (!isFinite(segundos) || segundos <= 0) return ''
  if (segundos < 60) return 'menos de 1 min'
  const min = Math.round(segundos / 60)
  if (min < 60) return `~${min} min`
  return `~${Math.floor(min / 60)} h${min % 60 ? ` ${min % 60} min` : ''}`
}

export const mbps = (bps: number) => `${Math.max(1, Math.round((bps * 8) / 1e6))} Mbps`

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

export function EnviosProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const hibrido = useModoNuvem()?.modo === 'hibrido'
  const [envios, setEnvios] = useState<Envio[]>([])
  const [doMac, setDoMac] = useState<Record<number, EnvioDoMac>>({})

  // Envios do motor. A mesma consulta do painel de sincronização; rápida só
  // enquanto há envio em curso.
  const { data: sinc } = useQuery({
    queryKey: ['sincronizacao'],
    queryFn: api.sincronizacao,
    enabled: hibrido,
    refetchInterval: (q) =>
      q.state.data?.tarefas.some((t) => t.tipo === 'enviar' && t.estado !== 'erro') ? 1000 : 5000,
    // Com a aba em segundo plano o React Query pausa o intervalo; um envio
    // do Mac que terminasse assim só apareceria ao voltar. A consulta é
    // barata e o intervalo cai para 5 s quando nada está subindo.
    refetchIntervalInBackground: true,
  })

  // O uploader consulta isto a cada 200 ms: o kill switch chega pelo SSE e
  // aborta as partes em voo.
  const hibridoRef = useRef(hibrido)
  hibridoRef.current = hibrido
  const enviosRef = useRef(envios)
  enviosRef.current = envios
  const doMacRef = useRef(doMac)
  doMacRef.current = doMac

  const muda = (chave: string, patch: Partial<Envio>) =>
    setEnvios((lista) => lista.map((e) => (e.chave === chave ? { ...e, ...patch } : e)))

  const roda = async (envio: Envio) => {
    muda(envio.chave, { estado: 'enviando', erro: undefined })
    try {
      let upload
      if (envio.uploadId === undefined) {
        upload = await criarEnvio(envio.arquivo, envio.libraryId)
        muda(envio.chave, { uploadId: upload.id })
        envio = { ...envio, uploadId: upload.id }
      } else {
        upload = (await api.partesDoUpload(envio.uploadId)).upload
      }
      await enviar(envio.arquivo, upload, () => hibridoRef.current, (p) => muda(envio.chave, { enviados: p.enviados }))
      muda(envio.chave, { estado: 'pronto', enviados: envio.arquivo.size })
      void queryClient.invalidateQueries()
    } catch (e) {
      if (e instanceof PausadoPeloModo || !hibridoRef.current) muda(envio.chave, { estado: 'pausado' })
      else muda(envio.chave, { estado: 'erro', erro: (e as Error).message })
    }
  }

  // Voltou o híbrido: retoma o que o kill switch pausou.
  useEffect(() => {
    if (!hibrido) return
    for (const e of enviosRef.current) if (e.estado === 'pausado') void roda(e)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hibrido])

  // Nada subindo = lote novo; com algo em curso, os novos entram no mesmo.
  // Vale para os dois lados: um envio do Mac no meio de um do navegador vai
  // para a mesma nuvem.
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
        const estado: EstadoDoEnvio = t.estado === 'erro' ? 'erro' : t.estado === 'pausado' ? 'pausado' : 'enviando'
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
      tentar: () =>
        void api.acaoDeNuvem(Number(id), 'enviar').then(() => queryClient.invalidateQueries({ queryKey: ['sincronizacao'] })),
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

  // O lote mais recente: o progresso somado das linhas dele que não falharam.
  const numero = linhas.reduce((n, e) => Math.max(n, e.lote), 0)
  const doLote = linhas.filter((e) => e.lote === numero && e.estado !== 'erro')
  let lote: ResumoDoLote | null = null
  if (doLote.length > 0) {
    const total = doLote.reduce((n, e) => n + e.total, 0)
    const feito = doLote.reduce((n, e) => n + (e.estado === 'pronto' ? e.total : e.feitos), 0)
    // Previsão: o que falta sobre a soma das velocidades dos envios em curso
    // (eles sobem em paralelo e dividem o link).
    const bps = doLote.reduce((n, e) => n + (velocidades.get(e.chave) ?? 0), 0)
    lote = {
      numero,
      linhas: doLote,
      total,
      feito,
      progresso: total > 0 ? feito / total : 0,
      ativo: doLote.some((e) => e.estado === 'enviando'),
      bps,
      falta: restante(bps > 0 ? (total - feito) / bps : 0),
    }
  }

  const adiciona = (arquivos: FileList | File[], libraryId: number) => {
    if (!libraryId) return
    const numeroDoLote = proximoLote()
    const novos = [...arquivos].map<Envio>((arquivo) => ({
      chave: `${arquivo.name}-${arquivo.size}-${Math.random()}`,
      arquivo,
      libraryId,
      enviados: 0,
      estado: hibridoRef.current ? 'enviando' : 'pausado',
      lote: numeroDoLote,
    }))
    setEnvios((lista) => [...novos, ...lista])
    if (hibridoRef.current) for (const e of novos) void roda(e)
  }

  return <Contexto.Provider value={{ linhas, velocidades, lote, adiciona }}>{children}</Contexto.Provider>
}
