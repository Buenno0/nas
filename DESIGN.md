# Design system — NAS Ozymandias

Este documento descreve o que **existe no código**, não um ideal. Cada valor foi
lido dos arquivos; cada número de contraste foi calculado (WCAG 2.1, fórmula de
luminância relativa). Onde o sistema se contradiz, está marcado como
contradição em vez de maquiado.

Última auditoria: 17/08/2026.

---

## 1. A ideia

A marca é uma máscara funerária — Ozymandias, o rei da estátua em ruínas de
Shelley. Disso saem três decisões que se repetem em tudo:

- **Escuro é o padrão**, porque o produto é para assistir vídeo. O claro existe
  e é um tema completo, não um filtro invertido.
- **O acervo é o herói.** Interface em tons neutros, cor forte só no que é
  ação. Pôster e backdrop são as únicas fontes de cor saturada.
- **Ruína como assunto.** Areia, pedra e um dourado gasto aparecem nas
  superfícies de borda do produto (login, erros), não no uso diário.

---

## 2. Três paletas, e por quê

O projeto tem **três** conjuntos de cor. Não é acidente, mas é o ponto que mais
pede atenção de quem for mexer.

| paleta | onde vive | arquivo |
|---|---|---|
| **Produto** | app inteiro depois do login | `web/src/index.css` |
| **Papel** | tela de login | `web/src/pages/login.css` |
| **Ruína** | telas de 401/404/500 | `web/public/telas-erro/*.html` |

Produto é neutro e frio; Papel e Ruína são quentes. A troca acontece na
fronteira do produto: você entra pela areia e trabalha no cinza.

### 2.1 Produto

Tokens em `:root, .dark` e `.light`, expostos ao Tailwind v4 via `@theme inline`
(`bg-bg`, `text-ink`, `border-line`, `bg-accent`…).

| token | escuro | claro | uso |
|---|---|---|---|
| `--bg` | `#0a0b0e` | `#f6f7fa` | fundo da página |
| `--surface` | `#141720` | `#ffffff` | cartão, barra lateral, listas |
| `--elev` | `#1c2029` | `#eef1f6` | hover, chip, poço de imagem |
| `--line` | `#262b37` | `#e0e5ee` | toda borda e divisória |
| `--ink` | `#eef1f7` | `#12141c` | texto de corpo |
| `--muted` | `#98a2b8` | `#566072` | texto secundário, ícone inativo |
| `--accent` | `#7c6cff` | `#5b45f5` | ação primária, estado ativo, foco |
| `--accent-ink` | `#ffffff` | `#ffffff` | texto sobre o acento |

Regra prática: **quatro níveis de profundidade** (`bg` → `surface` → `elev` →
borda) e nada além disso. Não existe sombra elevando cartão; separação é sempre
por borda de 1px em `--line`.

### 2.2 Papel (login)

Prefixo `--lg-*`, escopo `.tela-login`, tema pela classe do `<html>`.

| token | escuro | claro |
|---|---|---|
| `--lg-bg` | `#0d0f14` | `#f6f3ee` |
| `--lg-card` | `#14161d` | `#fdfbf7` |
| `--lg-field` | `#0e1015` | `#f1ece4` |
| `--lg-line` / `-2` / `-3` | `#23262f` / `#2b3040` / `#3a4152` | `#e2dbd0` / `#cec5b6` / `#b0a695` |
| `--lg-text` | `#f2f3f7` | `#241e17` |
| `--lg-muted` | `#8b90a0` | `#665e53` |
| `--lg-faint` | `#5f6577` | `#7d7466` |
| `--lg-btn-bg` / `--lg-btn-fg` | `#8b7cff` / `#12101f` | `#2b2118` / `#f7f4ee` |
| dunas `--lg-dune-0..3` | `#151821` → `#232939` | `#e9e2d6` → `#d4c9b5` |
| estátua `--lg-stone` / `-2` | `#333850` / `#3f4661` | `#2b2118` / `#c98f3c` |

O claro é **papel morno, não areia**. A diferença entre canal vermelho e azul do
fundo é a medida prática de "amarelado":

```
areia saturada   #efe0c8   R−B = 39   (versão anterior, puxava para o amarelo)
papel morno      #f6f3ee   R−B =  8   (atual)
```

### 2.3 Ruína (telas de erro)

Mesma estrutura, com tokens próprios para a cena: `--numeral` (o número gigante
atrás do texto), `--stone`, `--cut`, `--dune-0..3`, `--mote` (poeira) e o grupo
`--mark-*` que pinta a marca.

O claro aqui **ainda é a areia saturada** (`#efe0c8`). É a contradição conhecida
nº 1 (§9).

### 2.4 A marca

Uma geometria só, quatro peças, duas paletas. Definida em
`web/src/components/Mark.tsx` e replicada nos HTML autônomos.

| peça | escuro | claro |
|---|---|---|
| corpo (escudo) | `#8b7cff` | `#2b2118` |
| touca | `#0d0f14` a 35% | `#c98f3c` a 100% |
| olhos | `#0d0f14` | `#efe0c8` |
| play | `#0d0f14` | `#a8642c` |

Regras: nunca desenhar a marca com outra cor; nunca sobrepor a fundo saturado
(usar o corpo já é o contraste); o ícone de app (`icons/appicon-*.png`) é a
única variante **fixa**, porque iOS não troca ícone por tema.

Variante de silhueta: nas dunas, os olhos viram fendas (`rect` arredondado) em
vez de círculos. É proposital — estátua distante e gasta —, e vale só ali.

---

## 3. Tipografia

Duas famílias, servidas pelo próprio NAS em `/fonts/` (nenhuma requisição a
terceiros; funciona offline).

| família | pesos | onde |
|---|---|---|
| **Archivo** | 400, 600, 700 | login e telas de erro |
| **JetBrains Mono** | 400 (500 nos erros) | micro-rótulos, código, tempo do player |
| pilha do sistema | — | app depois do login |

Só o subset `latin` está embutido: todos os acentos do português cabem nele, e
cortar `latin-ext` derrubou o peso de 281 KB para 163 KB.

### Escala em uso (contagem real de ocorrências)

| classe | tamanho | ocorrências | papel |
|---|---|---|---|
| `text-sm` | 14px | 52 | **padrão do produto** |
| `text-xs` | 12px | 34 | metadado, legenda, chip |
| `text-base` | 16px | 5 | título de prateleira |
| `text-2xl` | 24px | 5 | título de página |
| `text-[11px]` / `text-[10px]` | 11/10px | 11 | selo sobre pôster, micro-rótulo mono |

O corpo do app é **14px**, não 16px: é uma interface de navegação densa, com
muita lista. Prosa longa (sinopse) sobe para 14px com `leading-relaxed`.

Pesos: **400 e 500** na maior parte; `600` só em título de página e botão
primário; `700` apenas no `h1` do login. Nada de 800/900.

---

## 4. Forma e espaço

### Raio de canto — o que a contagem diz

| raio | ocorrências | uso |
|---|---|---|
| `rounded-lg` (8px) | 37 | botão, campo, item de lista |
| `rounded-full` | 26 | botão de ícone, avatar, pílula de status |
| `rounded-xl` (12px) | 9 | cartão, pôster, hero |
| `rounded-md` (6px) | 6 | chip, selo pequeno |
| `rounded-2xl` (16px) | 4 | diálogo, cartão do login |

Regra: **quanto maior a superfície, maior o raio**. Controles 8px, contêineres
12px, sobreposições 16px. O login usa 11px nos campos e no botão — valor próprio
daquela tela, herdado do desenho original.

### Espaçamento

Escala do Tailwind, com uma convenção de página:

- padding horizontal de página: `px-4 sm:px-6`
- respiro vertical entre seções: `space-y-6` (configurações) e `space-y-10` (home)
- gap dentro de cartão: `gap-3`
- gap entre pôsteres: `gap-3 sm:gap-4`
- rodapé com folga para o mini player: `pb-40 lg:pb-28`

### Grades

| tela | grade |
|---|---|
| biblioteca / busca | `grid-cols-3 sm:grid-cols-4 md:grid-cols-5 xl:grid-cols-7` |
| fotos | `grid-cols-3 sm:grid-cols-4 md:grid-cols-6 xl:grid-cols-8` |
| prateleira (home) | rolagem horizontal com `snap-x`, item de `w-32 sm:w-36 md:w-40` |

Pôster é sempre **2:3**; miniatura de vídeo e cartão de "continuar assistindo"
são **16:9**; foto é **1:1** na grade e proporção real no lightbox.

### Breakpoints

`sm:` (48 usos) é o divisor de águas — celular versus resto. `lg:` (11) marca a
troca de navegação: barra lateral fixa acima de 1024px, barra inferior abaixo.
`md:` e `xl:` só ajustam densidade de grade.

---

## 5. Componentes

Inventário do que existe, com onde mexer.

| componente | arquivo | notas |
|---|---|---|
| `Layout` | `components/Layout.tsx` | barra lateral (desktop) + barra inferior (celular) + topo com busca |
| `Mark` | `components/Mark.tsx` | a marca; SVG inline, troca de paleta pelo tema |
| `Poster` / `PosterSkeleton` | `components/Poster.tsx` | cartão 2:3 com selo de tipo e barra de progresso |
| `Row` / `RowItem` | `components/Row.tsx` | prateleira horizontal com setas no desktop |
| `MiniPlayer` | `components/MiniPlayer.tsx` | barra fixa de música, sobrevive à navegação |
| `PhotoGrid` | `components/PhotoGrid.tsx` | grade quadrada + lightbox com teclado |
| `states` | `components/states.tsx` | `Spinner`, `EmptyState`, `ErrorState` |
| `icons` | `components/icons.tsx` | 21 ícones |

### Receitas (as classes que se repetem)

```
cartão            rounded-xl border border-line bg-surface p-5
botão primário    rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-ink
                  transition hover:opacity-90 disabled:opacity-60
botão secundário  rounded-lg border border-line bg-surface px-4 py-2 text-sm
                  font-medium transition hover:bg-elev
campo             rounded-lg border border-line bg-bg px-3 py-2 text-sm
                  outline-none focus:border-accent
chip / selo       rounded-md bg-elev px-2 py-0.5 text-[11px] text-muted
botão de ícone    grid h-9 w-9 place-items-center rounded-full border border-line
                  bg-surface text-muted transition hover:text-ink
```

### O botão do login é exceção deliberada

`.lg-submit` não segue a receita do produto. Ele repete a construção da marca —
corpo colorido, tinta escura — sem gradiente, sem brilho colorido, sem ícone
decorativo:

```css
background: var(--lg-btn-bg);           /* violeta no escuro, marrom no claro */
color: var(--lg-btn-fg);                /* tinta escura / papel */
border: 1px solid color-mix(in oklab, var(--lg-btn-bg) 84%, black);
box-shadow: inset 0 1px 0 color-mix(in oklab, white 22%, transparent);
```

Hover **escurece** (`color-mix … 91%, black`); o clique afunda 1px. Nada levita.

### Ícones

21 ícones em `icons.tsx`, todos com a mesma assinatura: `viewBox="0 0 24 24"`,
traço `1.8`, pontas e junções redondas, tamanho `1.25em`, cor herdada de
`currentColor`. Preenchimento sólido só em `PlayIcon` e `PauseIcon` (onde a
silhueta é o significado) e em `HeartIcon` quando ativo.

Para adicionar: usar o componente `Icon` interno, jamais colar um SVG de
biblioteca com outro traço — a diferença de peso aparece na hora ao lado dos
outros.

---

## 6. Movimento

Nada anima por decoração no produto; anima no login e nas telas de erro, onde a
cena é o conteúdo.

| duração | uso |
|---|---|
| 60ms | afundar do botão no clique |
| 140–160ms | cor de hover, borda de foco |
| 200ms | abrir/fechar do menu Ruínas, escala do pôster |
| 300ms | esmaecimento de cor na troca de tema (só fallback) |
| **520ms** | círculo da troca de tema |
| 700–800ms | entrada em cascata do login (`lg-rise`) |
| 1,1–1,6s | areia subindo e estátua assentando (`lg-sandUp`, `lg-settle`) |
| 9–90s | laços ambientes: poeira, deriva das dunas, brilho da touca |

### Troca de tema

View Transitions API: o tema novo é revelado por um **círculo que cresce a
partir do botão clicado**, 520ms, `cubic-bezier(0.4, 0, 0.2, 1)`. O raio é a
hipotenusa até o canto mais distante.

O esmaecimento cruzado padrão da API fica **desligado** — os dois retratos
sobrepostos lavam a tela no meio do caminho:

```css
::view-transition-old(root), ::view-transition-new(root) { animation: none }
::view-transition-old(root) { z-index: 1 }
::view-transition-new(root) { z-index: 2 }
```

Sem a API (Firefox hoje), entra um esmaecimento de 300ms nas cores, ligado
apenas durante a troca por `[data-tema-trocando]` — deixá-lo sempre ativo faria
cada hover herdar 300ms de atraso.

### Movimento reduzido

`prefers-reduced-motion: reduce` desliga tudo: as animações das telas
autônomas, o círculo da troca de tema (que nem é iniciado) e as transições do
produto. É respeitado em `index.css`, `login.css` e nos três HTML de erro.

---

## 7. Estados

Todo pedido de rede tem quatro caras, e as três primeiras existem de verdade
como componente:

1. **Carregando** — `Spinner` centralizado; nas grades, `PosterSkeleton` com
   `animate-pulse` no mesmo formato do conteúdo final.
2. **Vazio** — `EmptyState` com título, explicação e uma ação. Nunca uma tela
   branca ("Seu acervo está vazio" leva para as configurações).
3. **Erro** — `ErrorState` com o ícone de aviso, a mensagem real da API e
   "Tentar de novo".
4. **Pronto** — o conteúdo.

Estados de erro específicos que valem citar:

- **Formato não suportado** (`Watch.tsx`): distingue container de codec e mostra
  o comando `ffmpeg` certo para cada caso, em vez de uma tela preta.
- **401 / 404 / 500**: telas próprias, com o status HTTP real (§2.3).
- **Caps Lock** no login: aviso enquanto a tecla está ativa.

### Foco

`:focus-visible` global: contorno de 2px na cor do acento, deslocado 2px. Campos
mudam a borda para o acento; o botão do login usa contorno com deslocamento de
3px. Nada de `outline: none` sem substituto.

---

## 8. Contraste medido

Calculado com a fórmula WCAG 2.1. "Passa" = ≥ 4,5:1 (texto normal); micro-rótulo
decorativo é avaliado em 3,0:1.

**Produto — escuro**

| par | frente | fundo | contraste | AA |
|---|---|---|---|---|
| corpo sobre fundo | `#eef1f7` | `#0a0b0e` | 17,39:1 | passa |
| corpo sobre superfície | `#eef1f7` | `#141720` | 15,82:1 | passa |
| secundário sobre fundo | `#98a2b8` | `#0a0b0e` | 7,68:1 | passa |
| secundário sobre superfície | `#98a2b8` | `#141720` | 6,99:1 | passa |
| acento como texto | `#7c6cff` | `#0a0b0e` | 5,10:1 | passa |
| **botão primário** | `#ffffff` | `#7c6cff` | **3,86:1** | **reprova** |

**Produto — claro**

| par | contraste | AA |
|---|---|---|
| corpo sobre fundo | 17,16:1 | passa |
| secundário sobre fundo | 5,92:1 | passa |
| botão primário | 5,81:1 | passa |
| acento como texto | 5,42:1 | passa |

**Login — escuro**

| par | contraste | AA |
|---|---|---|
| corpo | 17,29:1 | passa |
| secundário | 5,67:1 | passa |
| micro-rótulo mono | 3,11:1 | passa (limiar 3,0) |
| botão primário | 5,74:1 | passa |

**Login — claro**

| par | contraste | AA |
|---|---|---|
| corpo | 14,90:1 | passa |
| secundário | 6,17:1 | passa |
| micro-rótulo mono | 4,45:1 | passa |
| botão primário | 14,35:1 | passa |
| **link terracota** | **4,49:1** | **reprova por 0,01** |

---

## 9. Contradições conhecidas

Escrito aqui porque um design system que esconde as próprias falhas serve para
nada.

1. **Dois temas claros.** Produto é cinza frio (`#f6f7fa`); login e erros são
   quentes. Sai da areia e entra no cinza ao fazer login. Conserto: escolher um
   lado — ou esquentar levemente o produto, ou esfriar o login.

2. **A areia antiga sobrou nas telas de erro.** O login virou papel morno
   (`#f6f3ee`), os erros continuam em `#efe0c8`. Mesma troca de paleta resolve.

3. **Botão primário do produto reprova em contraste** (3,86:1 no escuro).
   Afeta "Assistir", "Abrir", "Escanear agora", "Tocar álbum". A correção é uma
   linha: `--accent-ink` deixa de ser branco e vira tinta escura no tema escuro,
   como já é no login.

4. **O link terracota do login perde por 0,01** (4,49 contra 4,5). Escurecer
   para `#9d5a26` resolve com folga.

5. **O ícone de app não acompanha o tema** — limitação do iOS, não do desenho.
   Fica sempre na variante violeta com gradiente.

6. **A tela de login tem CSS próprio**, fora do Tailwind (`login.css`, ~600
   linhas). Foi a escolha certa para dunas e poeira em SVG, mas significa que
   um token novo precisa ser declarado nos dois lugares.

---

## 10. Como mexer sem quebrar

- **Cor nova?** Primeiro procure entre os oito tokens do produto. Cor solta em
  classe utilitária (`text-red-400` e afins) só aparece hoje em erro e aviso —
  se precisar de mais, promova a token.
- **Componente novo?** Comece copiando uma das receitas do §5. Se ele precisa de
  sombra para se separar do fundo, provavelmente devia ser uma borda.
- **Animação nova?** Verifique se ela sobrevive a `prefers-reduced-motion` e se
  a duração está na tabela do §6. Fora dessas faixas, justifique.
- **Cor de texto nova?** Meça o contraste antes de commitar. As tabelas do §8
  foram geradas com a fórmula WCAG; refaça a conta em vez de confiar no olho.
- **Alterou token do produto?** Ele existe em dois lugares (`index.css` e, se
  aparecer no login, `login.css`). Faça a busca antes de dar por concluído.
