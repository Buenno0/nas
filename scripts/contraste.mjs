#!/usr/bin/env node
// Contraste WCAG 2.1 para os pares de cor do design system.
// Uso: node scripts/contraste.mjs   (imprime a tabela e sai 1 se algo reprova)

const canal = (v) => {
  const s = v / 255
  return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
}
const luminancia = (hex) => {
  const h = hex.replace('#', '')
  const n = parseInt(h.length === 3 ? h.split('').map((c) => c + c).join('') : h, 16)
  return 0.2126 * canal((n >> 16) & 255) + 0.7152 * canal((n >> 8) & 255) + 0.0722 * canal(n & 255)
}
export const contraste = (a, b) => {
  const [x, y] = [luminancia(a), luminancia(b)].sort((p, q) => q - p)
  return (x + 0.05) / (y + 0.05)
}

const P = {
  escuro: {
    bg: '#0c0a08', surface: '#16130f', elev: '#211c15', line: '#40372b',
    ink: '#f4efe4', muted: '#a99a84', accent: '#d29a44', accentInk: '#1a1206',
    ok: '#7fb79a', warn: '#dca84a', danger: '#e08163',
  },
  claro: {
    bg: '#f6f3ee', surface: '#fdfbf7', elev: '#efe9df', line: '#d0c4b1',
    ink: '#241e17', muted: '#6b6154', accent: '#9d5a26', accentInk: '#fdfbf7',
    ok: '#2f6f52', warn: '#8a5a12', danger: '#9c3f1d',
  },
}

const pares = (p) => [
  ['corpo sobre fundo', p.ink, p.bg, 4.5],
  ['corpo sobre superfície', p.ink, p.surface, 4.5],
  ['corpo sobre elevação', p.ink, p.elev, 4.5],
  ['secundário sobre fundo', p.muted, p.bg, 4.5],
  ['secundário sobre superfície', p.muted, p.surface, 4.5],
  ['secundário sobre elevação', p.muted, p.elev, 4.5],
  ['acento como texto sobre fundo', p.accent, p.bg, 4.5],
  ['acento como texto sobre superfície', p.accent, p.surface, 4.5],
  ['botão primário', p.accentInk, p.accent, 4.5],
  ['sucesso sobre superfície', p.ok, p.surface, 4.5],
  ['aviso sobre superfície', p.warn, p.surface, 4.5],
  ['perigo sobre superfície', p.danger, p.surface, 4.5],
  ['borda sobre superfície', p.line, p.surface, 1.4],
  ['borda sobre fundo', p.line, p.bg, 1.4],
]

let falhas = 0
for (const [nome, p] of Object.entries(P)) {
  console.log(`\n── produto ${nome} ${'─'.repeat(46 - nome.length)}`)
  for (const [rotulo, fg, bg, alvo] of pares(p)) {
    const c = contraste(fg, bg)
    const ok = c >= alvo
    if (!ok) falhas++
    console.log(
      `${ok ? '  ok  ' : ' FALHA'} ${rotulo.padEnd(36)} ${fg} / ${bg}  ${c.toFixed(2).padStart(5)}:1  (alvo ${alvo})`,
    )
  }
}

// Login (prefixo --lg-*) e telas de erro: mesma paleta, tokens próprios para
// a cena. Os valores são copiados de login.css e dos três HTML autônomos —
// se você mexer lá, mexa aqui, ou a tabela do DESIGN.md vira ficção.
const L = {
  escuro: {
    bg: '#0c0a08', card: '#16130f', field: '#100d09', line2: '#3a3126',
    text: '#f4efe4', muted: '#a99a84', faint: '#7a6d5c',
    accent: '#d29a44', accentInk: '#1a1206', danger: '#e08163', ok: '#7fb79a',
    btnBg: '#d29a44', btnFg: '#1a1206',
    lua: '#ded1b6', luaCratera: '#c3b28e',
  },
  claro: {
    bg: '#f6f3ee', card: '#fdfbf7', field: '#f1ece4', line2: '#d0c4b1',
    text: '#241e17', muted: '#6b6154', faint: '#7d7466',
    accent: '#9d5a26', accentInk: '#fdfbf7', danger: '#9c3f1d', ok: '#2f6f52',
    btnBg: '#9d5a26', btnFg: '#fdfbf7',
  },
}

// O micro-rótulo mono é decorativo: 3,0:1 basta.
const paresLogin = (p) => [
  ['corpo sobre cartão', p.text, p.card, 4.5],
  ['secundário sobre cartão', p.muted, p.card, 4.5],
  ['micro-rótulo mono sobre fundo', p.faint, p.bg, 3.0],
  ['acento como link sobre cartão', p.accent, p.card, 4.5],
  ['acento como link sobre fundo', p.accent, p.bg, 4.5],
  ['botão primário', p.btnFg, p.btnBg, 4.5],
  ['tique sobre caixa marcada', p.accentInk, p.accent, 4.5],
  ['erro sobre cartão', p.danger, p.card, 4.5],
  ['ponto de status sobre fundo', p.ok, p.bg, 3.0],
  ['texto em campo', p.text, p.field, 4.5],
  // A lua é fonte de luz, não texto: o alvo é o de elemento gráfico (3,0).
  // Ela só existe no escuro, então no claro o par é pulado.
  ...(p.lua
    ? [
        ['lua sobre o céu', p.lua, p.bg, 3.0],
        ['cratera sobre a lua', p.luaCratera, p.lua, 1.2],
      ]
    : []),
]

for (const [nome, p] of Object.entries(L)) {
  console.log(`\n── login e ruínas ${nome} ${'─'.repeat(40 - nome.length)}`)
  for (const [rotulo, fg, bg, alvo] of paresLogin(p)) {
    const c = contraste(fg, bg)
    const ok = c >= alvo
    if (!ok) falhas++
    console.log(
      `${ok ? '  ok  ' : ' FALHA'} ${rotulo.padEnd(36)} ${fg} / ${bg}  ${c.toFixed(2).padStart(5)}:1  (alvo ${alvo})`,
    )
  }
}

console.log(falhas ? `\n${falhas} par(es) reprovando.` : '\nTodos os pares passam.')
process.exit(falhas ? 1 : 0)
