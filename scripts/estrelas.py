#!/usr/bin/env python3
"""Campo de estrelas do céu noturno — a fonte única das duas cópias.

O mesmo campo precisa existir em dois formatos: um array TSX para o login
(React) e <circle> soltos para as três telas de erro (HTML autônomo, que roda
sem o React montado). Manter os dois à mão garantiria que um dia divergissem.

    ./scripts/estrelas.py tsx    → o array para web/src/pages/Login.tsx
    ./scripts/estrelas.py html   → os <circle> para web/public/telas-erro/*.html

Não roda no build: é um gerador de mexer à mão quando o campo muda. Depois de
rodar, cole a saída no lugar correspondente e refaça `make web`.
"""
import sys

# viewBox 0 0 800 400, ancorado no topo. `cy` para em 232 porque abaixo disso
# são as dunas — estrela em cima de areia é erro de desenho, não de opacidade.
# Cada uma cintila no seu ritmo: duração e atraso distintos, senão o céu
# inteiro pisca junto e parece um LED.
ESTRELAS = [
    ( 38,  52, 1.1, '6.5s', '-0.4s'),
    ( 96, 118, 0.8, '8.0s', '-3.1s'),
    (134,  30, 1.4, '5.5s', '-1.9s'),
    (168, 176, 0.9, '9.0s', '-5.6s'),
    (212,  76, 1.2, '7.0s', '-2.3s'),
    (247, 142, 0.7, '6.0s', '-4.8s'),
    (286,  24, 1.0, '8.5s', '-0.9s'),
    (318, 196, 1.3, '5.0s', '-6.2s'),
    (352,  96, 0.8, '7.5s', '-3.7s'),
    (391,  46, 1.5, '6.2s', '-1.2s'),
    (424, 158, 1.0, '9.5s', '-5.1s'),
    (462, 108, 0.7, '5.8s', '-2.6s'),
    (498,  62, 1.2, '8.2s', '-4.3s'),
    (531, 208, 0.9, '6.8s', '-0.6s'),
    (569,  34, 1.1, '7.8s', '-6.9s'),
    (604, 128, 1.4, '5.3s', '-2.0s'),
    (641,  88, 0.8, '9.2s', '-4.6s'),
    (676, 182, 1.0, '6.6s', '-1.5s'),
    (712,  56, 1.3, '8.8s', '-3.4s'),
    (748, 146, 0.9, '5.6s', '-5.9s'),
    (778,  98, 1.1, '7.2s', '-2.8s'),
    ( 62, 172, 1.0, '8.6s', '-6.4s'),
    (270, 116, 0.9, '6.4s', '-1.7s'),
    (452, 226, 0.8, '7.6s', '-4.1s'),
    (588, 152, 0.7, '9.8s', '-0.2s'),
    (156, 100, 1.2, '6.9s', '-5.3s'),
]


def tsx() -> str:
    return '\n'.join(
        f"  {{ cx: {cx}, cy: {cy}, r: {r}, duracao: '{d}', atraso: '{a}' }},"
        for cx, cy, r, d, a in ESTRELAS
    )


def html(indent: str = '        ') -> str:
    return '\n'.join(
        f'{indent}<circle class="estrela" cx="{cx}" cy="{cy}" r="{r}" '
        f'fill="var(--estrela)" style="animation-duration:{d}; animation-delay:{a};"/>'
        for cx, cy, r, d, a in ESTRELAS
    )


if __name__ == '__main__':
    formato = sys.argv[1] if len(sys.argv) > 1 else ''
    if formato == 'tsx':
        print(tsx())
    elif formato == 'html':
        print(html())
    else:
        print(__doc__.strip(), file=sys.stderr)
        sys.exit(2)
