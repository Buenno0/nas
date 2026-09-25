import { useTheme } from '../lib/theme'

// A marca do NAS, inline em SVG: são poucos bytes, então não vale uma
// requisição de rede — e a troca de tema acontece no mesmo frame, sem piscar.
//
// Um objeto só, duas iluminações. No claro é a máscara vista à luz: pedra
// escura, touca de ouro, olhos de areia. No escuro é a mesma máscara vista
// contra o basalto — o ouro passa a ser o corpo e o resto vira sombra.
//
// As cores são as mesmas do produto (ver `index.css`): o corpo é o acento e
// o vazado é a tinta do acento. Trocar uma coisa sem a outra quebra o par.

interface MarkProps {
  /** Lado do quadrado, em pixels. */
  size?: number
  className?: string
}

const paletas = {
  light: {
    corpo: '#241e17', // --ink
    touca: '#c98f3c',
    toucaOpacidade: 1,
    olhos: '#f6f3ee', // --bg
    play: '#9d5a26', // --accent
  },
  dark: {
    corpo: '#d29a44', // --accent
    touca: '#1a1206', // --accent-ink
    toucaOpacidade: 0.38,
    olhos: '#1a1206',
    play: '#1a1206',
  },
} as const

export function Mark({ size = 32, className }: MarkProps) {
  const { theme } = useTheme()
  const c = paletas[theme]

  return (
    <svg
      viewBox="0 0 96 96"
      width={size}
      height={size}
      fill="none"
      className={className}
      role="img"
      aria-label="Ozymandias"
    >
      <path
        d="M18 22 C18 14 27 9 48 9 C69 9 78 14 78 22 L78 52 C78 68 66 80 48 87 C30 80 18 68 18 52 Z"
        fill={c.corpo}
      />
      <path
        d="M24 24 C24 18 32 14 48 14 C64 14 72 18 72 24 L72 34 L24 34 Z"
        fill={c.touca}
        opacity={c.toucaOpacidade}
      />
      <rect x="24" y="36" width="48" height="4" fill={c.touca} opacity={c.toucaOpacidade} />
      <circle cx="37" cy="53" r="8" fill={c.olhos} />
      <circle cx="59" cy="53" r="8" fill={c.olhos} />
      <path d="M41 66 L41 78 L55 72 Z" fill={c.play} />
    </svg>
  )
}
