import { useEffect, useState } from 'react'
import { api, ApiError, type MetricsSnapshot } from './api'

/**
 * Acompanha a telemetria por SSE, com o mesmo desenho de useScanStatus: uma
 * conexão em vez de uma pergunta por segundo, e polling se o EventSource
 * falhar.
 *
 * O segundo valor devolvido é o erro de permissão: a rota é só de admin, e um
 * 403 precisa virar mensagem na tela em vez de painel eternamente vazio.
 */
export function useMetrics(): { snap?: MetricsSnapshot; proibido: boolean } {
  const [snap, setSnap] = useState<MetricsSnapshot>()
  const [proibido, setProibido] = useState(false)

  useEffect(() => {
    let poll: number | undefined
    let source: EventSource | undefined
    let vivo = true

    const startPolling = () => {
      if (poll || !vivo) return
      const tick = () =>
        void api
          .metrics()
          .then((m) => setSnap(m))
          .catch((e) => {
            if (e instanceof ApiError && (e.status === 403 || e.status === 401)) {
              setProibido(true)
              // Insistir num 403 a cada 2s só enche o log do servidor.
              if (poll) window.clearInterval(poll)
              poll = undefined
            }
          })
      tick()
      poll = window.setInterval(tick, 2000)
    }

    if ('EventSource' in window) {
      source = new EventSource(api.metricsEventsUrl())
      source.onmessage = (event) => {
        try {
          setSnap(JSON.parse(event.data) as MetricsSnapshot)
        } catch {
          // mensagem malformada: ignora e espera a próxima
        }
      }
      // O EventSource não expõe o status HTTP do erro, então a queda cai no
      // polling — que aí sim descobre se era 403 ou rede.
      source.onerror = () => {
        source?.close()
        source = undefined
        startPolling()
      }
    } else {
      startPolling()
    }

    return () => {
      vivo = false
      source?.close()
      if (poll) window.clearInterval(poll)
    }
  }, [])

  return { snap, proibido }
}
