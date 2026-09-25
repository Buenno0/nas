import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import { useEnvios } from '../lib/envios'
import { useModoNuvem } from '../lib/nuvem'
import { UploadIcon } from './icons'

/**
 * Atalho de envio para a nuvem, fora de Configurações: escolhe a biblioteca
 * (quando há mais de uma) e abre o seletor de arquivos. O envio em si é o
 * mesmo de Configurações (lib/envios), então aparece no cartão flutuante.
 * Só para admin, e só com a nuvem ligada.
 */
export function EnviarAtalho({ variante }: { variante: 'menu' | 'icone' }) {
  const { data: user } = useQuery({ queryKey: ['me'], queryFn: api.me })
  const { data: libraries } = useQuery({ queryKey: ['libraries'], queryFn: api.libraries })
  const estado = useModoNuvem()
  const { adiciona } = useEnvios()
  const entrada = useRef<HTMLInputElement>(null)
  const caixa = useRef<HTMLDivElement>(null)
  const [destino, setDestino] = useState(0)
  const [aberto, setAberto] = useState(false)

  // Fecha a lista de bibliotecas ao clicar fora ou apertar Esc.
  useEffect(() => {
    if (!aberto) return
    const fora = (e: MouseEvent) => {
      if (!caixa.current?.contains(e.target as Node)) setAberto(false)
    }
    const esc = (e: KeyboardEvent) => e.key === 'Escape' && setAberto(false)
    document.addEventListener('mousedown', fora)
    document.addEventListener('keydown', esc)
    return () => {
      document.removeEventListener('mousedown', fora)
      document.removeEventListener('keydown', esc)
    }
  }, [aberto])

  const hibrido = estado?.modo === 'hibrido'
  if (!user?.is_admin || !hibrido || !libraries?.length) return null

  const escolher = (id: number) => {
    setDestino(id)
    setAberto(false)
    // O clique precisa sair no mesmo gesto do usuário, senão o navegador
    // bloqueia o seletor de arquivos.
    entrada.current?.click()
  }
  const clicar = () => (libraries.length === 1 ? escolher(libraries[0].id) : setAberto((a) => !a))

  return (
    <div ref={caixa} className="relative">
      {variante === 'menu' ? (
        <button
          type="button"
          onClick={clicar}
          aria-expanded={aberto}
          className="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium text-muted transition hover:bg-elev/60 hover:text-ink"
        >
          <UploadIcon /> Enviar à nuvem
        </button>
      ) : (
        <button
          type="button"
          onClick={clicar}
          aria-expanded={aberto}
          aria-label="Enviar à nuvem"
          title="Enviar à nuvem"
          className="grid h-9 w-9 shrink-0 place-items-center rounded-lg text-muted transition hover:bg-elev hover:text-ink"
        >
          <UploadIcon />
        </button>
      )}

      {aberto && (
        <div
          role="menu"
          className={[
            'absolute z-50 w-56 overflow-hidden rounded-xl border border-line bg-surface p-1 shadow-2xl shadow-black/40',
            variante === 'menu' ? 'bottom-full left-0 mb-1' : 'top-full right-0 mt-1',
          ].join(' ')}
        >
          <p className="px-3 pt-2 pb-1 text-[11px] tracking-wide text-muted uppercase">Para qual biblioteca?</p>
          {libraries.map((lib) => (
            <button
              key={lib.id}
              type="button"
              role="menuitem"
              onClick={() => escolher(lib.id)}
              className="block w-full truncate rounded-lg px-3 py-2 text-left text-sm transition hover:bg-elev"
            >
              {lib.name}
            </button>
          ))}
        </div>
      )}

      <input
        ref={entrada}
        type="file"
        multiple
        className="sr-only"
        tabIndex={-1}
        aria-hidden="true"
        onChange={(e) => {
          if (e.target.files?.length && destino) adiciona(e.target.files, destino)
          e.target.value = ''
        }}
      />
    </div>
  )
}
