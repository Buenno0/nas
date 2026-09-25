// Ícones inline: nenhuma dependência extra e todos herdam currentColor.
import { useId, type SVGProps } from 'react'

type IconProps = SVGProps<SVGSVGElement>

function Icon({ children, ...props }: IconProps & { children: React.ReactNode }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
      width="1.25em"
      height="1.25em"
      aria-hidden="true"
      {...props}
    >
      {children}
    </svg>
  )
}

export const HomeIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M3 10.5 12 3l9 7.5" />
    <path d="M5 9.5V21h14V9.5" />
  </Icon>
)

export const FilmIcon = (p: IconProps) => (
  <Icon {...p}>
    <rect x="3" y="4" width="18" height="16" rx="2" />
    <path d="M8 4v16M16 4v16M3 12h18M3 8h5M3 16h5M16 8h5M16 16h5" />
  </Icon>
)

export const TvIcon = (p: IconProps) => (
  <Icon {...p}>
    <rect x="2" y="6" width="20" height="13" rx="2" />
    <path d="m8 3 4 3 4-3" />
  </Icon>
)

export const MusicIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M9 18V5l11-2v13" />
    <circle cx="6" cy="18" r="3" />
    <circle cx="17" cy="16" r="3" />
  </Icon>
)

export const PhotoIcon = (p: IconProps) => (
  <Icon {...p}>
    <rect x="3" y="4" width="18" height="16" rx="2" />
    <circle cx="9" cy="10" r="2" />
    <path d="m3 17 5-4 4 3 3-2 6 5" />
  </Icon>
)

export const SearchIcon = (p: IconProps) => (
  <Icon {...p}>
    <circle cx="11" cy="11" r="7" />
    <path d="m20 20-3.2-3.2" />
  </Icon>
)

export const SettingsIcon = (p: IconProps) => (
  <Icon {...p}>
    <circle cx="12" cy="12" r="3.2" />
    <path d="M20 12a8 8 0 0 0-.14-1.5l2-1.5-2-3.4-2.4 1a8 8 0 0 0-2.6-1.5L14.5 2h-5l-.36 2.6A8 8 0 0 0 6.54 6.1l-2.4-1-2 3.4 2 1.5A8 8 0 0 0 4 12c0 .5.05 1 .14 1.5l-2 1.5 2 3.4 2.4-1a8 8 0 0 0 2.6 1.5l.36 2.6h5l.36-2.6a8 8 0 0 0 2.6-1.5l2.4 1 2-3.4-2-1.5c.09-.5.14-1 .14-1.5Z" />
  </Icon>
)

export const SunIcon = (p: IconProps) => (
  <Icon {...p}>
    <circle cx="12" cy="12" r="4" />
    <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
  </Icon>
)

export const MoonIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M20 14.5A8.5 8.5 0 0 1 9.5 4a8.5 8.5 0 1 0 10.5 10.5Z" />
  </Icon>
)

export const PlayIcon = (p: IconProps) => (
  <Icon fill="currentColor" stroke="none" {...p}>
    <path d="M8 5.5v13l11-6.5z" />
  </Icon>
)

export const PauseIcon = (p: IconProps) => (
  <Icon fill="currentColor" stroke="none" {...p}>
    <rect x="7" y="5" width="3.5" height="14" rx="1" />
    <rect x="13.5" y="5" width="3.5" height="14" rx="1" />
  </Icon>
)

export const HeartIcon = ({ filled, ...p }: IconProps & { filled?: boolean }) => (
  <Icon fill={filled ? 'currentColor' : 'none'} {...p}>
    <path d="M12 20s-7-4.4-7-9.2A4 4 0 0 1 12 8a4 4 0 0 1 7 2.8C19 15.6 12 20 12 20Z" />
  </Icon>
)

export const LogoutIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M15 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3" />
    <path d="M10 8 6 12l4 4M6 12h10" />
  </Icon>
)

export const ChevronLeft = (p: IconProps) => (
  <Icon {...p}>
    <path d="m15 5-7 7 7 7" />
  </Icon>
)

export const ChevronRight = (p: IconProps) => (
  <Icon {...p}>
    <path d="m9 5 7 7-7 7" />
  </Icon>
)

export const RefreshIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M20 12a8 8 0 1 1-2.6-5.9" />
    <path d="M20 4v5h-5" />
  </Icon>
)

export const VolumeIcon = ({ muted, ...p }: IconProps & { muted?: boolean }) => (
  <Icon {...p}>
    <path d="M4 9v6h3.5L12 19V5L7.5 9H4Z" />
    {muted ? <path d="m16 9 5 6M21 9l-5 6" /> : <path d="M16 9.5a4 4 0 0 1 0 5M18.5 7a7 7 0 0 1 0 10" />}
  </Icon>
)

export const FullscreenIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" />
  </Icon>
)

export const PipIcon = (p: IconProps) => (
  <Icon {...p}>
    <rect x="3" y="5" width="18" height="14" rx="2" />
    <rect x="12" y="11" width="7" height="6" rx="1" />
  </Icon>
)

export const DownloadIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M12 4v10m0 0 4-4m-4 4-4-4" />
    <path d="M5 18h14" />
  </Icon>
)

export const WarningIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M12 4 2.8 20h18.4L12 4Z" />
    <path d="M12 10v4M12 17.2v.1" />
  </Icon>
)

export const ActivityIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M2 12h4l3-7 4 14 3-7h6" />
  </Icon>
)

export const ClockIcon = (p: IconProps) => (
  <Icon {...p}>
    <circle cx="12" cy="12" r="9" />
    <path d="M12 7v5.2l3.4 2" />
  </Icon>
)

export const ChipIcon = (p: IconProps) => (
  <Icon {...p}>
    <rect x="7" y="7" width="10" height="10" rx="1.5" />
    <path d="M10 3v4M14 3v4M10 17v4M14 17v4M3 10h4M3 14h4M17 10h4M17 14h4" />
  </Icon>
)

export const DiskIcon = (p: IconProps) => (
  <Icon {...p}>
    <ellipse cx="12" cy="6" rx="8" ry="3" />
    <path d="M4 6v12c0 1.66 3.58 3 8 3s8-1.34 8-3V6" />
    <path d="M4 12c0 1.66 3.58 3 8 3s8-1.34 8-3" />
  </Icon>
)

export const ShuffleIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M3 6h3.5l4 5.5M3 18h3.5l4-5.5" />
    <path d="M14 6h4.5m0 0L16 3.5M18.5 6 16 8.5" />
    <path d="M14 18h4.5m0 0L16 15.5M18.5 18 16 20.5" />
    <path d="M12.5 8.2 14 6M12.5 15.8 14 18" />
  </Icon>
)

export const ListIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M8 6h13M8 12h13M8 18h13" />
    <path d="M3.5 6h.01M3.5 12h.01M3.5 18h.01" />
  </Icon>
)

export const CloudIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M7 18h10.5a4 4 0 0 0 .6-7.95A6 6 0 0 0 6.5 9.2 4.5 4.5 0 0 0 7 18Z" />
  </Icon>
)

export const CloudOffIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M9 5.6A6 6 0 0 1 18.1 10a4 4 0 0 1 2.4 6.6M17 18H7a4.5 4.5 0 0 1-.9-8.9" />
    <path d="M3 3l18 18" />
  </Icon>
)

export const UploadIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M12 16V4M7 9l5-5 5 5" />
    <path d="M4 16v3a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-3" />
  </Icon>
)

/** Os estados de nuvem, um desenho e uma animação cada (canvas "Céu fechado"). */
export type EstadoNuvem =
  | 'hibrido'
  | 'local'
  | 'conectando'
  | 'enviando'
  | 'baixando'
  | 'sincronizando'
  | 'concluido'
  | 'erro'
  | 'processando'
  | 'no-mac'

const CONTORNO = 'M7 18h10.5a4 4 0 0 0 .6-7.95A6 6 0 0 0 6.5 9.2 4.5 4.5 0 0 0 7 18Z'

/** Nuvem animada. `estado` escolhe o desenho; a cor vem de currentColor,
 *  então quem usa decide o tom (ouro para ação, osso para espera…). */
export function NuvemIcon({ estado, ...p }: IconProps & { estado: EstadoNuvem }) {
  switch (estado) {
    case 'hibrido':
      return (
        <Icon {...p} style={{ overflow: 'visible', ...p.style }}>
          <path className="nv-flutua" d={CONTORNO} />
        </Icon>
      )
    case 'local':
      return (
        <Icon {...p}>
          <path className="nv-esmaece" d="M9 5.6A6 6 0 0 1 18.1 10a4 4 0 0 1 2.4 6.6M17 18H7a4.5 4.5 0 0 1-.9-8.9" />
          <path className="nv-corta" d="M3 3l18 18" />
        </Icon>
      )
    case 'conectando':
      return (
        <Icon {...p}>
          <path d={CONTORNO} strokeOpacity={0.3} />
          <path className="nv-contorno" d={CONTORNO} />
        </Icon>
      )
    case 'enviando':
      return (
        <Icon {...p}>
          <path d={CONTORNO} strokeOpacity={0.45} />
          <g className="nv-sobe">
            <path d="M12 16v-6M9.5 12.5 12 10l2.5 2.5" />
          </g>
        </Icon>
      )
    case 'baixando':
      return (
        <Icon {...p}>
          <path d={CONTORNO} strokeOpacity={0.45} />
          <g className="nv-desce">
            <path d="M12 10v6M9.5 13.5 12 16l2.5-2.5" />
          </g>
        </Icon>
      )
    case 'sincronizando':
      return (
        <Icon {...p}>
          <path d={CONTORNO} />
          <g className="nv-gira">
            <path d="M9.6 12.6a2.5 2.5 0 0 1 4.3-1.7M14.4 13.4a2.5 2.5 0 0 1-4.3 1.7" />
          </g>
        </Icon>
      )
    case 'concluido':
      return (
        <Icon {...p}>
          <path d={CONTORNO} />
          <path className="nv-marca" d="M9.3 13.3 11.2 15.2 14.8 11.4" />
        </Icon>
      )
    case 'erro':
      return (
        <Icon {...p}>
          <g className="nv-treme">
            <path d={CONTORNO} />
            <path className="nv-alerta" d="M12 10.5v2.6M12 15.4v.1" />
          </g>
        </Icon>
      )
    case 'processando':
      return (
        <Icon {...p}>
          <path d={CONTORNO} />
          {[9.5, 12, 14.5].map((cx, i) => (
            <circle key={cx} className="nv-ponto" cx={cx} cy={13.5} r={0.6} fill="currentColor" style={{ animationDelay: `${i * 0.2}s` }} />
          ))}
        </Icon>
      )
    case 'no-mac':
      return (
        <Icon {...p}>
          <path d="M4 15.5h16M6 15.5V7.5a1.5 1.5 0 0 1 1.5-1.5h9A1.5 1.5 0 0 1 18 7.5v8M3 18h18" />
          <path className="nv-dorme" d="M10.6 9.2a2.2 2.2 0 1 0 2.8 2.8 1.8 1.8 0 0 1-2.8-2.8Z" />
        </Icon>
      )
  }
}

/** A lua no lugar do spinner: troca de fase em 3,2 s. */
export function LuaSpinner(p: IconProps) {
  // Uma máscara por instância: dois spinners na tela não podem dividir o id.
  const id = useId()
  return (
    <svg viewBox="0 0 120 120" width="1.25em" height="1.25em" aria-hidden="true" {...p}>
      <defs>
        <mask id={id}>
          <rect width="120" height="120" fill="#000" />
          <circle cx="60" cy="60" r="27" fill="#fff" />
          <rect x="60" y="0" width="60" height="120" fill="#000" />
          <ellipse className="nv-fase" cx="60" cy="60" rx="12" ry="27" fill="#fff" />
        </mask>
      </defs>
      <circle cx="60" cy="60" r="27" fill="currentColor" opacity={0.18} />
      <circle cx="60" cy="60" r="27" fill="currentColor" mask={`url(#${id})`} />
    </svg>
  )
}
