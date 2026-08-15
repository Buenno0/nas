import { useEffect, useState } from 'react'
import { api, type ScanStatus } from './api'

/**
 * Acompanha o scan por SSE, que é uma conexão só em vez de uma pergunta por
 * segundo. Se o navegador ou a rede não colaborarem, cai para polling.
 */
export function useScanStatus(): ScanStatus | undefined {
  const [status, setStatus] = useState<ScanStatus>()

  useEffect(() => {
    let poll: number | undefined
    let source: EventSource | undefined

    const startPolling = () => {
      if (poll) return
      const tick = () => void api.scanStatus().then(setStatus).catch(() => {})
      tick()
      poll = window.setInterval(tick, 2000)
    }

    if ('EventSource' in window) {
      source = new EventSource('/api/scan/events')
      source.onmessage = (event) => {
        try {
          setStatus(JSON.parse(event.data) as ScanStatus)
        } catch {
          // mensagem malformada: ignora e espera a próxima
        }
      }
      source.onerror = () => {
        source?.close()
        source = undefined
        startPolling()
      }
    } else {
      startPolling()
    }

    return () => {
      source?.close()
      if (poll) window.clearInterval(poll)
    }
  }, [])

  return status
}
