#!/usr/bin/env bash
# Sobe a folha de provas do design system: os onze tokens com o valor
# resolvido e cada receita do §5 desenhada, nos dois temas.
#
# Usa o CSS já compilado em internal/web/dist — então rode `make web` antes se
# você acabou de mexer em index.css. Provar o CSS de produção é o ponto: uma
# folha que reimplementasse as receitas à mão provaria a folha, não o sistema.
#
# Uso: ./scripts/folha-de-provas.sh [porta]
set -euo pipefail
cd "$(dirname "$0")/.."

porta=${1:-5199}
css=$(ls -t internal/web/dist/assets/index-*.css 2>/dev/null | head -1)
if [ -z "$css" ]; then
  echo "não achei o CSS compilado. rode 'make web' primeiro." >&2
  exit 1
fi

sala=$(mktemp -d)
trap 'rm -rf "$sala"' EXIT
cp scripts/folha-de-provas.html "$sala/index.html"
cp "$css" "$sala/sistema.css"
cp -R web/public/fonts "$sala/fonts"

echo "folha de provas em http://localhost:$porta  (CSS: $css)"
echo "Ctrl+C para parar."
cd "$sala" && python3 -m http.server "$porta" --bind 127.0.0.1 >/dev/null
