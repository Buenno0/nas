import { mbps, restante, type Linha } from '../lib/envios'
import { humanSize } from '../lib/format'
import { NuvemIcon } from './icons'

/** A lista de envios, a mesma em Configurações e no cartão flutuante. */
export function ListaDeEnvios({
  linhas,
  velocidades,
  compacta = false,
}: {
  linhas: Linha[]
  velocidades: Map<string, number>
  compacta?: boolean
}) {
  if (linhas.length === 0) return null
  return (
    <ul className={`divide-y divide-line overflow-hidden rounded-lg border border-line ${compacta ? 'text-xs' : ''}`}>
      {linhas.map((e) => {
        const pct = e.total > 0 ? Math.round((e.feitos / e.total) * 100) : 0
        const bps = velocidades.get(e.chave) ?? 0
        const falta = restante(bps > 0 ? (e.total - e.feitos) / bps : 0)
        const andamento = compacta
          ? `${pct}%${falta ? ` · ${falta}` : ''}`
          : `${pct}% de ${humanSize(e.total)}${falta ? ` · faltam ${falta} · ${mbps(bps)}` : ''}`
        return (
          <li key={e.chave} className={compacta ? 'px-2.5 py-2' : 'px-3 py-2.5'}>
            <div className={`flex items-center justify-between gap-3 ${compacta ? '' : 'text-sm'}`}>
              <span className="flex min-w-0 items-center gap-2">
                <NuvemIcon
                  key={e.estado}
                  estado={e.estado === 'pronto' ? 'concluido' : e.estado === 'erro' ? 'erro' : e.estado === 'pausado' ? 'local' : 'enviando'}
                  className={e.estado === 'erro' ? 'shrink-0 text-danger' : e.estado === 'pronto' ? 'shrink-0 text-ok' : 'shrink-0 text-accent'}
                />
                <span className="line-clamp-1 font-medium">{e.nome}</span>
                {e.doMac && (
                  <span className="shrink-0 rounded bg-elev px-1.5 py-0.5 font-mono text-[10px] tracking-wide text-muted uppercase">
                    do Mac
                  </span>
                )}
              </span>
              <span
                className={[
                  'shrink-0',
                  compacta ? 'text-[11px]' : 'text-xs',
                  e.estado === 'erro' ? 'text-danger' : e.estado === 'pronto' ? 'text-ok' : 'text-muted',
                ].join(' ')}
              >
                {e.estado === 'pronto'
                  ? e.doMac
                    ? 'no Mac e na nuvem'
                    : 'na nuvem'
                  : e.estado === 'pausado'
                    ? `pausado · ${pct}%`
                    : e.estado === 'erro'
                      ? 'falhou'
                      : andamento}
              </span>
            </div>
            <div className="mt-1.5 h-1 overflow-hidden rounded bg-elev">
              <div
                className={`h-full transition-[width] ${e.estado === 'pronto' ? 'bg-ok' : 'bg-accent'}`}
                style={{ width: `${e.estado === 'pronto' ? 100 : pct}%` }}
              />
            </div>
            {e.erro && (
              <div className="mt-1.5 flex items-center justify-between gap-2 text-xs text-danger">
                <span className="line-clamp-2">{e.erro}</span>
                {e.tentar && (
                  <button type="button" onClick={e.tentar} className="shrink-0 underline">
                    tentar de novo
                  </button>
                )}
              </div>
            )}
          </li>
        )
      })}
    </ul>
  )
}
