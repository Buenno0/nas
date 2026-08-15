# NAS Ozymandias

Servidor de mídia pessoal: um binário Go com o frontend React embutido dentro
dele. Roda de qualquer pasta, indexa as suas pastas de filmes, séries, músicas
e fotos, e serve tudo numa interface estilo streaming — escura por padrão,
clara se você quiser.

Dois modos de acesso, **um por vez**:

1. **Local** — acessível na sua rede (`http://192.168.x.x:8787`)
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
  NAS Ozymandias

  1) Local    → acessível na sua rede
  2) Tunnel   → acessível pela internet (Cloudflare)
  q) Sair
```

No primeiro boot, o NAS cria o seu usuário e mostra uma senha aleatória no
terminal. Anote: ela não aparece de novo.

### Comandos

| Comando | O que faz |
|---|---|
| `nas` | menu interativo |
| `nas serve --local` | sobe direto no modo local |
| `nas serve --tunnel` | sobe com tunnel Cloudflare |
| `nas lib add <pasta>` | cadastra uma biblioteca (`--kind movie\|tv\|music\|photo`) |
| `nas lib ls` / `nas lib rm <id>` | lista / remove bibliotecas do índice |
| `nas scan [--force]` | indexa os arquivos e busca metadados |
| `nas meta [--all]` | só os metadados |
| `nas user add <nome> [--admin]` | cria outro usuário |
| `nas user ls` | lista contas e papéis |
| `nas user rm <nome>` | remove uma conta |
| `nas user promote\|demote <nome>` | muda o papel |
| `nas config [set <chave> <valor>]` | mostra ou altera a configuração |
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

O modo tunnel usa o `cloudflared`. Sem configuração nenhuma, ele abre um
*quick tunnel* e imprime uma URL `*.trycloudflare.com` — que muda a cada
execução. Para uma URL fixa no seu domínio:

```bash
cloudflared login
cloudflared tunnel create nas
nas config set tunnel nas
nas serve --tunnel
```

No modo tunnel o servidor escuta só em `127.0.0.1` (o cloudflared é a única
entrada) e o cookie de sessão vai com `Secure`.

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
├── scan/           walker, ffprobe, watcher
│   └── nameparse/  título/ano/SxxExx a partir do nome do arquivo
├── meta/           casamento com o TMDB
│   └── tmdb/       cliente da API
├── media/          miniaturas via ffmpeg
├── auth/           argon2id, sessões, rate limit
├── api/            handlers HTTP e streaming
├── tunnel/         supervisor do cloudflared
├── lock/           instância única
└── web/            frontend embutido
web/                React + TypeScript + Tailwind
```

## As ruínas (telas de erro)

As páginas de 401, 404 e 500 ficam em `web/public/telas-erro/` — HTML puro, um
arquivo cada, com tema claro e escuro por `prefers-color-scheme` e animação de
entrada (a estátua assenta na areia, a poeira sobe, o numeral respira). Tudo
desliga sozinho com `prefers-reduced-motion: reduce`.

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

A recusa acontece na API (403), não só na interface — esconder o botão não
protegeria nada. Ninguém vê o histórico de ninguém: progresso e favoritos são
por usuário.

O servidor nunca fica sem administrador: `demote` e `rm` recusam quando sobrou
um só.

## Limitações conhecidas

- **Sem transcodificação.** O arquivo é entregue como está (*direct play*). Se
  o navegador não souber tocar o codec — MKV com H.265, por exemplo — o player
  avisa e oferece o download em vez de mostrar uma tela preta.
- **Sem biblioteca privada.** Existem dois papéis (admin e usuário), mas o
  acervo é um só: não dá para dizer "esta pasta é só minha" nem criar um perfil
  infantil.
- **Legendas externas** (`.srt` ao lado do vídeo) ainda não são carregadas.
- O watcher observa até 2000 pastas; acima disso, as mudanças aparecem no scan
  periódico (padrão: a cada 6h, configurável em `scan_every`).
