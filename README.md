# Ozymandias

Servidor de mídia pessoal: um binário Go com o frontend React embutido dentro
dele. Roda de qualquer pasta, indexa as suas pastas de filmes, séries, músicas
e fotos, e serve tudo numa interface estilo streaming — escura por padrão,
clara se você quiser.

Dois modos de acesso à rede, **um por vez** (não confundir com o modo de nuvem
local/híbrido descrito em `PLANO-HIBRIDO.md`; os dois eixos são independentes):

1. **LAN** (`--local`) — acessível na sua rede (`http://192.168.x.x:8787`)
2. **Tunnel** — acessível pela internet via Cloudflare

## Instalação

Requisitos: Go 1.24+, Node 20+, ffmpeg. No macOS:

```bash
brew install go node ffmpeg cloudflared
```

Compilar e instalar o comando global:

```bash
make web && make install
```

Isso coloca o binário em `~/.local/bin/nas`. Se esse diretório não estiver no
seu `PATH`, adicione ao `~/.zshrc`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

## Uso

```bash
nas
```

Sem argumentos, abre o menu:

```
  Ozymandias

  1) Local    → acessível na sua rede
  2) Tunnel   → acessível pela internet (Cloudflare)
  q) Sair
```

No primeiro boot, o NAS cria o seu usuário e mostra uma senha aleatória no
terminal. Anote: ela não aparece de novo.

Ao iniciar, o endereço compartilhável é copiado automaticamente para a área
de transferência e aparece também como QR Code no terminal. No acesso LAN, o QR
aponta para o IP da rede; no quick tunnel, ele é atualizado assim que o domínio
`*.trycloudflare.com` fica disponível.

No acesso LAN o servidor também anuncia `_ozymandias._tcp.local` por DNS-SD.
Clientes de TV encontram nome, porta e versão da API sem exigir que o usuário
digite o IP, mesmo quando o roteador atribui outro endereço ao computador.

### Comandos

| Comando | O que faz |
|---|---|
| `nas` | menu interativo |
| `nas serve --local` | sobe direto no acesso LAN |
| `nas serve --tunnel` | sobe com tunnel Cloudflare |
| `nas lib add <pasta>` | cadastra uma biblioteca (`--kind movie\|tv\|music\|photo`) |
| `nas lib ls` / `nas lib rm <id>` | lista / remove bibliotecas do índice |
| `nas scan [--force]` | indexa os arquivos e busca metadados |
| `nas meta [--all]` | só os metadados |
| `nas user add <nome> [--admin]` | cria outro usuário |
| `nas user ls` | lista contas e papéis |
| `nas user rm <nome>` | remove uma conta |
| `nas user promote\|demote <nome>` | muda o papel |
| `nas passwd <nome>` | redefine a senha de alguém (recuperação de acesso) |
| `nas config [set <chave> <valor>]` | mostra ou altera a configuração |
| `nas modo [local\|hibrido]` | modo de nuvem; `local` é o kill switch |
| `nas push <arquivo> [--lib N]` | envia ao bucket (modo híbrido) |
| `nas status` / `nas stop` | estado da instância / encerra |

O tipo da biblioteca é inferido pelo nome da pasta quando possível
(`Filmes`, `Series`, `Musicas`, `Fotos`), então normalmente basta:

```bash
nas lib add ~/Media/Filmes
nas lib add ~/Media/Series
nas scan
```

### Capas e metadados (TMDB)

Sem configuração, a capa de cada item é um quadro extraído do próprio vídeo
(ou a arte embutida do MP3). Com uma chave gratuita do
[TMDB](https://www.themoviedb.org/settings/api), o NAS busca pôster, backdrop,
sinopse, nota, gêneros e nomes de episódio:

```bash
nas config set tmdb_key SUA_CHAVE
nas meta --all
```

As imagens são baixadas uma vez para `~/.nas/cache/`. O navegador nunca fala
com o TMDB — funciona offline depois de indexado.

Errou o match? Na página do título há o botão **Trocar capa**, que busca no
TMDB e deixa você escolher o item certo.

### Tunnel

O acesso Tunnel usa o `cloudflared`. Sem configuração nenhuma, ele abre um
*quick tunnel* e imprime uma URL `*.trycloudflare.com` — que muda a cada
execução. A URL é copiada automaticamente e o terminal mostra um QR Code
pronto para leitura pelo celular. Para uma URL fixa no seu domínio:

```bash
cloudflared login
cloudflared tunnel create nas
nas config set tunnel nas
nas serve --tunnel
```

No acesso Tunnel o servidor escuta só em `127.0.0.1` (o cloudflared é a única
entrada) e o cookie de sessão vai com `Secure`.

### Compressão dos assets

O `make web` comprime os arquivos do frontend depois do build
(`scripts/comprimir-assets.sh`), e o servidor entrega a variante `.gz` quando o
navegador aceita:

| | cru | comprimido |
|---|---|---|
| JavaScript | 343 KB | **103 KB** |
| CSS | 56 KB | **10 KB** |
| telas de erro | 15 KB cada | **4,5 KB** |
| **carga fria total** | **~399 KB** | **~114 KB** |

Comprimir no build em vez de a cada requisição: o nome dos arquivos tem hash e
o conteúdo nunca muda, então repetir a mesma compressão a cada visita seria só
gasto de CPU. Fontes (`.woff2`) e imagens (`.png`) ficam de fora — já vêm
comprimidas, e passar gzip nelas aumentaria o tamanho.

Isso importa **na rede local**, que é o uso principal: pelo tunnel a Cloudflare
já comprime por conta própria. Quem não manda `Accept-Encoding: gzip` continua
recebendo o arquivo cru.

Se você mexer no frontend sem passar pelo `make web`, rode `make comprimir`
para regerar os `.gz` — sem eles o servidor volta a entregar tudo cru, em
silêncio.

## Onde ficam as coisas

```
~/.nas/
├── config.json        porta, chave do TMDB, intervalo de scan
├── nas.db             índice (SQLite)
├── nas.pid            lock: garante um modo por vez
└── cache/
    ├── posters/       capas e backdrops
    └── thumbs/        miniaturas de fotos e vídeos
```

Nada é gravado nas suas pastas de mídia. Remover uma biblioteca (`nas lib rm`)
apaga só o índice — os arquivos ficam onde estão.

## Desenvolvimento

```bash
make run        # backend em :8787
make web-dev    # Vite em :5173 com proxy para o backend
make test       # testes Go
```

A interface em desenvolvimento roda pelo Vite (`:5173`) e conversa com o Go
por proxy. Em produção o `make web` gera `internal/web/dist`, que entra no
binário via `//go:embed`.

### Estrutura

```
cmd/nas/            entrypoint
internal/
├── cli/            comandos e menu
├── config/         ~/.nas/config.json
├── db/             SQLite + migrations embutidas
├── scan/           walker, ffprobe, EXIF, watcher
│   └── nameparse/  título/ano/SxxExx a partir do nome do arquivo
├── meta/           casamento com o TMDB
│   └── tmdb/       cliente da API
├── media/          miniaturas via ffmpeg
├── metrics/        CPU/RAM do processo e latência por rota
├── auth/           argon2id, sessões, rate limit
├── api/            handlers HTTP e streaming
├── tunnel/         supervisor do cloudflared
├── lock/           instância única
└── web/            frontend embutido
web/                React + TypeScript + Tailwind
```

## As ruínas (telas de erro)

As páginas de 401, 404 e 500 ficam em `web/public/telas-erro/` — HTML puro, um
arquivo cada, com tema claro e escuro que acompanha a escolha salva no NAS e
usa `prefers-color-scheme` como fallback. A animação de entrada faz a estátua
assentar na areia, a poeira subir e o numeral respirar. Tudo desliga sozinho
com `prefers-reduced-motion: reduce`.

As fontes (Archivo e JetBrains Mono) são servidas pelo próprio NAS, em
`web/public/fonts/` — as telas funcionam offline e nenhuma requisição sai para
terceiros. A versão anterior, sem animação, está guardada em
`assets/telas-erro-v1/` e não vai para o binário.

Elas são as telas de erro **de verdade** do servidor:

- qualquer endereço que não seja rota do app → 404 com a tela;
- um link de `/stream/...` aberto sem sessão → 401 com a tela (o SPA, que pede
  JSON, continua recebendo JSON).

E como erro bom é erro que ninguém vê, existe `/ruinas`: uma página pública com
três iscas que devolvem 401, 404 e 500 **de propósito**, com o status HTTP real.
Nada ali toca no acervo. O contador de visitas vive na memória do servidor e
some quando ele reinicia — não registra quem clicou.

## Transcodificação sob demanda

O NAS entrega o arquivo original sempre que o navegador consegue tocá-lo. Quando
não consegue, ele prepara uma versão compatível — gastando o mínimo necessário,
nesta ordem:

| o que falta | o que o NAS faz | custo medido nesta máquina |
|---|---|---|
| nada | entrega o original (*direct play*) | zero |
| só o container (`.mkv` com h264/aac) | troca a embalagem, sem recodificar | **830× tempo real** |
| só o áudio (AC3, DTS, TrueHD) | recodifica apenas o som para AAC | **137× tempo real** |
| a imagem (HEVC sem suporte, VC-1, H.264 High 10) | recodifica em H.264 até 1080p | **9× tempo real** |

Na prática: um filme de 2h remuxa em ~9 segundos e conserta áudio em ~53
segundos. Só a recodificação de imagem é lenta (~18 min para 2h47), e é o caso
raro.

**Quem decide é o servidor, com o que o cliente declarou.** O player pergunta ao
próprio navegador o que ele decodifica (`MediaSource.isTypeSupported`) e manda
junto no pedido — então Safari e Chrome no macOS, que tocam HEVC por hardware,
recebem o original e ninguém queima CPU à toa. Sem essa negociação, o mesmo
arquivo HEVC seria recodificado para um navegador que já sabia tocá-lo.

A negociação tem dois dialetos, e a diferença é o quanto o cliente sabe sobre si
mesmo:

| quem | manda | o que o servidor assume |
|---|---|---|
| o SPA | `?can=hevc,vp9,av1` | os padrões de navegador para container e áudio |
| um cliente nativo | `?vid=…&aud=…&cont=…` | só o que ele declarou; nenhum padrão vale |

Isso existe porque "o que um navegador toca" não é uma verdade universal. Todo
navegador abre WebM e toca Opus; o AVPlayer do iOS não faz nem uma coisa nem a
outra. Um cliente nativo declara os três conjuntos por inteiro — codecs de
vídeo, codecs de áudio, containers — e o que ele disse é a verdade. Os três são
independentes: declarar áudio não afrouxa o veredito sobre a imagem.

Conjunto omitido significa "usa o padrão", e é por isso que o SPA continua
funcionando sem mudar uma linha.

O veredito também considera coisas que o cliente não enxerga: `pix_fmt` e
perfil. H.264 **High 10** e 4:2:2 são h264 legítimos que decodificador de
hardware nenhum abre — nem navegador, nem VideoToolbox — e nenhum perfil compra
isso declarando "h264". A heurística antiga mandava esses arquivos para *direct
play* e o resultado era tela preta.

Os parâmetros que escolhem a receita (`can`, `vid`, `aud`, `cont`, `audio`)
entram na chave do cache de preparo. Precisam viajar **idênticos** nas quatro
chamadas de uma reprodução — consultar o plano, pedir o preparo, acompanhar o
progresso, buscar o pronto — ou o cliente espera por um preparo e pede outro.

### Cache

Os arquivos preparados ficam em `~/.nas/cache/preparados/`, com nome derivado de
(caminho, data de modificação, receita, versão da receita). Consequências:

- pedir o mesmo arquivo de novo é instantâneo;
- mexer no arquivo original invalida o preparo sozinho;
- mudar uma receita invalida o cache inteiro, sem limpeza manual.

O orçamento padrão é 8 GB (`cache_gb`), e o NAS **nunca** consome os últimos 5 GB
do disco (`reserva_gb`). Isso não é zelo estético: o cache divide o volume com o
`nas.db`, e encher o disco faria o SQLite devolver `SQLITE_FULL` — login e
progresso cairiam junto com a reprodução. Quando o orçamento estoura, sai o
arquivo preparado usado há mais tempo.

Dois processos ffmpeg no máximo (`trabalhos`), porque o motor de hardware da
Apple tem vazão fixa: mais processos apenas repartem a mesma banda.

### O que este pipeline não faz

- **Não começa a tocar antes de terminar de preparar.** O arquivo preparado é um
  MP4 completo, servido com Range — o seek fica perfeito e o progresso salvo
  continua valendo entre original e preparado, mas no caso de recodificar imagem
  você espera. HLS sob demanda (começar em segundos, gerar segmentos conforme o
  seek) é o próximo passo, e está medido: `-ss` antes do `-i` entrega o primeiro
  segmento em 0,71s mesmo a 1h30 de um arquivo de 5 GB.
- **Não escolhe qualidade.** Uma receita por caso, 1080p a 4 Mbit/s. Não há
  seleção de resolução nem bitrate por cliente.
- **Não transcodifica áudio multicanal para surround.** Tudo desce para estéreo
  AAC 160 kbit/s.
- **Não toca legenda embutida de MKV.** O preparo descarta legendas (`-sn`),
  porque MP4 não aceita o formato que costuma vir em Matroska.

## Contas e permissões

Quem instalou o servidor — o primeiro usuário criado — é **administrador**.
Só ele mexe no que afeta todo mundo:

| Ação | Admin | Usuário comum |
|---|---|---|
| Assistir, ouvir, ver fotos | sim | sim |
| Progresso e favoritos próprios | sim | sim |
| Trocar a própria senha e o tema | sim | sim |
| Corrigir a capa de um título | sim | sim |
| Disparar scan e buscar metadados | sim | **não** |
| Trocar a chave do TMDB | sim | **não** |
| Ver o caminho das pastas no disco | sim | **não** |
| Abrir o painel de métricas | sim | **não** |

A recusa acontece na API (403), não só na interface — esconder o botão não
protegeria nada. Ninguém vê o histórico de ninguém: progresso e favoritos são
por usuário.

O servidor nunca fica sem administrador: `demote` e `rm` recusam quando sobrou
um só.

### Clientes que não são o navegador

O SPA nunca vê o token da sessão dele: fica num cookie `HttpOnly`, que é o que
impede um XSS de roubá-lo. Um app nativo não tem esse problema nem esse luxo —
guarda a credencial ele mesmo. Duas portas a mais existem para isso:

- **`Authorization: Bearer <token>`** vale em qualquer rota protegida, no lugar
  do cookie. O token sai no corpo do login para quem pedir:
  `POST /api/auth/login` com `"token_na_resposta": true`. O SPA nunca pede.
- **`POST /api/auth/media-token`** devolve uma credencial curta que cabe numa
  URL, para ser usada como `?t=…` em `/stream`, `/preparado`, `/img/…` e nas
  legendas.

O token de mídia existe por um motivo específico: o AVPlayer do iOS abre a URL
do vídeo num processo próprio, sem o cookie jar nem os cabeçalhos do app. Sem
credencial na URL, sobra o `AVURLAssetHTTPCookiesKey` — que não sobrevive ao
AirPlay nem ao download em segundo plano.

É a única credencial deste servidor que viaja à vista, então foi desenhada para
vazar pouco:

- vale 6 horas, não 30 dias;
- abre a mídia e **nada** da API — nem `/api/home`, nem gerar outro token;
- morre junto com a sessão que a pediu (sair da conta ou trocar a senha derruba
  o link que estava aberto no player);
- morre junto com o processo: a chave que assina é sorteada no boot e nunca é
  gravada.

Use `?t=` só onde é necessário. Requisições que passam pelo cliente HTTP do app
— capas, legendas, a API inteira — devem usar o Bearer: as imagens são servidas
com `max-age=604800`, e uma URL com token na query vira uma entrada de cache
diferente a cada renovação.

## Métricas do servidor

`/metricas` na barra lateral (só administrador) mostra a saúde do processo em
tempo real, por SSE — uma conexão aberta em vez de uma pergunta por segundo.

**Nada disso é gravado.** Tudo vive em memória e volta a zero a cada
`nas serve`, no mesmo espírito do progresso de scan e do placar das ruínas. É um
painel para olhar agora, não uma série histórica: gravar métricas no `nas.db`
faria o banco crescer para sempre em troca de um gráfico que ninguém consulta.

### Recursos do processo

CPU, heap, pico de RSS e goroutines. A CPU sai da diferença entre duas leituras
de `getrusage` — o sistema só oferece tempo de CPU acumulado, então a
porcentagem instantânea exige comparar duas amostras. Um único goroutine amostra
a cada segundo e os handlers só leem o resultado pronto: dois leitores
concorrentes dividiriam a mesma janela de delta e cada um veria metade do valor.

Memória aparece em dois números porque nenhum conta a história sozinho —
**heap agora** sobe e desce, **pico de RSS** nunca desce, e mostrar só o pico
como "uso atual" seria mentira.

**Limitação assumida, não escondida:** isto mede o processo do NAS, não o
`ffmpeg`. Durante um preparo de vídeo a máquina sua, mas quem trabalha é um
processo filho — a CPU do painel continua baixa. O card diz isso na própria
tela.

### Disco

Barra de três segmentos: o que já está ocupado no volume, o que o cache de
preparo usa, e a reserva intocável. A reserva existe porque o cache divide o
volume com o `nas.db` e o WAL: encher o disco não derrubaria só a reprodução, o
SQLite passaria a devolver `SQLITE_FULL` e o login pararia junto.

### Tráfego HTTP

Requisições, latência e taxa de erro **por padrão de rota**, não por caminho:
`/api/titles/7` e `/api/titles/9` caem na mesma linha (`GET /api/titles/{id}`).
Agregar pelo caminho resolvido daria cardinalidade infinita — cada ID viraria
uma linha nova e a memória cresceria sem limite. A chave vem de `r.Pattern`, que
o `ServeMux` preenche durante o roteamento e continua legível no middleware
depois do `ServeHTTP`.

`GET /` junta o index com todos os assets estáticos: é literalmente o padrão que
o mux registrou para o SPA.

**Streaming e SSE ficam de fora.** Uma resposta de vídeo de duas horas entraria
na conta como "latência de 7200s" e destruiria qualquer percentil útil. A
exclusão é pelo `Content-Type` da resposta (`video/`, `audio/`,
`text/event-stream`), não por uma lista de rotas — assim uma rota de streaming
futura já nasce excluída sem ninguém precisar lembrar.

**p50 e p95 são estimativas.** Vêm de doze faixas de latência com interpolação
dentro da faixa (o mesmo método do Prometheus), não da lista de amostras: é o
que mantém a memória em O(1) por rota. O valor erra dentro da faixa. A média e o
**máximo**, esses, são exatos.

Duas correções que vieram da verificação ao vivo, e ficam registradas porque são
o tipo de erro que passa desapercebido:

- As faixas começavam em 0–10ms. Numa rota cujo pior caso real foi 1,01ms, o
  p50 interpolado dava **5ms** — número inventado. As faixas agora começam em
  1ms e 2,5ms, onde este servidor realmente vive.
- O percentil interpolado podia passar do máximo medido (95% de uma faixa de
  0–1ms dá 0,95ms mesmo quando a pior requisição levou 0,74ms). O máximo é
  medido, o percentil é estimado: agora o máximo tem a última palavra.

## Faixas de áudio e legendas

Um MKV "Dual Áudio" tem duas faixas; um MKV legendado tem as legendas dentro.
Até a versão anterior o NAS descartava as duas coisas em silêncio — o probe
guardava só o primeiro áudio e toda receita de transcodificação passava
`-map 0:a:0 -sn`. O próprio `nameparse` já reconhecia "dual", "dublado" e
"legendado" nos nomes dos arquivos: o acervo tinha, e o servidor jogava fora.

### Áudio

O player mostra um menu quando o arquivo tem mais de uma faixa, com o rótulo que
veio no arquivo ("Dublado", "Original") ou o idioma traduzido, mais a contagem
de canais em português ("estéreo", "5.1").

**Trocar de faixa obriga a reembalar o arquivo**, mesmo quando ele tocaria
direto. Não é escolha nossa: `<video>` não expõe troca de faixa de áudio fora do
Safari, então quem escolhe tem de ser o servidor, remontando o arquivo com aquela
faixa como a única. Por isso a faixa entra na chave do cache — sem isso, pedir o
áudio original devolveria o dublado que já estava em cache, para sempre.

O índice da faixa é validado contra o banco antes de virar argumento de `-map`:
esse número vem da URL, e aceitá-lo cru deixaria o cliente apontar para qualquer
stream do arquivo.

### Legendas

Legenda **não** é embutida no MP4. Ela sai por fora, convertida para WebVTT e
servida como `<track>` — que é o único jeito de o espectador poder ligar e
desligar sem o servidor reembalar o filme inteiro a cada clique.

Funcionam três origens:

- **Embutidas no container** (SRT, ASS, mov_text), extraídas sob demanda.
- **Em arquivo ao lado**: `filme.mkv` + `filme.srt`, `filme.pt-BR.srt`,
  `filme.eng.forced.srt`. O sufixo vira idioma e marca de "forçada" — é a
  convenção que todo mundo usa e ninguém documenta.
- **Nenhuma**: aí o menu não aparece.

Largar um `.srt` ao lado de um filme **já indexado** funciona sem
`nas scan --force`: o vídeo não mudou, então o scan normal não o reexaminaria,
e por isso as legendas em arquivo são reconciliadas à parte, comparando a
contagem por vídeo.

**Legenda de Blu-ray aparece na lista, desabilitada, com o motivo.** PGS e
DVD-sub são imagem, não texto: converter exigiria OCR. Escondê-las faria parecer
que o arquivo não tem legenda nenhuma.

## Linha do tempo das fotos

A galeria agrupa por mês, do mais recente para o mais antigo, lendo a data de
captura do EXIF.

O `ffprobe` **não** devolve EXIF de imagem — testado nesta máquina, `format.tags`
vem vazio para JPEG. E o projeto não traz dependência para isso. A leitura é
feita em `internal/scan/exif.go`, procurando a assinatura `Exif\0\0` no começo
do arquivo e lendo o TIFF que vem depois. Um caminho só cobre a foto do iPhone
(HEIC), a da câmera (JPEG) e a exportada (PNG/WebP), porque é o mesmo bloco EXIF
nos três.

O risco da busca por assinatura é falso positivo dentro dos dados da imagem, e
ele é fechado por três validações em sequência: ordem de bytes II ou MM, número
mágico 42 do TIFF, e uma data plausível.

Preferência entre as tags: `DateTimeOriginal` (o clique) → `DateTimeDigitized`
(virou arquivo) → `DateTime` (última edição). **Quando o arquivo não diz nada, o
mtime serve de reserva** — e uma câmera com o relógio zerado, que grava
`0000:00:00`, é tratada como "não diz", em vez de jogar a foto para 1970.

## Prateleiras da home

A home era fixa no código: recentes, favoritos e uma fileira por biblioteca. Um
acervo parado ficava com a mesma tela para sempre.

- **Neste dia** — fotos tiradas neste mesmo dia do calendário, em anos
  anteriores. É a única prateleira que muda sozinha todo dia.
- **Esquecidos** — começado há mais de 30 dias e nunca terminado.
- **Você nunca abriu** — sorteio entre o que nunca foi tocado, em ordem
  aleatória a cada carregamento. É o antídoto do acervo grande: sem isso, o que
  entrou há dois anos e nunca foi clicado nunca mais aparece.

**"Continuar assistindo" passou a olhar só os últimos 30 dias**, e o que é mais
antigo aparece em "Esquecidos", com esse nome. A união das duas é exatamente o
que a lista antiga mostrava — nada deixou de ser exibido, só parou de fingir que
o que você abandonou há um ano é a sessão de ontem.

Todas falham em silêncio: uma consulta com problema não pode derrubar a primeira
tela do aplicativo.

## Artistas

`titles.artist` existe desde o esquema inicial e nunca teve tela. Agora tem
`/artistas` e `/artista/:nome`, com discografia, todas as faixas, "tocar tudo" e
modo aleatório. O link só aparece na barra lateral quando existe biblioteca de
música: link para lista vazia é pior que link nenhum.

## Coleções

Listas montadas à mão — uma trilogia, uma maratona, o que ver com alguém. Até
aqui a única curadoria possível era `favorites`, um booleano por título: dava
para dizer "gosto disto", nunca "isto vai junto".

São **por usuário**, como progresso e favoritos. O dono é conferido dentro da
própria cláusula `WHERE` de cada operação — sem o `user_id` ali, o id da URL
bastaria para mexer na lista de outra pessoa. Coleção alheia responde **404, não
403**: dizer "proibido" já revelaria que ela existe.

A ordem é a que você escolher, não alfabética — uma trilogia fora de ordem não é
uma trilogia. A reordenação manda a lista inteira, não pares de troca: assim a
ordem final é sempre a que você está vendo, mesmo se dois arrastes chegarem
fora de ordem.

## Desempenho

O que foi medido nesta máquina (M4, 10 núcleos), com os números que sobreviveram
à medição — dois dos quatro itens que eu suspeitava renderam muito menos do que
eu estimava, e ficam registrados assim.

### Miniaturas: voo único e teto de paralelismo

Este era o problema de verdade. Uma grade de capas abrindo com o cache frio faz
o navegador pedir tudo de uma vez, e nada impedia que cada pedido virasse um
processo `ffmpeg`.

| cenário | antes | depois |
|---|---|---|
| 1 miniatura, sem disputa | 199 ms | 199 ms |
| 16 pedidos da **mesma** miniatura | **2,47 s**, 904% de CPU | **254 ms**, 1 processo |
| 16 miniaturas distintas | 16 processos | pico de 4 processos |

Duas mudanças, ambas em `internal/media/thumb.go`:

**Voo único por chave de cache.** Dezesseis pedidos da mesma miniatura eram
dezesseis `ffmpeg` produzindo byte por byte o mesmo JPEG. Agora o primeiro gera
e os outros esperam por ele.

**Teto de quatro gerações simultâneas.** O número saiu de medição, não de
palpite: com 16 miniaturas distintas, teto 1 dá 3,12 s, teto 4 dá 2,22 s e teto
16 dá 2,39 s. O trabalho é leitura de disco e decodificação, não cálculo — passar
de 4 não acelera nada e só rouba núcleos do streaming e da transcodificação, que
rodam ao mesmo tempo.

A geração roda solta, não dentro da requisição que a pediu: quem navegou para
outra página não deve poder matar o trabalho que outros clientes estão esperando,
e um JPEG abandonado a 90% é desperdício puro quando terminá-lo transforma o
próximo pedido em acerto de cache.

### Índice em `episodes.media_file_id`

Toda listagem de arquivos faz `LEFT JOIN episodes ON e.media_file_id = f.id`, e
essa coluna não tinha índice. O `EXPLAIN QUERY PLAN` mostrava o que realmente
acontecia:

```
SEARCH e USING AUTOMATIC COVERING INDEX (media_file_id=?)
```

O SQLite não fazia varredura por linha — ele construía um índice temporário a
cada execução e jogava fora no fim. Ou seja, a página de uma série pagava para
reindexar os episódios toda vez que era aberta.

**Ganho medido: 10%** (2,45 ms → 2,20 ms numa série de 1000 episódios), não os
43× que eu tinha estimado antes de medir. A estimativa estava errada: o custo
dominante da página não é o join, é o driver Go materializando 1000 linhas
(1,70 ms dos 2,20 ms são só executar a query e percorrer as linhas).

O índice fica porque continua sendo o certo — e por um ganho que o benchmark não
mede: `media_file_id` é `ON DELETE CASCADE`, então sem índice cada arquivo
removido num scan obrigava o SQLite a varrer a tabela de episódios inteira para
achar as filhas.

### Atime do cache de preparo

`Consultar` escrevia o atime do arquivo em cache a cada chamada — e ele é chamado
uma vez por carregamento de página, uma vez por segundo enquanto o SSE de preparo
está aberto e **uma vez por requisição Range**, ou seja, dezenas de vezes por
minuto durante uma reprodução com seek. Uma escrita de metadados em disco por
requisição, para nada: o cache é podado por "usado há mais tempo", e a diferença
entre "usado agora" e "usado há dez minutos" jamais muda quem é o mais antigo. O
toque agora tem folga de 10 minutos, e a checagem sai do `os.Stat` que já era
feito — nenhuma syscall extra.

### Pool de conexões do SQLite

Limitado a 8 conexões abertas e 8 ociosas. **Isto não é ganho de latência**: com
10 goroutines em paralelo a diferença contra o padrão do `database/sql` ficou
dentro do ruído (204µs contra 208µs por query). É teto de recurso — o padrão
abre conexões sem limite, cada uma com seu cache de páginas, e uma rajada podia
subir a memória sem dar vazão nenhuma, já que SQLite serializa escrita.

## Modo de nuvem: local ⇄ híbrido

Independente do acesso LAN/Tunnel, o Ozymandias tem um segundo eixo: o **modo
de nuvem**. O desenho completo está em `PLANO-HIBRIDO.md`; o que existe hoje é
o MVP.

- **local** (padrão): o Mac não faz nenhuma chamada à AWS. Itens que moram só
  no bucket continuam no catálogo, marcados "na nuvem, indisponível".
- **híbrido**: o bucket S3 é armazenamento de verdade. Itens da nuvem tocam
  direto do bucket (URL assinada de 1 h), sem passar os bytes pelo Mac.

```bash
nas config set nuvem.bucket ozymandias-midia
nas config set nuvem.regiao us-east-1
nas config set nuvem.perfil ozymandias   # perfil do ~/.aws/config (Roles Anywhere)
nas modo hibrido                         # grava e avisa o servidor no ar (SIGHUP)
nas modo local                           # kill switch
nas serve --sem-nuvem                    # break-glass: ignora o config nesta execução
```

O **kill switch** troca um estado atômico e cancela o contexto raiz de todo
trabalho de nuvem: uploads, assinaturas e ffmpeg lendo URLs do bucket. Todo
cliente HTTP de nuvem passa por um guard que, no modo local, recusa a
requisição e soma em `nuvem_bloqueadas_total` (visível em Configurações e em
`/api/metrics/status`). Falhar ao ligar o híbrido volta para local com o erro
na tela: nunca fica meio-conectado.

**Upload.** Em Configurações → *Enviar para a nuvem*, o navegador fatia o
arquivo e manda as partes direto ao S3 (multipart com URLs assinadas). O kill
switch pausa o envio; ao voltar o híbrido, ele retoma das partes que o bucket
já tem. Pelo terminal:

```bash
nas push ~/Filmes/Duna.mkv          # já está numa biblioteca: vira "no Mac e na nuvem"
nas push ~/Downloads/x.mkv --lib 1  # fora das bibliotecas: entra como item só da nuvem
```

O push também é retomável: rodar de novo continua de onde parou.

**Localização.** Cada arquivo é `local`, `enviando`, `ambos`, `baixando` ou
`nuvem`. O scan nunca apaga itens `nuvem`, e um item `ambos` cujo arquivo
local sumiu volta a ser `nuvem` em vez de sair do catálogo.

**Infra.** `infra/` tem o OpenTofu do bucket (versioning, CORS com
`ExposeHeaders: ETag`, que o upload do navegador exige, lifecycle de multipart
órfão, Intelligent-Tiering), da identidade (IAM Roles Anywhere, role restrita
a `bibliotecas/*`) e do Budgets. Copie `terraform.tfvars.exemplo` e rode
`tofu apply`; os outputs trazem o perfil do `~/.aws/config` e os comandos
`nas config`. Se você usar `nuvem.prefixo`, ajuste o `Resource` da policy.

**Sem SDK da AWS.** `go build -tags nocloud` gera um binário sem o SDK; o modo
local continua inteiro e o híbrido responde "sem suporte".

Teste do adapter contra MinIO (sem AWS): veja o comentário em
`internal/cloud/aws/s3_test.go`.

Ainda não existe (fases seguintes do plano): fixar/liberar espaço, outbox e
reconciliação, CloudFront, preparo (transcodificação) de itens da nuvem,
workers em Docker e instância cloud.

## Limitações conhecidas

- **Sem biblioteca privada.** Existem dois papéis (admin e usuário), mas o
  acervo é um só: não dá para dizer "esta pasta é só minha" nem criar um perfil
  infantil.
- **Legenda de imagem não converte.** PGS e DVD-sub são bitmaps: aparecem na
  lista desabilitadas, com o motivo, porque converter exigiria OCR.
- **Sem histórico de reprodução.** A tabela `progress` guarda só o estado atual
  (a chave é usuário + arquivo), então rever algo sobrescreve o registro
  anterior. Não existe "assistido em", nem contagem, nem retrospectiva.
- **A CPU do painel de métricas é a do processo do NAS**, não a do `ffmpeg`
  filho durante uma transcodificação.
- O watcher observa até 2000 pastas; acima disso, as mudanças aparecem no scan
  periódico (padrão: a cada 6h, configurável em `scan_every`).
