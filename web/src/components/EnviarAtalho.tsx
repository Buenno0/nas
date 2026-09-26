import { NavLink } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import { useModoNuvem } from '../lib/nuvem'
import { UploadIcon } from './icons'

/**
 * Atalho para a tela de envio (/enviar), fora de Configurações. Só para admin
 * e só com a nuvem ligada; o card de Configurações continua existindo.
 */
export function EnviarAtalho({ variante }: { variante: 'menu' | 'icone' }) {
  const { data: user } = useQuery({ queryKey: ['me'], queryFn: api.me })
  const estado = useModoNuvem()
  if (!user?.is_admin || estado?.modo !== 'hibrido') return null

  if (variante === 'icone') {
    return (
      <NavLink
        to="/enviar"
        aria-label="Enviar à nuvem"
        title="Enviar à nuvem"
        className="grid h-9 w-9 shrink-0 place-items-center rounded-lg text-muted transition hover:bg-elev hover:text-ink"
      >
        <UploadIcon />
      </NavLink>
    )
  }
  return (
    <NavLink
      to="/enviar"
      className={({ isActive }) =>
        [
          'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition',
          isActive ? 'bg-elev text-ink' : 'text-muted hover:bg-elev/60 hover:text-ink',
        ].join(' ')
      }
    >
      <UploadIcon /> Enviar à nuvem
    </NavLink>
  )
}
