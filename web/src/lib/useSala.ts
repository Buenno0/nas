// "Assistir junto": mantém este <video> no mesmo ponto que o resto da sala.
// A sala manda o estado (tocando, posição, instante); aqui se corrige a
// diferença — pequena, mexendo na velocidade; grande, pulando.
import { useCallback, useEffect, useRef, useState, type RefObject } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type ComandoDaSala, type EstadoDaSala, type MensagemDaSala } from './api'

const PULA_ACIMA_S = 2
const AJUSTA_ACIMA_S = 0.3
const AJUSTE_DE_VELOCIDADE = 0.05
// Ignora os eventos do <video> causados por nós mesmos por este tempo.
const ECO_MS = 700
// Só avisa a sala que travou se durar isto (um seek curto também "trava").
const TRAVA_MS = 1500
const SAIDA_MS = 4000
// Arrastando a barra, só o ponto onde o dedo parou vai para a sala.
const PULO_MS = 350
// Esta aba, para reconhecer os próprios comandos quando voltam do servidor.
const CLIENTE = Math.random().toString(36).slice(2)

export interface Reacao {
  id: number
  de: string
  emoji: string
}

export function useSala(codigo: string | null, videoRef: RefObject<HTMLVideoElement | null>) {
  const { data: eu } = useQuery({ queryKey: ['me'], queryFn: api.me })
  const [estado, setEstado] = useState<EstadoDaSala>()
  const seq = useRef(0)
  const puloTimer = useRef<number | undefined>(undefined)
  const avisouPronto = useRef(false)
  const [reacoes, setReacoes] = useState<Reacao[]>([])
  const [erro, setErro] = useState<string>()
  const [bloqueado, setBloqueado] = useState(false)
  const [fim, setFim] = useState<string>()
  const [avisos, setAvisos] = useState<{ id: number; texto: string }[]>([])
  // Saída só vira aviso se durar: uma conexão que cai e volta não é "saiu".
  const saidas = useRef(new Map<string, number>())

  const estadoRef = useRef<EstadoDaSala>(undefined)
  // Diferença entre o relógio do servidor e o daqui (ms). Cada mensagem
  // chega atrasada pela rede, o que puxa a conta para baixo: fica a maior
  // já vista, a que menos sofreu com o atraso.
  const desvio = useRef<number | undefined>(undefined)
  const ecoAte = useRef(0)
  const travaTimer = useRef<number | undefined>(undefined)
  const avisouTrava = useRef(false)
  // Até alcançar o ponto da sala, nada do que este <video> faz vira comando:
  // era assim que quem entrava pelo link (tocando do 0:00) puxava todo mundo
  // de volta ao começo.
  const sincronizado = useRef(false)

  const avisar = useCallback((texto: string) => {
    const id = Math.random()
    setAvisos((a) => [...a.slice(-3), { id, texto }])
    window.setTimeout(() => setAvisos((a) => a.filter((x) => x.id !== id)), 4000)
  }, [])

  const enviar = useCallback(
    (c: ComandoDaSala) => {
      if (!codigo) return
      if (c.tipo === 'play' || c.tipo === 'pause' || c.tipo === 'seek') {
        c = { ...c, cliente: CLIENTE, seq: seq.current }
      }
      void api.comandoDaSala(codigo, c).catch((e: Error) => setErro(e.message))
    },
    [codigo],
  )

  const alvo = useCallback(() => {
    const e = estadoRef.current
    if (!e) return 0
    if (!e.tocando) return e.posicao
    const agoraServidor = Date.now() + (desvio.current ?? 0)
    return e.posicao + Math.max(0, agoraServidor - e.em) / 1000
  }, [])

  const alinhar = useCallback(() => {
    const video = videoRef.current
    const e = estadoRef.current
    // Pulando (pelo espectador ou por nós), currentTime ainda não assentou.
    if (!video || !e || video.readyState < 1 || video.seeking) return
    const onde = alvo()
    const dif = onde - video.currentTime
    if (Math.abs(dif) < 1 && video.paused === !e.tocando) sincronizado.current = true
    if (!e.tocando) {
      // A sala está esperando por nós (depois de um pulo ou travada): avisa
      // quando der para tocar do ponto novo sem engasgar.
      const esperando = !!eu && (e.aguardando ?? []).includes(eu.username)
      if (!esperando) avisouPronto.current = false
      else if (!avisouPronto.current && Math.abs(dif) < 0.5 && video.readyState >= 3) {
        avisouPronto.current = true
        enviar({ tipo: 'pronto' })
      }
      if (!video.paused) {
        ecoAte.current = Date.now() + ECO_MS
        video.pause()
      }
      if (Math.abs(dif) > 0.5) {
        ecoAte.current = Date.now() + ECO_MS
        video.currentTime = onde
      }
      video.playbackRate = 1
      return
    }
    if (Math.abs(dif) > PULA_ACIMA_S) {
      ecoAte.current = Date.now() + ECO_MS
      video.currentTime = onde
      video.playbackRate = 1
    } else if (Math.abs(dif) > AJUSTA_ACIMA_S) {
      video.playbackRate = 1 + Math.sign(dif) * AJUSTE_DE_VELOCIDADE
    } else {
      video.playbackRate = 1
    }
    if (video.paused) {
      ecoAte.current = Date.now() + ECO_MS
      video.play().then(
        () => setBloqueado(false),
        // O navegador só deixa tocar com som depois de um toque na página.
        () => setBloqueado(true),
      )
    }
  }, [alvo, enviar, eu, videoRef])

  useEffect(() => {
    if (!codigo || !('EventSource' in window)) return
    const fonte = new EventSource(api.eventosDaSalaUrl(codigo))
    fonte.onmessage = (ev) => {
      const m = JSON.parse(ev.data as string) as MensagemDaSala
      const d = m.agora - Date.now()
      if (desvio.current === undefined || d > desvio.current) desvio.current = d
      if (m.tipo === 'fim') {
        fonte.close()
        setFim(m.de)
        return
      }
      if (m.tipo === 'estado') {
        if (estadoRef.current?.file_id !== m.estado.file_id) sincronizado.current = false
        const antes = estadoRef.current?.presenca
        if (antes) {
          for (const nome of m.estado.presenca) {
            if (antes.includes(nome)) continue
            const pendente = saidas.current.get(nome)
            if (pendente) {
              window.clearTimeout(pendente)
              saidas.current.delete(nome)
            } else avisar(`${nome} entrou na sala`)
          }
          for (const nome of antes) {
            if (m.estado.presenca.includes(nome) || saidas.current.has(nome)) continue
            saidas.current.set(
              nome,
              window.setTimeout(() => {
                saidas.current.delete(nome)
                avisar(`${nome} saiu da sala`)
              }, SAIDA_MS),
            )
          }
        }
        // Eco de um comando nosso mais velho que o último que já aplicamos:
        // vale a presença, não o ponto (era ele que puxava o pulo de volta).
        const eco = m.estado.cliente === CLIENTE && (m.estado.seq ?? 0) < seq.current
        const atual = estadoRef.current
        estadoRef.current =
          eco && atual
            ? { ...m.estado, tocando: atual.tocando, posicao: atual.posicao, em: atual.em }
            : m.estado
        setEstado(m.estado)
        setErro(undefined)
        alinhar()
      } else {
        const id = Math.random()
        setReacoes((r) => [...r.slice(-12), { id, de: m.de, emoji: m.emoji }])
        window.setTimeout(() => setReacoes((r) => r.filter((x) => x.id !== id)), 4000)
      }
    }
    fonte.onerror = () => {
      // O EventSource reconecta sozinho; se a sala sumiu, ele desiste.
      if (fonte.readyState === EventSource.CLOSED) setErro('A sala foi encerrada.')
    }
    const relogio = window.setInterval(alinhar, 1000)
    return () => {
      fonte.close()
      window.clearInterval(relogio)
      window.clearTimeout(travaTimer.current)
    }
  }, [codigo, alinhar, avisar])

  const nosso = () => !sincronizado.current || !estadoRef.current || Date.now() < ecoAte.current

  // O comando local vale na hora, sem esperar a volta do servidor: senão o
  // relógio de alinhamento, ainda com o estado velho, desfazia o pulo (ou a
  // pausa) no segundo seguinte.
  const assumir = useCallback((mudanca: Partial<EstadoDaSala>) => {
    const e = estadoRef.current
    if (!e) return
    // O número sobe já aqui, e não no envio: durante a espera do pulo
    // (PULO_MS) um eco do comando anterior também tem de ser ignorado.
    seq.current += 1
    estadoRef.current = { ...e, ...mudanca, em: Date.now() + (desvio.current ?? 0) }
  }, [])

  // Ganchos para os eventos do <video>: só o que o espectador fez vira comando.
  const aoTocar = useCallback(() => {
    const v = videoRef.current
    if (!codigo || !v || nosso()) return
    if (estadoRef.current?.tocando) return
    assumir({ tocando: true, posicao: v.currentTime })
    enviar({ tipo: 'play', posicao: v.currentTime })
  }, [assumir, codigo, enviar, videoRef])

  const aoPausar = useCallback(() => {
    const v = videoRef.current
    if (!codigo || !v || nosso() || v.ended) return
    if (!estadoRef.current?.tocando) return
    assumir({ tocando: false, posicao: v.currentTime })
    enviar({ tipo: 'pause', posicao: v.currentTime })
  }, [assumir, codigo, enviar, videoRef])

  const aoPular = useCallback(() => {
    const v = videoRef.current
    if (!codigo || !v || nosso()) return
    if (Math.abs(alvo() - v.currentTime) < 1) return
    assumir({ posicao: v.currentTime })
    window.clearTimeout(puloTimer.current)
    puloTimer.current = window.setTimeout(() => {
      const agora = videoRef.current
      if (agora) enviar({ tipo: 'seek', posicao: agora.currentTime })
    }, PULO_MS)
  }, [alvo, assumir, codigo, enviar, videoRef])

  const aoTravar = useCallback(() => {
    if (!codigo) return
    window.clearTimeout(travaTimer.current)
    travaTimer.current = window.setTimeout(() => {
      if (!estadoRef.current?.tocando) return
      avisouTrava.current = true
      enviar({ tipo: 'carregando' })
    }, TRAVA_MS)
  }, [codigo, enviar])

  const aoDestravar = useCallback(() => {
    window.clearTimeout(travaTimer.current)
    if (avisouTrava.current) {
      avisouTrava.current = false
      enviar({ tipo: 'pronto' })
    }
  }, [enviar])

  const entrar = useCallback(() => {
    setBloqueado(false)
    alinhar()
  }, [alinhar])

  return { estado, reacoes, avisos, erro, bloqueado, fim, enviar, entrar, alinhar, aoTocar, aoPausar, aoPular, aoTravar, aoDestravar }
}
