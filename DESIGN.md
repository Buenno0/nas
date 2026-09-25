# Design system — NAS Ozymandias

Este documento descreve o que **existe no código**, não um ideal. Cada valor foi
lido dos arquivos; cada número de contraste foi calculado, e a conta está em
`scripts/contraste.mjs` — rode `node scripts/contraste.mjs` e ele devolve a
tabela do §8 e sai com erro se algum par reprovar. Onde o sistema se contradiz,
está marcado como contradição em vez de maquiado.

Última auditoria: 27/08/2026 — a virada para a paleta única.

---

## 1. A ideia

A marca é uma máscara funerária — Ozymandias, o rei da estátua em ruínas de
Shelley. Disso saem três decisões que se repetem em tudo:

- **Escuro é o padrão**, porque o produto é para assistir vídeo. O claro existe
  e é um tema completo, não um filtro invertido.
- **O acervo é o herói.** Interface em pedra e areia, cor forte só no que é
  ação. Pôster e backdrop são as únicas fontes de cor saturada.
- **Ruína como assunto — no produto inteiro.** Antes a ruína era só a moldura
  (login, erros) e o uso diário acontecia num cinza neutro qualquer. Agora a
  areia não para na porta.

---

## 2. Uma paleta, e por quê

O projeto tinha **três** conjuntos de cor: produto cinza-frio com acento
violeta, login em papel morno, telas de erro em areia saturada. Você entrava
pela areia e ia trabalhar no cinza. Agora é **um** conjunto, com três
declarações porque o código vive em três lugares:

| onde vive | arquivo | tokens |
|---|---|---|
| app inteiro depois do login | `web/src/index.css` | 8 base + 3 semânticos |
| tela de login | `web/src/pages/login.css` | prefixo `--lg-*` |
| telas de 401/404/500 | `web/public/telas-erro/*.html` | os mesmos, mais a cena |

Os valores são iguais; o que muda é o nome do token e os extras que só a cena
usa (dunas, pedra, poeira, o numeral gigante). São cópias à mão — ver
contradição nº 1 (§9).

### 2.1 Produto

Tokens em `:root, .dark` e `.light`, expostos ao Tailwind v4 via `@theme inline`
(`bg-bg`, `text-ink`, `border-line`, `bg-accent`, `text-warn`…).

| token | escuro | claro | uso |
|---|---|---|---|
| `--bg` | `#0c0a08` | `#f6f3ee` | fundo da página |
| `--surface` | `#16130f` | `#fdfbf7` | cartão, barra lateral, listas |
| `--elev` | `#211c15` | `#efe9df` | hover, chip, poço de imagem |
| `--line` | `#40372b` | `#d0c4b1` | toda borda e divisória |
| `--ink` | `#f4efe4` | `#241e17` | texto de corpo |
| `--muted` | `#a99a84` | `#6b6154` | texto secundário, ícone inativo |
| `--accent` | `#d29a44` | `#9d5a26` | ação primária, estado ativo, foco |
| `--accent-ink` | `#1a1206` | `#fdfbf7` | texto sobre o acento |

O escuro é **basalto**: pedra à noite, não cinza de painel. O acento escuro é o
**ouro gasto da touca da máscara** — a mesma cor, não uma prima dela. No claro
o acento é **terracota**, e o papel é morno em vez de areia saturada: a medida
prática de "amarelado" é a diferença entre o canal vermelho e o azul do fundo.

```
areia saturada   #efe0c8   R−B = 39   (as telas de erro, até esta auditoria)
papel morno      #f6f3ee   R−B =  8   (o claro inteiro, hoje)
```

Regra prática: **quatro níveis de profundidade** (`bg` → `surface` → `elev` →
borda) e nada além disso. Não existe sombra elevando cartão; separação é sempre
por borda de 1px em `--line`.

### 2.2 As três semânticas

Existem porque "verde" e "vermelho" soltos em classe utilitária eram a única
cor fora do sistema — eram 31 ocorrências de `text-amber-400`, `text-red-400`,
`bg-emerald-500/15` e afins espalhadas por sete arquivos. Aqui elas também são
ruína: pátina de bronze, ocre e óxido.

| token | escuro | claro | usos hoje |
|---|---|---|---|
| `--ok` | `#7fb79a` | `#2f6f52` | 3 |
| `--warn` | `#dca84a` | `#8a5a12` | 15 |
| `--danger` | `#e08163` | `#9c3f1d` | 11 |

Elas são o **teto**: nenhuma cor nova entra sem virar token. Se você precisou de
uma décima segunda cor, o problema provavelmente não é de paleta.

### 2.3 O código de status

401, 404 e 500 têm cada um a sua cor, e ela é a mesma nos dois lugares onde
aparecem — no menu Ruínas da barra lateral e na sobrancelha da tela de erro:

| status | token | onde |
|---|---|---|
| 401 entrada proibida | `--accent` | `Layout.tsx`, `401.html` |
| 404 caminho perdido | `--warn` | `Layout.tsx`, `404.html` |
| 500 colapso final | `--danger` | `Layout.tsx`, `500.html` |

### 2.4 A marca

Uma geometria só, quatro peças, duas iluminações. Definida em
`web/src/components/Mark.tsx` e replicada nos HTML autônomos e no favicon.

| peça | claro | escuro |
|---|---|---|
| corpo (escudo) | `#241e17` (`--ink`) | `#d29a44` (`--accent`) |
| touca | `#c98f3c` a 100% | `#1a1206` a 38% |
| olhos | `#f6f3ee` (`--bg`) | `#1a1206` |
| play | `#9d5a26` (`--accent`) | `#1a1206` |

No claro é a máscara vista à luz: pedra escura, touca de ouro, olhos de areia.
No escuro é a mesma máscara contra o basalto — o ouro passa a ser o corpo e o
resto vira sombra. **O corpo é sempre o acento ou a tinta do acento**: é essa
amarração que faz o botão primário e a marca serem visivelmente a mesma coisa.

Regras: nunca desenhar a marca com outra cor; nunca sobrepor a fundo saturado
(usar o corpo já é o contraste).

Variante de silhueta: nas dunas, os olhos viram fendas (`rect` arredondado) em
vez de círculos. É proposital — estátua distante e gasta —, e vale só ali.

### 2.5 A sala escura

Regra nova, e a paleta quente foi quem a exigiu: **chrome desenhado por cima de
mídia usa sempre os tokens do escuro**, mesmo com o tema claro ligado.

```css
.sala-escura { --accent: #d29a44; --accent-ink: #1a1206; --ink: #f4efe4;
               --muted: #a99a84; --ok/--warn/--danger: os do escuro }
```

Aplicada em três lugares: o `<div>` raiz do player (`Watch.tsx`), o hero da home
e o poço do pôster (a área da capa, não a legenda embaixo dela).

O motivo é medível. O acento claro é terracota `#9d5a26`; sobre o preto de um
vídeo ou sobre o degradê `from-black/85` do hero, ele vira um botão escuro em
fundo escuro. O acento violeta antigo não tinha esse problema porque as duas
versões tinham luminosidade parecida — trocar para uma paleta quente, onde o
claro é bem mais escuro que o escuro, é que criou a necessidade.

Fixa só o que é desenhado sobre a mídia: `--bg`, `--surface` e `--line`
continuam vindo do tema, porque ali nada os usa.

### 2.6 A lua

Só no tema escuro, no login e nas três telas de erro.

Não é enfeite: é a fonte de luz que faltava. A cena noturna tinha uma estátua
com a touca dourada e nada explicando de onde vinha o ouro. O tema claro já
tem sol — o degradê radial `--lg-glow` no topo — e acender os dois daria duas
fontes brigando com uma estátua que projeta sombra para um lado só.

| token | valor | papel |
|---|---|---|
| `--lua` | `#ded1b6` | o disco; osso pálido, 13,08:1 contra o céu |
| `--lua-cratera` | `#c3b28e` | as crateras, 1,38:1 contra o disco |
| `--lua-halo` | `#d29a44` | o halo — é o acento, e é o que amarra a lua ao ouro da touca |

**Gibosa minguante, 72% de face iluminada.** A fase se constrói em três passos
numa `<mask>`, e a ordem importa:

```svg
<circle cx="60" cy="60" r="27" fill="#fff"/>   <!-- o disco inteiro -->
<rect x="60" y="0" width="60" height="120" fill="#000"/>  <!-- meia sombra -->
<ellipse cx="60" cy="60" rx="12" ry="27" fill="#fff"/>    <!-- acende de volta -->
```

A elipse é o que faz o terminador **arquear para o lado escuro**. A tentativa
óbvia — um segundo círculo mordendo a borda — arqueia para o lado errado e
produz uma foice, não uma gibosa. `rx` sobre `ry` é a fase: 12/27 dá 72%.

As crateras repetem as órbitas vazadas da máscara, na mesma pedra gasta.

**Onde ela não aparece:** no claro — `display: none`, não `opacity: 0`, para o
halo não continuar clareando o papel.

**No celular ela muda de lugar, e nas duas telas de forma diferente**, porque
os dois layouts empilham de jeitos opostos:

| tela | desktop | ≤ 860/980px |
|---|---|---|
| login | céu, `top 9% / left 31%` | escondida — o céu vira faixa e sobra o título |
| erro | céu, `top 10% / left 9%` | horizonte, `bottom 29% / left 7%` |

Nas telas de erro a cópia sobe para o topo inteiro, então não há céu: a lua
desce para a faixa vazia à esquerda da estátua enterrada — que é onde uma lua
do deserto estaria mesmo. Essa consulta de mídia mora **junto do bloco da lua**,
não com as outras regras de celular do arquivo; lá em cima a regra de desktop,
declarada depois e com a mesma especificidade, desfazia a de celular.

**Três camadas, três donos.** Entrada, deriva e parallax animam todos
`transform`; no mesmo elemento, a última declarada apagaria as outras.

| elemento | anima | por quê |
|---|---|---|
| `<svg class="lua">` | parallax (style inline) | só nas telas de erro |
| `<g class="lua-corpo">` | `luaNasce` 1,6s | chega junto com a estátua |
| `<g class="lua-deriva">` | `luaDeriva` 90s | a maré lenta |
| `.lua-halo` | `luaBrilho` 14s | a respiração do halo |

No parallax ela é o objeto mais distante, então é o que menos se move: numeral
18/12px, estátua −9/−5, lua −4/−3.

**Encostar nela apaga.** Só onde há ponteiro de verdade (`@media (hover: hover)`),
para um toque no celular não deixar a cena apagada.

O alvo é um `<span>` HTML invisível, não o SVG, por duas razões:

1. **Recorte.** A caixa do `<svg>` inclui o halo inteiro; usá-la apagaria a lua
   com o ponteiro a meio palmo de distância. `inset: 27.5%` com borda redonda
   dá exatamente o disco (r=27 num viewBox de 120 são 45% da caixa). Encostar
   no brilho não é encostar na lua.
2. **O alvo não pode ser a coisa que se apaga.** Se o disco fosse o alvo, ao
   chegar em opacidade 0 ele deixaria de ser atingível, o hover se perderia, a
   lua voltaria — e ela piscaria com o ponteiro parado em cima. O `<span>` é
   invisível desde sempre e nunca muda de opacidade.

O span fica **antes** do `<svg>` no DOM, para `.lua-alvo:hover ~ .lua` alcançá-lo.

Apagar leva 0,45s e reacender 1,1s. A assimetria é o que faz ler como brasa que
esfria, e não como elemento que some.

### 2.7 O céu

Junto com a lua, e sob a mesma regra: só no escuro.

| token | valor | papel |
|---|---|---|
| `--estrela` | `#efe7d6` | estrelas e o rastro da cadente |

**Vinte e seis estrelas**, cada uma cintilando no seu ritmo — duração entre 5 e
9,8s e atraso próprio. Sem isso o céu inteiro pisca junto e parece um LED. As
coordenadas ficam num viewBox `0 0 800 400` ancorado no topo, com `cy` parando
em 232: abaixo disso são as dunas, e estrela em cima de areia é erro de desenho.

O campo fica **atrás da lua e das dunas** na ordem do DOM, que é a ordem de
pintura. Assim o halo lava as estrelas próximas e a areia cobre as de baixo, sem
ninguém precisar recortar o campo à mão.

**Uma estrela cadente**, rara de propósito: ciclo de 19s para 1,3s de risco. O
trajeto inteiro acontece nos primeiros 7% do ciclo e o resto é céu vazio
esperando a próxima. Uma que passasse a cada dois segundos viraria pisca-pisca,
não acontecimento.

Ela **não vive dentro do campo de estrelas**. O campo usa
`preserveAspectRatio="slice"`, que recorta as laterais quando o contêiner é mais
estreito que o viewBox — e a coluna do login é. Ancorado ali, o risco saía de
cena na metade do caminho. A cadente tem caixa própria, posicionada em
porcentagem como a lua, e o trajeto cabe dentro dela em qualquer largura.

### No celular

O mesmo problema da lua, e uma solução a mais. A cópia ocupa o topo inteiro,
então não há céu lá em cima:

| elemento | desktop | ≤ 860/980px |
|---|---|---|
| campo (erro) | tela cheia | faixa `top 46% / height 24%` |
| campo (login) | tela cheia | escondido |
| cadente (erro) | `top 13% / left 32%` | `bottom 44% / left 26%` |
| cadente (login) | `top 13% / left 52%` | escondida |

A faixa não é capricho: em retrato, `slice` num contêiner de tela cheia amplia
tanto que sobra um quinto da largura do campo — **seis estrelas de vinte e
seis**. Numa faixa com a proporção do próprio viewBox o recorte quase some, e
ela cai exatamente no céu que existe: abaixo da cópia, acima das dunas.

A primeira tentativa foi mascarar o topo do campo com um degradê. Funcionava
para tirar as estrelas de cima do texto e piorava o resto: somada ao recorte,
sobrava uma estrela.

### 2.8 Os PNG da marca

`web/public/icons/*.png` são a única variante **fixa**, porque iOS não troca
ícone por tema. São gerados por `scripts/gerar-icones.sh` a partir da mesma
geometria — se a paleta mudar, rode o script em vez de editar imagem à mão.

O degradê da placa **desce, não corre na diagonal**, e o motivo é técnico: PNG
comprime cada linha por diferença com a de cima, então um degradê vertical
deixa essa diferença constante. O mesmo desenho na diagonal ocupava 213 KB;
vertical, 73 KB.

---

## 3. Tipografia

Duas famílias, servidas pelo próprio NAS em `/fonts/` (nenhuma requisição a
terceiros; funciona offline).

| família | pesos | onde |
|---|---|---|
| **Archivo** | 400, 600, 700 | login e telas de erro |
| **JetBrains Mono** | 400, 500 | micro-rótulos, código, status, tempo do player |
| pilha do sistema | — | corpo do app depois do login |

Só o subset `latin` está embutido: todos os acentos do português cabem nele, e
cortar `latin-ext` derrubou o peso de 281 KB para 163 KB.

O produto passou a declarar a JetBrains Mono em `index.css` — antes ela existia
só no login e nas telas de erro, e as classes `font-mono` do produto caíam na
pilha do sistema. Os micro-rótulos das Ruínas mudaram de fonte sem que uma
linha de TSX fosse tocada.

### Escala em uso (contagem real de ocorrências)

| classe | tamanho | ocorrências | papel |
|---|---|---|---|
| `text-sm` | 14px | 58 | **padrão do produto** |
| `text-xs` | 12px | 40 | metadado, legenda, chip |
| `text-[11px]` | 11px | 11 | selo sobre pôster |
| `text-base` | 16px | 7 | título de prateleira |
| `text-2xl` | 24px | 6 | título de página |
| `text-lg` / `text-4xl` | 18/36px | 3 / 3 | prateleira no desktop, hero |
| `text-[10px]` | 10px | 3 | micro-rótulo mono |

O corpo do app é **14px**, não 16px: é uma interface de navegação densa, com
muita lista. Prosa longa (sinopse) fica em 14px com `leading-relaxed`.

Pesos: **400 e 500** na maior parte; `600` só em título de página e botão
primário; `700` apenas no `h1` do login e das telas de erro. Nada de 800/900.

### `.rotulo` — o micro-rótulo, agora uma receita

Estava repetido como pilha de utilitários em quatro lugares, cada um com um
tamanho e um tracking diferente (`text-[11px] font-semibold tracking-wider
text-muted uppercase` num, `text-xs font-medium tracking-wide` noutro). Virou
uma classe:

```css
.rotulo {
  font-family: var(--font-mono);
  font-size: 10px;
  font-weight: 500;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: var(--muted);
}
```

Vive dentro de `@layer components` de propósito: assim `text-accent` no mesmo
elemento ainda vence, como qualquer utilitário vence um componente. Fora da
camada, a classe crua ganharia do utilitário e a sobreposição viraria armadilha.

---

## 4. Forma e espaço

### Raio de canto — o que a contagem diz

| raio | ocorrências | uso |
|---|---|---|
| `rounded-lg` (8px) | 38 | botão, campo, item de lista |
| `rounded-full` | 33 | botão de ícone, avatar, pílula de status |
| `rounded-xl` (12px) | 11 | cartão, pôster, hero |
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
| métricas | `grid-cols-2 sm:grid-cols-4` |
| prateleira (home) | rolagem horizontal com `snap-x`, item de `w-32 sm:w-36 md:w-40` |

Pôster é sempre **2:3**; miniatura de vídeo e cartão de "continuar assistindo"
são **16:9**; foto é **1:1** na grade e proporção real no lightbox.

### Pôster sem capa — o arco quente

`gradientFor()` em `lib/format.ts` dá a cada título sem capa um bloco estável,
derivado do nome. O matiz **não passeia pelos 360°**: fica num arco de 95°, de
terracota (~25°) a bronze esverdeado (~120°).

```
antes   hue = hash % 360        oklch(0.45 0.13 h)   → rosa e ciano na grade
agora   hue = 25 + hash % 95    oklch(0.42 0.085 h)  → pedra, ocre, óxido
```

Era a cor mais fora do sistema em toda a interface: uma grade de pedra e areia
com um pôster magenta no meio. A croma também caiu — esses blocos são o fundo
de um título sem capa, não o assunto da tela.

### Breakpoints

`sm:` (54 usos) é o divisor de águas — celular versus resto. `lg:` (11) marca a
troca de navegação: barra lateral fixa acima de 1024px, barra inferior abaixo.
`md:` (5) e `xl:` (3) só ajustam densidade de grade.

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
| `icons` | `components/icons.tsx` | 25 ícones |

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
micro-rótulo      rotulo
sobre mídia       sala-escura no contêiner (§2.5)
```

Para conferir as receitas lado a lado sem subir o NAS, rode
`./scripts/folha-de-provas.sh`: ela lê o CSS **já compilado** em
`internal/web/dist`, mostra os onze tokens com o valor resolvido e desenha cada
receita nos dois temas. Provar o CSS de produção é o ponto — uma folha que
reimplementasse as receitas à mão provaria a folha, não o sistema.

### O botão do login deixou de ser exceção

Antes `.lg-submit` não seguia a receita do produto, porque o botão primário do
produto reprovava em contraste e o do login não podia reprovar junto. Agora os
dois são o mesmo par — corpo do acento, tinta escura:

```css
background: var(--lg-btn-bg);           /* = --accent */
color: var(--lg-btn-fg);                /* = --accent-ink */
border: 1px solid color-mix(in oklab, var(--lg-btn-bg) 84%, black);
box-shadow: inset 0 1px 0 color-mix(in oklab, white 22%, transparent);
```

Hover **escurece** (`color-mix … 91%, black`); o clique afunda 1px. Nada levita.

### Ícones

25 ícones em `icons.tsx`, todos com a mesma assinatura: `viewBox="0 0 24 24"`,
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
| 450ms / 1,1s | a lua apagando e reacendendo no hover (§2.6) |
| 200ms | abrir/fechar do menu Ruínas, escala do pôster |
| 300ms | esmaecimento de cor na troca de tema (só fallback) |
| **520ms** | círculo da troca de tema |
| 700–800ms | entrada em cascata do login (`lg-rise`) |
| 1,1–1,6s | areia subindo, estátua assentando e lua nascendo (`lg-sandUp`, `lg-settle`, `lg-luaNasce`) |
| 9–90s | laços ambientes: poeira, dunas, touca, halo e maré da lua, cintilar das estrelas (5–9,8s), cadente (19s) |

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

A troca ficou **mais** confortável com a paleta única: antes o círculo revelava
um cinza frio por cima de um cinza frio mais claro e a mudança de temperatura
não existia; agora os dois lados são a mesma pedra sob luz diferente.

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
3. **Erro** — `ErrorState` com o ícone de aviso em `--danger`, a mensagem real
   da API e "Tentar de novo".
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

Gerado por `node scripts/contraste.mjs` (fórmula WCAG 2.1). "Passa" = ≥ 4,5:1
para texto normal; micro-rótulo decorativo é avaliado em 3,0:1 e borda em 1,4:1,
que é o limiar onde a divisória ainda se enxerga.

**Produto — escuro**

| par | frente | fundo | contraste | AA |
|---|---|---|---|---|
| corpo sobre fundo | `#f4efe4` | `#0c0a08` | 17,24:1 | passa |
| corpo sobre superfície | `#f4efe4` | `#16130f` | 16,15:1 | passa |
| secundário sobre fundo | `#a99a84` | `#0c0a08` | 7,19:1 | passa |
| secundário sobre superfície | `#a99a84` | `#16130f` | 6,74:1 | passa |
| acento como texto | `#d29a44` | `#0c0a08` | 7,95:1 | passa |
| **botão primário** | `#1a1206` | `#d29a44` | **7,45:1** | **passa** |
| sucesso / aviso / perigo | — | `#16130f` | 8,06 / 8,59 / 6,60:1 | passa |
| borda sobre superfície | `#40372b` | `#16130f` | 1,59:1 | passa |

**Produto — claro**

| par | contraste | AA |
|---|---|---|
| corpo sobre fundo | 14,90:1 | passa |
| secundário sobre fundo | 5,48:1 | passa |
| acento como texto | 4,84:1 | passa |
| botão primário | 5,18:1 | passa |
| sucesso / aviso / perigo | 5,78 / 5,72 / 6,47:1 | passa |
| borda sobre fundo | 1,55:1 | passa |

**Login e ruínas — escuro**

| par | contraste | AA |
|---|---|---|
| corpo sobre cartão | 16,15:1 | passa |
| secundário sobre cartão | 6,74:1 | passa |
| micro-rótulo mono | 3,92:1 | passa (limiar 3,0) |
| botão primário | 7,45:1 | passa |
| tique sobre caixa marcada | 7,45:1 | passa |

**Login e ruínas — claro**

| par | contraste | AA |
|---|---|---|
| corpo sobre cartão | 15,96:1 | passa |
| secundário sobre cartão | 5,87:1 | passa |
| micro-rótulo mono | 4,16:1 | passa |
| **link terracota** | **4,84:1** | **passa** |
| botão primário | 5,18:1 | passa |

São **48 pares medidos, 48 passando.** As duas reprovações da auditoria
anterior estão fechadas: o botão primário do produto (3,86:1) e o link
terracota do login (4,49:1).

---

## 9. Contradições conhecidas

Escrito aqui porque um design system que esconde as próprias falhas serve para
nada. As quatro primeiras da auditoria anterior morreram na virada; estas são
as que ficaram, mais as que a virada criou.

1. **A paleta é uma, mas está escrita em quatro lugares.** `index.css`,
   `login.css`, os três HTML de erro e — de novo — `scripts/contraste.mjs`.
   Nada deriva de nada: são cópias de valor à mão. O script é o guarda-costas
   disso, não a cura; ele acusa contraste ruim, não divergência entre arquivos.
   Conserto de verdade: gerar os quatro a partir de uma fonte só no build.

   A lua (§2.6) e o céu (§2.7) pioraram isto: agora são quatro cópias de um
   **desenho**, não só de uma cor. Mexer na fase da lua significa editar a
   mesma `<mask>` em `Login.tsx` e nos três HTML; mexer no campo de estrelas
   significa reemitir as 26 coordenadas nos dois formatos. É o preço de as
   telas de erro serem arquivos autônomos que funcionam sem o React montado.

   O campo de estrelas ao menos tem uma fonte só: `scripts/estrelas.py` emite
   tanto o array TSX quanto os `<circle>` do HTML. Isso não está no build —
   é um gerador que se roda à mão quando o campo muda.

2. **A tipografia continua partida.** A cor deixou de mudar na fronteira do
   login, mas a fonte ainda muda: Archivo no login e nos erros, pilha do
   sistema no produto. O micro-rótulo agora é JetBrains Mono nos dois lados, o
   que só evidencia o resto. Ou o produto adota a Archivo (mais 105 KB de
   fonte), ou o login desce para a pilha do sistema e perde caráter.

3. **A tela de login tem CSS próprio** (`login.css`, ~680 linhas, fora do
   Tailwind). Foi a escolha certa para dunas e poeira em SVG, mas significa que
   um token novo precisa ser declarado nos dois lugares. É a causa raiz da
   contradição nº 1 e provavelmente não vale desfazer.

4. **O ícone de app não acompanha o tema** — limitação do iOS, não do desenho.
   Fica sempre na placa dourada, que é a variante fixa (§2.5).

5. **O texto sobre o pôster sem capa não é medido.** `gradientFor()` agora
   escolhe dentro de um arco quente, mas as iniciais em cima continuam fixas em
   `text-white/90`, sem conta de contraste por matiz. O arco tem luminosidade
   OKLCH travada em 0.42, o que torna o pior caso previsível — mas previsível
   não é medido.

6. **`--ok` tem três usos.** Um token com três ocorrências é quase uma cor
   solta com nome bonito. Ou aparece mais (confirmação de scan, arquivo
   preparado, sessão ativa), ou devia voltar a ser o que era.

---

## 10. Como mexer sem quebrar

- **Cor nova?** Primeiro procure entre os onze tokens do produto. Cor solta em
  classe utilitária (`text-red-400` e afins) não existe mais em lugar nenhum do
  `web/src` — foi tudo promovido a token, e a regra é manter assim.
- **Componente novo?** Comece copiando uma das receitas do §5, e confira o
  resultado na folha de provas. Se ele precisa de sombra para se separar do
  fundo, provavelmente devia ser uma borda.
- **Animação nova?** Verifique se ela sobrevive a `prefers-reduced-motion` e se
  a duração está na tabela do §6. Fora dessas faixas, justifique.
- **Cor de texto nova?** Acrescente o par em `scripts/contraste.mjs` e rode.
  Ele sai com erro se reprovar — dá para pendurar num hook de commit.
- **Alterou token do produto?** Ele existe em quatro lugares (§9.1). Faça a
  busca antes de dar por concluído, e rode `./scripts/gerar-icones.sh` se a
  mudança tocou o acento.
- **Desenhando por cima de capa ou vídeo?** Marque o contêiner com
  `sala-escura` (§2.5) e confira no tema **claro**, que é onde a falha aparece.
