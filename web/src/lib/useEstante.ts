// A Estante ao vivo: o servidor só sonda o PS5 e a TV enquanto este SSE está
// aberto, então o hook vive na página da Estante, não no layout.
import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type EstadoEstante } from './api'
import { useModoNuvem } from './nuvem'

const CHAVE = ['estante']

/** A navegação decide o link pelo papel do servidor, que já está em cache:
 *  pedir /api/estante aqui faria o Mac sondar a rede a cada app aberto. */
export function useEstanteDisponivel(): boolean {
  const modo = useModoNuvem()
  return !!modo && modo.papel !== 'nuvem'
}

export function useEstante() {
  const queryClient = useQueryClient()
  // Sem SSE (navegador antigo, proxy que corta), cai para polling.
  const [polling, setPolling] = useState(!('EventSource' in window))
  const query = useQuery({ queryKey: CHAVE, queryFn: api.estante, refetchInterval: polling ? 5000 : false })

  useEffect(() => {
    if (polling || query.data?.disponivel === false) return
    const fonte = new EventSource(api.estanteEventosUrl())
    fonte.onmessage = (ev) => {
      try {
        queryClient.setQueryData<EstadoEstante>(CHAVE, JSON.parse(ev.data) as EstadoEstante)
      } catch {
        // mensagem malformada: espera a próxima
      }
    }
    fonte.onerror = () => {
      // O EventSource reconecta sozinho; só desiste de vez se ele fechar.
      if (fonte.readyState === EventSource.CLOSED) setPolling(true)
    }
    return () => fonte.close()
  }, [polling, query.data?.disponivel, queryClient])

  return query
}

export function guardarEstante(queryClient: ReturnType<typeof useQueryClient>, e: EstadoEstante) {
  queryClient.setQueryData<EstadoEstante>(CHAVE, e)
}
