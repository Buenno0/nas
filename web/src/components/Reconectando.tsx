// Faixa discreta no topo enquanto o servidor está fora por instantes (troca
// de task num deploy): as leituras seguem tentando sozinhas por baixo.
import { useEffect, useState } from 'react'

export function Reconectando() {
  const [ativo, setAtivo] = useState(false)
  useEffect(() => {
    const aoMudar = (e: Event) => setAtivo((e as CustomEvent<boolean>).detail)
    window.addEventListener('ozy:reconectando', aoMudar)
    return () => window.removeEventListener('ozy:reconectando', aoMudar)
  }, [])
  if (!ativo) return null
  return (
    <div role="status" aria-live="polite" className="pointer-events-none fixed inset-x-0 top-3 z-50 flex justify-center">
      <p className="flex items-center gap-2 rounded-full bg-black/80 px-4 py-2 text-sm text-white shadow-lg backdrop-blur-sm">
        <span className="h-3 w-3 animate-spin rounded-full border-2 border-white/30 border-t-white" />
        Reconectando…
      </p>
    </div>
  )
}
