import { useEffect, useRef, useState } from 'react'

// O estrato das telas de erro: lente longa com a borda de cima acesa pela lua.
const ESTRATO = 'M0 40 C60 18 140 22 210 30 C260 10 330 12 382 30 C432 24 472 34 500 44 C420 54 300 52 200 54 C120 56 50 52 0 40 Z'
const BORDA = 'M24 34 C80 20 150 24 210 30 C262 12 330 14 380 31 C430 26 466 34 490 42'

/**
 * A nuvem do envio (canvas "Céu fechado"): cresce com o progresso do lote, e
 * os grãos de areia que sobem da barra ficam mais rápidos e mais numerosos
 * perto do fim. Quando o lote inteiro termina, a nuvem solta um raio na barra
 * e só então a marca se desenha — um raio por lote, não por arquivo, para
 * vários envios terminando juntos não virarem tempestade.
 */
export function NuvemDeEnvio({ progresso, ativo }: { progresso: number; ativo: boolean }) {
  const f = Math.max(0, Math.min(1, progresso))
  const completo = !ativo && f >= 1

  // O raio só cai na virada de "enviando" para "pronto", não ao montar a tela
  // com um lote que já tinha terminado.
  const estavaAtivo = useRef(ativo)
  const [raio, setRaio] = useState(0)
  useEffect(() => {
    if (estavaAtivo.current && completo) setRaio((n) => n + 1)
    estavaAtivo.current = ativo
  }, [ativo, completo])

  const escala = 0.55 + 0.75 * f
  const acelera = 1 - 0.72 * f
  const quantos = completo ? 0 : 6 + Math.round(10 * f)
  const borda = completo ? 0.85 : 0.16 + 0.34 * f

  return (
    <svg viewBox="0 0 460 150" className="h-auto w-full max-w-[460px] overflow-visible" aria-hidden="true">
      <g
        style={{
          transform: `scale(${escala.toFixed(3)})`,
          transformOrigin: '230px 22px',
          transition: 'transform .45s cubic-bezier(.2,.8,.2,1)',
        }}
      >
        <g className="nv-junta">
          <g transform="translate(110 -6) scale(0.48)">
            <path d={ESTRATO} className="fill-elev" />
            <path
              d={BORDA}
              fill="none"
              stroke="var(--ink)"
              strokeOpacity={borda}
              strokeWidth={2.2}
              strokeLinecap="round"
              style={{ transition: 'stroke-opacity .45s ease' }}
            />
          </g>
        </g>
        {completo && (
          <path
            key={`marca-${raio}`}
            className="nv-marca-fim"
            d="M221 16 L228 23 L241 9"
            fill="none"
            stroke="var(--ok)"
            strokeWidth={2.4}
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        )}
      </g>

      {Array.from({ length: quantos }, (_, i) => {
        const dur = (2.2 + (i % 4) * 0.3) * acelera
        return (
          <circle
            key={i}
            className="nv-grao"
            cx={30 + ((i * 97) % 400)}
            cy={146}
            r={1.6 + (i % 3) * 0.5}
            fill="var(--muted)"
            style={{ animationDuration: `${dur.toFixed(2)}s`, animationDelay: `${(-((i * 0.37) % dur)).toFixed(2)}s` }}
          />
        )
      })}

      {raio > 0 && completo && (
        <g key={`raio-${raio}`}>
          <rect x="-40" y="-40" width="540" height="230" fill="var(--ink)" className="nv-clarao" />
          <g className="nv-raio" style={{ filter: 'drop-shadow(0 0 6px var(--accent))' }}>
            <path
              className="nv-raio-traco"
              d="M236 40 L222 78 L238 80 L218 118 L234 120 L214 150"
              fill="none"
              stroke="var(--ink)"
              strokeWidth={2.6}
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </g>
          <circle className="nv-impacto" cx="214" cy="150" r="10" fill="var(--accent)" />
        </g>
      )}
    </svg>
  )
}
