// Estado do modo de nuvem, ao vivo. Uma única conexão SSE por aba, aberta pelo
// Layout; o resto do app só lê o cache do react-query.
import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, soNaNuvem, type EstadoNuvem, type Localizacao, type TitleCard } from './api'

export const chaveModo = ['modo'] as const

export function useModoNuvem(): EstadoNuvem | undefined {
  const { data } = useQuery({ queryKey: chaveModo, queryFn: api.modo, staleTime: Infinity })
  return data
}

/** true só quando dá para falar com a nuvem agora. */
export function useHibrido(): boolean {
  return useModoNuvem()?.modo === 'hibrido'
}

/** O que toca aqui. No Mac, item só da nuvem precisa do híbrido; na
 *  instância cloud, item sem cópia no bucket mora só no Mac. */
export function useDisponibilidade() {
  const estado = useModoNuvem()
  const naNuvem = estado?.papel === 'nuvem'
  const hibrido = estado?.modo === 'hibrido'
  return {
    naNuvem,
    hibrido,
    tocaArquivo: (loc?: Localizacao) => (naNuvem ? soNaNuvem(loc) || loc === 'ambos' : !soNaNuvem(loc) || hibrido),
    tocaTitulo: (t: TitleCard) => (naNuvem ? !t.so_no_mac : !(t.so_na_nuvem && !hibrido)),
    /** Texto do "indisponível" deste nó. */
    ondeMora: naNuvem ? 'no Mac' : 'na nuvem',
  }
}

/** Mantém o cache em dia pelo SSE. O kill switch chega aqui em milissegundos,
 *  e é isso que pausa os uploads do navegador. */
export function useModoAoVivo() {
  const queryClient = useQueryClient()
  useEffect(() => {
    const fonte = new EventSource(api.modoEventsUrl())
    fonte.onmessage = (ev) => {
      try {
        const estado = JSON.parse(ev.data) as EstadoNuvem
        const antes = queryClient.getQueryData<EstadoNuvem>(chaveModo)?.modo
        queryClient.setQueryData(chaveModo, estado)
        // Itens da nuvem mudam de "indisponível" para tocável e vice-versa.
        if (antes && antes !== estado.modo) void queryClient.invalidateQueries({ queryKey: ['home'] })
      } catch {
        // evento malformado: espera o próximo
      }
    }
    return () => fonte.close()
  }, [queryClient])
}
