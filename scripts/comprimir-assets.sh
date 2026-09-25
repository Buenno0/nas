#!/bin/sh
# Pré-compressão dos assets do frontend.
#
# Comprimir no build em vez de a cada requisição: os arquivos do Vite têm hash
# no nome e são imutáveis, então gastar CPU comprimindo o mesmo conteúdo a cada
# visita seria desperdício. Fontes e imagens ficam de fora — já vêm comprimidas,
# e passar gzip nelas só aumentaria o tamanho.
set -e
DIST="${1:-internal/web/dist}"
[ -d "$DIST" ] || { echo "sem $DIST: rode o build do frontend primeiro"; exit 1; }

total_cru=0
total_gz=0
n=0

# -type f e a lista fechada de extensões: nada de comprimir .woff2/.png/.gz.
find "$DIST" -type f \
  \( -name '*.js' -o -name '*.css' -o -name '*.html' -o -name '*.svg' -o -name '*.webmanifest' \) \
  | while read -r arquivo; do
      gzip -9 -c "$arquivo" > "$arquivo.gz"
      cru=$(stat -f%z "$arquivo")
      gz=$(stat -f%z "$arquivo.gz")
      # Se a compressão não valeu a pena (arquivo minúsculo), descarta o .gz.
      if [ "$gz" -ge "$cru" ]; then
        rm -f "$arquivo.gz"
        continue
      fi
      printf '  %-46s %7d → %6d bytes (%d%%)\n' \
        "$(basename "$arquivo")" "$cru" "$gz" "$((100 - gz * 100 / cru))"
    done
