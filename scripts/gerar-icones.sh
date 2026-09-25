#!/usr/bin/env bash
# Gera os PNG da marca a partir da mesma geometria de web/src/components/Mark.tsx.
#
# Os PNG existem porque iOS e o manifest não aceitam SVG com troca de tema; são
# a única variante FIXA da marca. Este script é a fonte deles — se a paleta
# mudar em index.css, rode-o de novo em vez de editar imagem à mão.
#
# Uso: ./scripts/gerar-icones.sh     (precisa de qlmanage, que já vem no macOS)
set -euo pipefail
cd "$(dirname "$0")/.."

destino=web/public/icons
trabalho=$(mktemp -d)
trap 'rm -rf "$trabalho"' EXIT

# A geometria, uma vez só. $1 = corpo, $2 = touca, $3 = vazado (olhos e play).
marca() {
  cat <<XML
    <path d="M18 22 C18 14 27 9 48 9 C69 9 78 14 78 22 L78 52 C78 68 66 80 48 87 C30 80 18 68 18 52 Z" fill="$1"/>
    <path d="M24 24 C24 18 32 14 48 14 C64 14 72 18 72 24 L72 34 L24 34 Z" fill="$2"/>
    <rect x="24" y="36" width="48" height="4" fill="$2"/>
    <circle cx="37" cy="53" r="8" fill="$3"/>
    <circle cx="59" cy="53" r="8" fill="$3"/>
    <path d="M41 66 L41 78 L55 72 Z" fill="$3"/>
XML
}

# Ícone de app: placa de ouro gasto a terracota, máscara em papel. É a variante
# fixa — iOS não troca ícone por tema, então ela não segue claro nem escuro.
#
# O degradê desce, não corre na diagonal. Não é gosto: PNG comprime por
# diferença com a linha de cima, e um degradê vertical deixa essa diferença
# constante. O mesmo desenho na diagonal ocupava 213 KB; assim, 73 KB.
cat > "$trabalho/appicon.svg" <<XML
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="512" height="512">
  <defs>
    <linearGradient id="placa" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#e0a94f"/>
      <stop offset="1" stop-color="#a8642c"/>
    </linearGradient>
  </defs>
  <rect width="512" height="512" rx="114" fill="url(#placa)"/>
  <g transform="translate(101.5 105.75) scale(3.25)">
$(marca '#fdfbf7' '#f0dcb8' '#6d3c15')
  </g>
</svg>
XML

# A marca solta, fundo transparente, um arquivo por tema. Mesmas cores de
# Mark.tsx: no claro é pedra com touca de ouro; no escuro, o ouro é o corpo.
cat > "$trabalho/mark-claro.svg" <<XML
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 96 96" width="512" height="512">
$(marca '#241e17' '#c98f3c' '#f6f3ee')
  <path d="M41 66 L41 78 L55 72 Z" fill="#9d5a26"/>
</svg>
XML

cat > "$trabalho/mark-escuro.svg" <<XML
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 96 96" width="512" height="512">
  <path d="M18 22 C18 14 27 9 48 9 C69 9 78 14 78 22 L78 52 C78 68 66 80 48 87 C30 80 18 68 18 52 Z" fill="#d29a44"/>
  <path d="M24 24 C24 18 32 14 48 14 C64 14 72 18 72 24 L72 34 L24 34 Z" fill="#1a1206" opacity="0.38"/>
  <rect x="24" y="36" width="48" height="4" fill="#1a1206" opacity="0.38"/>
  <circle cx="37" cy="53" r="8" fill="#1a1206"/>
  <circle cx="59" cy="53" r="8" fill="#1a1206"/>
  <path d="M41 66 L41 78 L55 72 Z" fill="#1a1206"/>
</svg>
XML

render() { # $1 = svg, $2 = lado, $3 = destino
  rm -f "$trabalho/$(basename "$1").png"
  qlmanage -t -s "$2" -o "$trabalho" "$1" >/dev/null 2>&1
  sips -s format png -Z "$2" "$trabalho/$(basename "$1").png" --out "$3" >/dev/null
  echo "  $3  (${2}px)"
}

echo "gerando ícones:"
render "$trabalho/appicon.svg"     512 "$destino/appicon-512.png"
render "$trabalho/appicon.svg"     192 "$destino/appicon-192.png"
render "$trabalho/appicon.svg"     192 "$destino/apple-touch-icon.png"
render "$trabalho/mark-claro.svg"  512 "$destino/mark-claro-512.png"
render "$trabalho/mark-escuro.svg" 512 "$destino/mark-escuro-512.png"
