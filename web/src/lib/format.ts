// Formatações compartilhadas pela interface.

/** 3725 → "1h 02min"; 145 → "2min"; 45 → "45s" */
export function humanDuration(seconds?: number): string {
  if (!seconds || seconds < 1) return ''
  const total = Math.round(seconds)
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  if (h > 0) return `${h}h ${String(m).padStart(2, '0')}min`
  if (m > 0) return `${m}min`
  return `${total}s`
}

/** 3725 → "1:02:05"; 145 → "2:25" — formato de player. */
export function clockTime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) seconds = 0
  const total = Math.floor(seconds)
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  return `${m}:${String(s).padStart(2, '0')}`
}

export function humanSize(bytes: number): string {
  if (!bytes) return ''
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`
}

/** Cor estável a partir do nome, para os pôsteres sem capa não ficarem iguais.
 *
 *  O matiz não passeia pelos 360°: fica num arco quente de 95°, de terracota
 *  (~25°) a bronze esverdeado (~120°), passando pelo ocre. Girar o círculo
 *  inteiro jogava rosa e ciano no meio de uma grade de pedra e areia — era a
 *  cor mais fora do sistema em toda a interface. A croma também caiu: esses
 *  blocos são o fundo de um título sem capa, não o assunto da tela. */
const ARCO_INICIO = 25
const ARCO_TAMANHO = 95

export function gradientFor(name: string): string {
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = (hash * 31 + name.charCodeAt(i)) >>> 0
  const hue = ARCO_INICIO + (hash % ARCO_TAMANHO)
  return `linear-gradient(145deg, oklch(0.42 0.085 ${hue}), oklch(0.25 0.055 ${hue + 22}))`
}

export function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  if (words.length === 0) return '?'
  if (words.length === 1) return words[0].slice(0, 2).toUpperCase()
  return (words[0][0] + words[1][0]).toUpperCase()
}

export const kindLabel: Record<string, string> = {
  movie: 'Filme',
  tv: 'Série',
  album: 'Álbum',
  photos: 'Fotos',
  music: 'Música',
  photo: 'Fotos',
}
