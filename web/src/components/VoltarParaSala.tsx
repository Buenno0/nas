// "Voltar para a sala": depois de recarregar ou fechar o player sem sair, a
// Home oferece a última sala — se ela ainda existir no servidor.
import { useState } from 'react'
import { Link } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import { esquecerSala, ultimaSala } from '../lib/useSala'

export function VoltarParaSala() {
  const [ultima, setUltima] = useState(ultimaSala)
  const { data: sala } = useQuery({
    queryKey: ['sala', ultima?.codigo],
    queryFn: () => api.sala(ultima!.codigo),
    enabled: !!ultima,
    retry: false,
  })
  if (!ultima || !sala) return null
  const dispensar = () => {
    esquecerSala()
    setUltima(undefined)
  }
  return (
    <div className="mx-4 flex flex-wrap items-center gap-3 rounded-2xl border border-accent/40 bg-accent/10 px-4 py-3 sm:mx-6">
      <p className="min-w-0 flex-1 text-sm">
        A sala <span className="font-mono font-semibold tracking-widest">{sala.codigo}</span> continua aberta
        {sala.presenca.length > 0 ? ` com ${sala.presenca.join(', ')}` : ''}.
      </p>
      <Link
        to={`/watch/${sala.file_id}?sala=${sala.codigo}`}
        className="rounded-full bg-accent px-4 py-1.5 text-sm font-semibold text-accent-ink transition hover:opacity-90"
      >
        Voltar para a sala
      </Link>
      <button type="button" onClick={dispensar} className="text-sm text-muted hover:text-ink">
        Agora não
      </button>
    </div>
  )
}
