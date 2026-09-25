#!/usr/bin/env python3
"""Gera as telas de erro da nuvem (502, 503 e 503-no-mac) a partir da 404.

As telas da nuvem são a mesma cena das ruínas — céu, lua, dunas, estátua,
parallax, os dois temas, as fontes locais — com nuvens. Gerar a partir da 404
mantém uma fonte só da cena: mexeu na 404, rode de novo.

    python3 scripts/telas-da-nuvem.py

O desenho vem do canvas "Céu fechado" (Claude Design).
"""
import os
import re

RAIZ = os.path.join(os.path.dirname(__file__), "..", "web", "public", "telas-erro")
BASE = open(os.path.join(RAIZ, "404.html"), encoding="utf-8").read()

# Um estrato: lente longa e baixa, com a borda de cima acesa pela lua.
def estrato(x, y, w, cor="var(--nuvem)", borda="var(--nuvem-borda)"):
    return (f'<g transform="translate({x} {y}) scale({w/500:.4f})">'
            f'<path d="M0 40 C60 18 140 22 210 30 C260 10 330 12 382 30 C432 24 472 34 500 44 C420 54 300 52 200 54 C120 56 50 52 0 40 Z" fill="{cor}"/>'
            f'<path d="M24 34 C80 20 150 24 210 30 C262 12 330 14 380 31 C430 26 466 34 490 42" fill="none" stroke="{borda}" stroke-width="2.2" stroke-linecap="round"/>'
            f'</g>')

def faixa(classe, nuvens):
    # O desenho repetido a 1600: correr 1600 volta ao começo sem emenda.
    um = "".join(estrato(x, y, w, *resto) for x, y, w, *resto in nuvens)
    dois = "".join(estrato(x + 1600, y, w, *resto) for x, y, w, *resto in nuvens)
    return f'<g class="{classe}">{um}{dois}</g>'

CSS_NUVEM = """
  /* ---------- as nuvens ----------
     Estratos longos em tom de pedra, a borda de cima acesa pela lua. Ficam
     DEPOIS da lua no DOM, então passam por cima dela; no claro, viram areia
     pálida sobre o sol. Cada faixa corre no seu ritmo: nada anda junto. */
  :root { --nuvem: #211c15; --nuvem-2: #262017; --nuvem-borda: rgba(222,209,182,0.16);
          --nevoa: rgba(194,174,142,0.13); --tom: #ded1b6; --accent-ink: #1a1206; }
  :root.light { --nuvem: #e3dbcd; --nuvem-2: #dcd2c1; --nuvem-borda: rgba(253,251,247,0.9);
                --nevoa: rgba(253,251,247,0.75); --tom: #6b6154; --accent-ink: #fdfbf7; }
  @media (prefers-color-scheme: light) {
    :root:not(.dark) { --nuvem: #e3dbcd; --nuvem-2: #dcd2c1; --nuvem-borda: rgba(253,251,247,0.9);
                       --nevoa: rgba(253,251,247,0.75); --tom: #6b6154; --accent-ink: #fdfbf7; }
  }
  .nuvens { position: absolute; z-index: 1; left: 0; top: 0; width: 100%; height: 62%; pointer-events: none; }
  .nevoas { position: absolute; z-index: 1; left: 0; bottom: 0; width: 100%; height: 58%; pointer-events: none; }
  .corre-lento { animation: nuvemCorre 150s linear infinite, nuvemEntra 2.2s cubic-bezier(.16,.84,.24,1) backwards; }
  .corre-medio { animation: nuvemCorre 96s linear infinite, nuvemEntra 2.2s cubic-bezier(.16,.84,.24,1) .15s backwards; }
  .corre-rapido { animation: nuvemCorre 64s linear infinite, nuvemEntra 2.2s cubic-bezier(.16,.84,.24,1) .3s backwards; }
  .nevoa-1 { animation: nuvemCorre 120s linear infinite, nevoaRespira 18s ease-in-out infinite; }
  .nevoa-2 { animation: nuvemCorre 80s linear infinite, nevoaRespira 13s ease-in-out -4s infinite; }
  .olho-sono { animation: sono 6s ease-in-out infinite; }
  @keyframes nuvemCorre { from { transform: translateX(0); } to { transform: translateX(-1600px); } }
  @keyframes nuvemEntra { from { opacity: 0; } to { opacity: 1; } }
  @keyframes nevoaRespira { 0%, 100% { opacity: .55; } 50% { opacity: .95; } }
  @keyframes sono { 0%, 100% { opacity: .12; } 50% { opacity: .55; } }
  .eyebrow { color: var(--tom-da-tela); }
  .acoes { display: flex; gap: 10px; flex-wrap: wrap; }
  .acao {
    display: inline-flex; align-items: center; min-height: 44px; padding: 0 18px; border-radius: 10px;
    font-size: 14px; font-weight: 700; transition: opacity .16s ease, transform .06s ease;
  }
  .acao:active { transform: translateY(1px); }
  .acao-principal { background: var(--accent); color: var(--accent-ink); }
  .acao-principal:hover { color: var(--accent-ink); opacity: .9; }
  .acao-segunda { border: 1px solid var(--line-2); color: var(--muted); font-weight: 600; }
  .acao-segunda:hover { color: var(--text); border-color: var(--muted); }
"""

def tela(nome, *, codigo, caminho, tom, sobrancelha, titulo, citacao, lead, acoes, ceu, depois_das_dunas="",
         css_extra="", ajuste=None):
    s = BASE
    s = s.replace("<title>404 — Ozymandias</title>", f"<title>{codigo} — Ozymandias</title>")
    s = s.replace('<span class="path">erro 404</span>', f'<span class="path">{caminho}</span>')
    s = s.replace('<div class="numeral"><span>404</span></div>', f'<div class="numeral"><span>{codigo}</span></div>')
    s = s.replace('<div class="eyebrow">Erro 404</div>', f'<div class="eyebrow">{sobrancelha}</div>')
    s = s.replace("<h1>Nada resta deste caminho</h1>", f"<h1>{titulo}</h1>")
    s = re.sub(r'<blockquote class="quote">.*?</blockquote>',
               f'<blockquote class="quote">{citacao}<cite>Shelley, 1818</cite></blockquote>', s, flags=re.S)
    s = re.sub(r'<p class="lead">.*?</p>', f'<p class="lead">{lead}</p>\n      <div class="acoes">{acoes}</div>', s, flags=re.S)
    s = s.replace('<a class="back" href="/ruinas" aria-label="Voltar para as ruínas">← Voltar</a>',
                  '<a class="back" href="/" aria-label="Voltar para o acervo">← Voltar</a>')
    # O céu de nuvens entra logo depois da lua (passa por cima dela) e antes
    # das dunas (a areia cobre a base).
    s = s.replace('    <svg class="dunes"', f'    {ceu}\n    <svg class="dunes"', 1)
    if depois_das_dunas:
        s = s.replace('    <div class="copy">', f'    {depois_das_dunas}\n    <div class="copy">', 1)
    s = s.replace("  @media (prefers-reduced-motion: reduce) {",
                  CSS_NUVEM + f"  :root {{ --tom-da-tela: {tom}; }}\n" + css_extra + "\n  @media (prefers-reduced-motion: reduce) {", 1)
    if ajuste:
        s = ajuste(s)
    for obrigatorio in (f"<span>{codigo}</span>", titulo, 'class="acoes"'):
        assert obrigatorio in s, f"{nome}: a 404 mudou e o gerador não achou {obrigatorio!r}"
    with open(os.path.join(RAIZ, nome + ".html"), "w", encoding="utf-8") as f:
        f.write(s)
    print("gerada", nome + ".html")

principal = lambda t, href: f'<a class="acao acao-principal" href="{href}">{t}</a>'
segunda = lambda t, href: f'<a class="acao acao-segunda" href="{href}">{t}</a>'

# 503 · modo local: três faixas fechando o céu sobre a lua.
tela("503", codigo=503, caminho="erro 503 · modo local", tom="var(--tom)",
     sobrancelha="Erro 503 · modo local", titulo="O céu está fechado",
     citacao="&ldquo;As areias, lisas e solitárias, se estendem ao longe.&rdquo;",
     lead="Este item mora só na nuvem, e o Ozymandias está no modo local: nenhuma chamada sai do Mac. "
          "Ele continua no catálogo e volta a tocar quando o híbrido for ligado.",
     acoes=principal("Ligar o híbrido", "/settings") + segunda("Voltar ao acervo", "/"),
     ceu=('<svg class="nuvens" viewBox="0 0 1600 400" preserveAspectRatio="xMinYMin slice" aria-hidden="true">'
          + faixa("corre-lento", [(40, 40, 620, "var(--nuvem)"), (880, 20, 520, "var(--nuvem)")])
          + faixa("corre-medio", [(0, 92, 780, "var(--nuvem)"), (960, 110, 600, "var(--nuvem)")])
          + faixa("corre-rapido", [(160, 150, 560, "var(--nuvem-2)"), (1040, 160, 480, "var(--nuvem-2)")])
          + '</svg>'),
     # Coberta, a lua perde força: o halo quase apaga.
     css_extra="  .lua-halo { opacity: .45; animation: none; }")

# 502 · o bucket não respondeu: névoa rasteira sobre as dunas, lua fraca.
tela("502", codigo=502, caminho="erro 502 · conectando", tom="var(--danger)",
     sobrancelha="Erro 502 · bucket sem resposta", titulo="Névoa no horizonte",
     citacao="&ldquo;Nada mais resta.&rdquo;",
     lead="Tentei falar com o bucket e ele não respondeu. Nada ficou pela metade: o Ozymandias voltou "
          "para o modo local, e tudo o que está no Mac continua tocando.",
     acoes=principal("Tentar de novo", "javascript:location.reload()") + segunda("Ficar no modo local", "/"),
     ceu="",
     depois_das_dunas=('<svg class="nevoas" viewBox="0 0 1600 300" preserveAspectRatio="xMinYMax slice" aria-hidden="true">'
                       + faixa("nevoa-1", [(0, 90, 900, "var(--nevoa)", "transparent"), (820, 120, 820, "var(--nevoa)", "transparent"), (400, 150, 700, "var(--nevoa)", "transparent")])
                       + faixa("nevoa-2", [(120, 200, 980, "var(--nevoa)", "transparent"), (1000, 220, 760, "var(--nevoa)", "transparent")])
                       + '</svg>'),
     css_extra="  .lua { opacity: .4; }\n  .lua-halo { animation: none; opacity: .3; }")

# 503 na instância cloud: o arquivo só existe no Mac. Céu limpo, a estátua
# com os olhos em brasa, respirando: o Mac está dormindo.
def olhos_em_brasa(s):
    velho = '<circle cx="59" cy="53" r="8" fill="var(--cut)"/>\n        <path d="M41 66 L41 78 L55 72 Z" fill="var(--cut)"/>'
    assert velho in s, "503-no-mac: a máscara da 404 mudou"
    return s.replace(velho, '<circle cx="59" cy="53" r="8" fill="var(--cut)"/>\n'
                     '        <circle class="olho-sono" cx="37" cy="53" r="5" fill="var(--accent)"/>'
                     '<circle class="olho-sono" cx="59" cy="53" r="5" fill="var(--accent)"/>\n'
                     '        <path d="M41 66 L41 78 L55 72 Z" fill="var(--cut)"/>', 1)

tela("503-no-mac", codigo=503, caminho="instância cloud", tom="var(--accent)",
     sobrancelha="No Mac · instância cloud", titulo="Isto dorme no Mac",
     citacao="&ldquo;Contemplai minha obra, ó Poderosos.&rdquo;",
     lead="Você está no Ozymandias da nuvem. Este arquivo só existe no disco do Mac, e o Mac está longe "
          "ou dormindo. Ele toca daqui quando ganhar uma cópia no bucket.",
     acoes=principal("Ver o que toca aqui", "/") + segunda("Voltar", "javascript:history.back()"),
     ceu=('<svg class="nuvens" viewBox="0 0 1600 400" preserveAspectRatio="xMinYMin slice" aria-hidden="true" style="opacity: .7">'
          + faixa("corre-lento", [(300, 230, 480, "var(--nuvem)"), (1100, 250, 420, "var(--nuvem)")])
          + '</svg>'),
     ajuste=olhos_em_brasa)
