# Ozymandias Híbrido: modos local ⇄ híbrido com kill switch

> Plano arquitetural. Nada foi implementado. Todas as investigações foram de leitura.

## Contexto

Você quer transformar o Ozymandias (servidor de mídia em Go com React embutido, rodando
num MacBook Air M4) num **sistema híbrido totalmente desacoplável**:

- **Dois modos**, alternáveis livremente a qualquer momento: **local** (zero AWS) e
  **híbrido** (Mac + AWS).
- **Kill switch com corte instantâneo.** No modo local, o Mac não faz nenhuma chamada à
  AWS. Itens que existem só na nuvem aparecem como *"na nuvem, indisponível"* e voltam
  quando o híbrido é reativado.
- **O bucket é armazenamento de verdade.** O arquivo mora no S3 e toca de lá, e você pode
  **fixar localmente** ("disponível offline") o que quiser.
- **Upload direto para o bucket**, pela interface web (o navegador envia direto ao S3) e
  pelo Mac (CLI/biblioteca).
- **Docker só na AWS.** Containers processam os arquivos do bucket, rodam uma **instância
  cloud do Ozymandias** e servem de **workers de bursting**. No Mac, tudo continua nativo.
- **Sem backup.** Foi cortado do escopo.
- **Sem domínio pago.** Todo endereço público usa nomes gratuitos: `*.cloudfront.net`,
  `*.ts.net` (Tailscale) e o quick tunnel `*.trycloudflare.com` que já existe.

### Fatos verificados que moldam o desenho

- **Máquina:** MacBook Air M4, 10 núcleos, 16 GB, arm64 (igual ao Graviton: uma imagem
  serve para os dois). **13 GB livres no SSD** (é aqui que "S3 como armazenamento + fixar
  local" ganha sentido real). Roda na bateria, dorme, IP público dinâmico, sem domínio
  próprio. Link medido de ~378 Mbps de upload e ~456 Mbps de download.
- **Ferramentas:** Docker Desktop instalado (daemon parado). Sem AWS CLI, OpenTofu nem
  Tailscale. cloudflared e ffmpeg instalados. FileVault ativo. **Firewall desligado**, com
  MariaDB `*:3306` e PHP `*:8000` expostos na LAN.
- **Código:** cerca de 120 arquivos sem commit, remoto `github.com/Buenno0/nas`.
- **O Ozymandias hoje:**
  - SQLite em WAL (`internal/db/db.go`), migrações embutidas (a última é a 0009);
  - watcher fsnotify (`internal/scan/watcher.go`), pools de ffprobe, thumbnails e
    conversões;
  - media tokens HMAC (`internal/auth/midia.go`), pareamento de TV por device flow
    (`internal/api/device_auth.go`), supervisor do cloudflared
    (`internal/tunnel/tunnel.go`);
  - estado de jobs só em memória, **sem hash de conteúdo**.
- **Não compila em Linux**:
  - `syscall.Stat_t.Atimespec` em `internal/media/cache.go:121`;
  - `h264_videotoolbox` fixo em `internal/media/receitas.go:45` (o fallback de
    `encoder.go:51` não chega à receita).

  Isso **bloqueia** o Ozymandias em container na AWS. É o primeiro refactor.
- **Colisão de nomes:** "Local" e "Tunnel" já são os **modos de acesso de rede**
  (`nas serve --local|--tunnel`, README). O novo eixo precisa de outro nome. Proposta:
  **modo de nuvem** = `local | hibrido`, com CLI `nas modo local|hibrido`, config
  `"modo": "local"|"hibrido"`, e flag de emergência `--sem-nuvem`. A documentação passa a
  chamar os modos antigos de acesso "LAN" e "Tunnel". Os dois eixos são independentes.

---

## 1. Princípios de desacoplamento

1. **Local é o modo base.** O binário funciona sem nenhuma config de AWS, exatamente como
   hoje. O híbrido é um **módulo opcional**, não uma dependência.
2. **Ports & adapters.** O app depende só de interfaces em `internal/cloud`. O SDK da AWS
   é importado **apenas** em `internal/cloud/aws`. Com a build tag `nocloud`, o binário
   sai **sem o SDK AWS**, e isso prova o desacoplamento no compilador.
3. **Substituível.** O adapter de objeto fala a API S3 com endpoint configurável:
   AWS S3, Cloudflare R2, Backblaze B2 ou MinIO local para testes. Fila e eventos passam
   por uma interface, o que permite trocar SQS/SNS por NATS.
4. **Contratos, não banco compartilhado.** Mac e nuvem trocam **eventos versionados**
   (JSON com `schema_version`). Nenhum lado lê o banco do outro.
5. **Nada no caminho de reprodução de um item local toca a AWS.**
6. **Tudo outbound.** A AWS nunca inicia conexão com o Mac.
7. **Os dois lados vivem sozinhos.** O Mac em modo local ignora a nuvem. A nuvem continua
   funcionando com o Mac dormindo ou desligado.

---

## 2. Os dois modos e o kill switch

### 2.1 Máquina de estados do modo

```
                 nas modo hibrido / toggle na UI
   ┌────────┐  ───────────────────────────────►  ┌────────────┐    ┌──────────┐
   │ LOCAL  │                                    │ CONECTANDO │───►│ HIBRIDO  │
   │ (base) │  ◄─────────────────────────────────└────────────┘    └────┬─────┘
   └────────┘       kill switch (instantâneo, de qualquer estado)       │
        ▲───────────────────────────────────────────────────────────────┘
```

- **LOCAL → CONECTANDO → HIBRIDO:**
  1. obtém uma credencial temporária (IAM Roles Anywhere; a chave privada fica no
     Keychain);
  2. busca a chave de assinatura do CloudFront no SSM, que fica **só em memória**;
  3. **reconcilia**: puxa os eventos desde o último cursor (ou um snapshot, se o cursor
     for antigo), drena o outbox local e retoma uploads multipart pendentes;
  4. marca os itens da nuvem como disponíveis.

  Se falhar, volta para LOCAL com um erro visível. **Nunca fica meio-conectado.**
- **Kill switch (qualquer estado → LOCAL)**, com meta de **< 100 ms**:
  1. troca o estado atômico (`atomic.Pointer`). A partir desse instante o **guard de
     transporte** recusa qualquer requisição a destinos da AWS;
  2. cancela o `context` raiz de todo trabalho de nuvem: long-poll, uploads, downloads de
     pin e processos ffmpeg/ffprobe que leem URLs da nuvem (todos via
     `exec.CommandContext`);
  3. descarta da memória as credenciais STS e a chave do CloudFront;
  4. grava o evento `modo_alterado` no journal local e avisa a UI por SSE (o padrão SSE já
     existe em `api/scan.go`).
- **Persistência:** `modo` fica no `config.json` e sobrevive a reinícios. A flag
  `--sem-nuvem` força LOCAL independentemente da config (break-glass).
- **Gatilhos:** toggle na UI (admin, `PUT /api/modo`); CLI `nas modo local|hibrido`
  (grava a config e manda SIGHUP para o pid de `~/.nas/nas.pid`; `nas stop` já usa
  sinais em `internal/lock`).
- **Limite honesto:** uma URL assinada do CloudFront já entregue a uma TV continua válida
  até expirar. O kill switch corta o **Mac**, não clientes que já receberam links. Por
  isso as URLs de mídia da nuvem têm TTL curto (1 h, renovável enquanto estiver no
  híbrido).
- **Defesa em profundidade:** o guard é um `http.RoundTripper` que envolve o cliente HTTP
  do SDK e qualquer outro cliente de nuvem. Mesmo que algum trecho de código esqueça de
  checar o modo, a requisição não sai. Um contador `nuvem_bloqueadas_total` aparece em
  `/metricas`.
- **Modo local não descarta trabalho:** mudanças locais (arquivo novo, progresso,
  favoritos) continuam sendo registradas no **outbox** (é barato e local). Ao voltar para
  o híbrido, tudo é drenado. **Alternar é sem perda.**

### 2.2 Estado de cada arquivo (localização)

```
            nas push / biblioteca espelhada          fixar (download)
  LOCAL ────────────► ENVIANDO ────────► AMBOS ◄──────── BAIXANDO ◄──── NUVEM
    ▲                                    │  │                             ▲
    │  "remover da nuvem" (só pela UI    │  │ "liberar espaço" (apaga a   │
    └──────────────  local) ─────────────┘  └── cópia local após checar ──┘
                                                 o hash da cópia na nuvem)
  upload pela web ──────────────────────────────────────────────────────► NUVEM
```

| Estado | Modo local | Modo híbrido |
|---|---|---|
| LOCAL | toca (como hoje) | toca do Mac |
| AMBOS (fixado) | toca do Mac | toca do Mac (mais rápido que a CDN na LAN) |
| NUVEM | **"na nuvem, indisponível"** | toca via CloudFront, direto do cliente para a CDN, **sem passar pelo Mac** |
| ENVIANDO/BAIXANDO | pausa (retoma no híbrido) | progresso por SSE |

**Invariante crítica.** O scanner hoje **apaga do índice arquivos que sumiram do disco**
(`internal/scan/scanner.go`, na etapa de limpeza). Com a localização, ele passa a
**ignorar linhas NUVEM/BAIXANDO**. Sem isso, "liberar espaço" apagaria o item do
catálogo.

"Liberar espaço" e "remover da nuvem" são as **únicas** ações destrutivas. Elas só podem
ser disparadas pela UI/CLI local, **nunca** por um evento vindo da nuvem.

---

## 3. Arquitetura

```
┌──────────────────────────────────── AWS (us-east-1) ─────────────────────────────────────┐
│                                                                                           │
│  S3 "ozy-midia"  (Versioning + expiração de versões antigas 30 d, SSE-S3, TLS-only, CORS) │
│    uploads/<ulid>/original      (Intelligent-Tiering; abort multipart incompleto em 7 d)  │
│    derivados/<ulid>/{mp4,thumbs,vtt,probe.json}                                           │
│    catalogo/snapshots/<no>/<seq>.json.gz                                                  │
│      │ S3 event (ObjectCreated uploads/)                                                  │
│      ▼                                                                                    │
│  SQS "jobs" (+DLQ) ◄──── bursting enviado pelo Mac                                        │
│      │  Application Auto Scaling pela profundidade da fila: 0..N                          │
│      ▼                                                                                    │
│  ECS Fargate Spot arm64 ── service "worker"  (imagem ozy: `nas worker`)                   │
│      │ probe, thumbnails, MP4 compatível/HLS, legendas → derivados/                       │
│      ▼                                                                                    │
│  SNS "catalogo" ──fan-out──► SQS "no-mac"   (o Mac consome, só no híbrido)                │
│         ▲                └─► SQS "no-cloud" (a instância cloud consome)                   │
│         │ publicam: worker, instância cloud, Mac                                          │
│                                                                                           │
│  ECS Fargate ── service "cloud" (1 tarefa, imagem ozy: `nas serve --nuvem`)               │
│      SQLite próprio + Litestream → S3   ·   sidecar Tailscale (Funnel, *.ts.net)          │
│                                                                                           │
│  CloudFront (OAC → S3; URLs assinadas por key group) ── reprodução de itens NUVEM         │
│  SSM Parameter Store (chave CloudFront, tmdb_key) · ECR · IAM Roles Anywhere (CA própria) │
│  CloudWatch (alarmes: DLQ>0, worker falhando) · Budgets · CloudTrail                      │
└──────────────────────────────────────────▲────────────────────────────────────────────────┘
                                           │ HTTPS 443 outbound, SigV4 com STS de 1 h
                                           │ (só existe no modo HÍBRIDO)
┌──────────────────────────────────────────┴────── MacBook (nativo, sem Docker) ────────────┐
│ nas serve                                                                                 │
│  ├─ Ozymandias (web, API, stream, transcode VideoToolbox), SQLite dono dos itens locais   │
│  ├─ internal/cloud  (port) ──► adapter aws | adapter nop                                  │
│  │     guard de transporte · switch atômico · credenciais em memória                      │
│  ├─ outbox/journal em SQLite · cursor de sincronização · uploads/pins retomáveis          │
│  └─ scheduler consciente de energia (decide o bursting)                                   │
│ cloudflared (acesso humano, já existe)                                                    │
└───────────────────────────────────────────────────────────────────────────────────────────┘
```

### 3.1 Componentes e responsabilidades

| Componente | Responsabilidade | Substituível por |
|---|---|---|
| Ozymandias no Mac | dono dos itens LOCAL/AMBOS, dos usuários e da UI principal | n/a |
| `internal/cloud` (port) | a única fronteira com a nuvem: objeto, fila, eventos, assinatura, modo | adapter `nop` (local), MinIO, R2 |
| S3 `ozy-midia` | armazenamento principal dos itens NUVEM e dos derivados | R2 (egress zero), B2 |
| SQS `jobs` + Fargate `worker` | processamento assíncrono com escala a zero | Lambda (arquivos pequenos), Batch |
| SNS `catalogo` → SQS por nó | barramento de eventos do catálogo, desacoplado da disponibilidade de cada nó | EventBridge bus, NATS JetStream |
| Fargate `cloud` | Ozymandias sempre disponível: catálogo, reprodução de itens NUVEM e upload com o Mac dormindo | Lambda + Web Adapter (ver 3.3) |
| CloudFront | entrega de mídia da nuvem (1 TB/mês gratuito) | URL pré-assinada do S3 (MVP) |
| Roles Anywhere | identidade do Mac sem chave de longa duração | IoT Core credentials provider |
| OpenTofu | toda a infra em código (`infra/`) | Terraform, CDK |

### 3.2 Uma imagem, três papéis

O mesmo binário Go, compilado para linux/arm64 com ffmpeg, vira uma imagem com três
subcomandos:
- `nas worker`: consome `jobs` e **reaproveita** `scan/probe.go`, `media/thumb.go` e
  `media/prepare.go`/`receitas.go` com libx264, porque no Linux não há VideoToolbox;
- `nas serve --nuvem`: a instância cloud;
- o mesmo `nas worker` atende os jobs de bursting enviados pelo Mac.

A imagem é construída via `docker buildx`/CI. Construir não é executar: **nenhum
container roda no Mac**.

### 3.3 Hospedagem da instância cloud (decisão de custo)

Restrição: **nenhum domínio pago**. Só servem endereços HTTPS estáveis e gratuitos.

| Opção | Custo/mês | Prós | Contras |
|---|---|---|---|
| **A) Fargate Spot 0,25 vCPU/0,5 GB + sidecar Tailscale** ✅ | ~US$ 2 de compute + US$ 3,60 de IPv4 ≈ **US$ 6** | nome fixo `nuvem.<tailnet>.ts.net` com HTTPS grátis; privado por padrão; **Funnel** (grátis no plano pessoal) expõe publicamente quando necessário (TV sem o app); sem ALB; sem porta de entrada (a conexão é de saída) | dependência do Tailscale (a auth key efêmera e com tag fica no SSM); o Funnel tem limite de banda, o que importa pouco porque a mídia vai pelo CloudFront; Spot pode reiniciar a tarefa (o Litestream restaura em segundos) |
| **B) CloudFront + Lambda (Lambda Web Adapter, mesma imagem)** | ≈ **US$ 0** | `*.cloudfront.net` com HTTPS; uma distribuição com `/assets` e `/img` → S3, `/api` → Function URL (OAC) e mídia assinada; escala a zero; é a opção que mais ensina AWS | SQLite em `/tmp` restaurado do S3 na partida e regravado a cada escrita, com **concorrência reservada = 1** (um só escritor): as requisições em paralelo recebem 429, e o frontend precisa repetir com backoff em `web/src/lib/api.ts`; ~1 s de cold start; SSE limitado a 15 min (reconecta) |
| C) CloudFront + ALB privado (VPC origin) + Fargate | ~US$ 22+ | padrão AWS clássico, sem domínio | o ALB custa mais que todo o resto junto |
| D) Quick tunnel no Fargate + Lambda que redireciona para a URL atual | ≈ US$ 6 | nada a instalar | a URL muda a cada reinício; a Cloudflare trata quick tunnels como teste e indica que eles não suportam SSE (confirmar isso, porque afetaria também o modo tunnel atual do Mac) |

**Recomendação: A.** Ela mantém o desenho com Fargate e só troca o sidecar. O Mac pode
usar o mesmo Tailscale para ganhar um nome fixo (`mac.<tailnet>.ts.net`) no lugar da URL
do quick tunnel, que muda a cada execução, sem alterar o fluxo atual. **B** é o caminho
se o objetivo for custo zero e aprendizado serverless.

**Origens para o CORS do bucket** (upload direto do navegador): `https://*.ts.net`,
`https://*.trycloudflare.com`, `https://<distribuição>.cloudfront.net` e
`http://*.local:8787`. O CORS não é a fronteira de segurança: sem uma URL pré-assinada
válida, o upload é recusado de qualquer forma.

### 3.4 Federação do catálogo (Mac × instância cloud)

- **Propriedade por dado (sem multi-master):**
  - Mac: itens LOCAL/AMBOS, **usuários** e papéis;
  - instância cloud: itens que ela recebeu por upload enquanto o Mac estava fora (são
    NUVEM desde a origem; o Mac os incorpora pelo evento);
  - worker: derivados e metadados técnicos dos itens NUVEM.
- **Eventos (envelope):** `event_id` (ULID), `type`, `schema_version`, `origem`,
  `occurred_at`, `idempotency_key`, `content_hash`, `payload`. Entrega at-least-once,
  consumidores **idempotentes** (tabela `eventos_processados`).
- **Tipos:**
  - `item.adicionado`, `item.localizacao_alterada`, `item.metadados`, `item.removido`;
  - `usuario.upsert`: hash argon2id, emitido só pelo Mac;
  - `progresso.atualizado` (LWW por `(user, file)` com `updated_at`);
  - `favorito.set` (LWW element set);
  - `job.concluido`.
- **Snapshots:** cada nó publica periodicamente um snapshot compactado em
  `catalogo/snapshots/`. Um nó que ficou fora mais de 14 dias (a retenção máxima do SQS)
  ou uma instância nova reconstrói o estado com snapshot + eventos. É event sourcing
  simplificado.
- **Sessões e pareamentos de TV** continuam **por nó**. Não se replicam.
- **Conteúdo da instância cloud:** ela mostra a união dos catálogos, mas só toca itens
  NUVEM/AMBOS. Itens só LOCAL aparecem como *"no Mac"*, o espelho simétrico do
  "indisponível" do modo local.

---

## 4. Fluxos principais

### 4.1 Upload pela interface web (navegador → S3, sem passar pelo Mac)

```
browser ── POST /api/uploads {nome, tamanho, tipo} ──► nó atual (Mac no híbrido, ou cloud)
        ◄── {upload_id, key uploads/<ulid>/original, URLs pré-assinadas das partes (15 min)}
browser ── PUT das partes direto no S3 (CORS; multipart, 3–4 em paralelo, retomável) ──► S3
browser ── POST /api/uploads/{id}/concluir {ETags + checksums} ──► nó ──► S3 CompleteMultipartUpload
S3 event ──► SQS jobs ──► Fargate worker (escala 0→1) ──► derivados/ + evento item.adicionado
SNS catalogo ──► no-mac / no-cloud ──► o item aparece como NUVEM nos dois catálogos
```

- No **Mac em modo local**, o botão de upload fica desabilitado. A **instância cloud**
  aceita upload sempre, porque é desacoplada do modo do Mac.
- Limites: tamanho máximo, tipos aceitos e URLs de 15 min. As partes usam checksum
  (`x-amz-checksum-crc64nvme`) validado pelo S3.

### 4.2 Upload pelo Mac

- `nas push <arquivo|pasta> [--manter-local]`: calcula BLAKE3 localmente (que vira o
  `content_hash`), faz upload multipart retomável (o estado fica no SQLite) → AMBOS, ou
  NUVEM se usar `--liberar`.
- **Biblioteca espelhada** (a biblioteca ganha a política `nuvem: espelhar`): o watcher
  existente detecta o arquivo novo, e o outbox enfileira o push no híbrido.
- Itens enviados pelo Mac já têm probe e thumbnail feitos localmente. O worker só gera
  derivados que a CDN precisa (por exemplo, um MP4 compatível quando o original não toca
  direto).

### 4.3 Reprodução

- **LOCAL/AMBOS:** exatamente o fluxo atual (`api/stream.go`, `api/playback.go`).
- **NUVEM:** o `plan.go` ganha o modo `nuvem`. O nó devolve uma **URL assinada do
  CloudFront** (TTL de 1 h) para o original, se o cliente toca direto (a lógica de
  `?can=` já existe), ou para o derivado compatível. O cliente fala direto com a CDN.
- **MVP (antes do CloudFront):** URL pré-assinada do S3, que é mais simples, mas paga
  egress acima de 100 GB/mês.

### 4.4 Fixar / liberar espaço

- **Fixar:** download retomável (Range) → verifica o hash → AMBOS → o scanner indexa o
  arquivo local.
- **Liberar espaço:** confirma que a cópia na nuvem existe e bate o `content_hash`
  (HeadObject + checksum) → só então apaga a cópia local → NUVEM.

### 4.5 Bursting (Mac → Docker na AWS)

- O scheduler do Mac classifica jobs locais (conversões na fila de `trabalhos`,
  `media/prepare.go:99-114`). Se a fila passar do limite, **ou o Mac estiver na bateria ou
  quente**, e o arquivo já for AMBOS (o dado já está na nuvem), o job vai para `jobs` com o
  tipo `preparar`.
- O worker produz o derivado em `derivados/` e o Mac o baixa (ou o cliente toca via CDN).
- **Não vale bursting** quando o arquivo é só LOCAL e grande: o upload de GBs custa mais
  tempo que o VideoToolbox do M4 (~9× tempo real). Hashes e indexação também nunca vão
  para a nuvem.
- **Vale bursting:** itens que já estão no bucket, thumbnails e probe de uploads web,
  legendas, conversões com o Mac na bateria.

---

## 5. Falhas, offline e consistência

| Situação | Comportamento |
|---|---|
| Kill switch no meio de um upload | o multipart é abortado localmente; `upload_id` e as partes concluídas ficam no SQLite; ao voltar para o híbrido, retoma (partes órfãs somem pelo lifecycle de 7 dias) |
| Internet cai no híbrido | o Ozymandias segue na LAN; itens NUVEM viram "indisponível (sem conexão)" por detecção, sem trocar o modo; o outbox acumula; retry com backoff + jitter |
| AWS fora | igual à queda de internet, com circuit breaker por serviço |
| Mac dormindo | a instância cloud atende; uploads continuam; eventos para `no-mac` esperam até 14 dias; depois disso, snapshot |
| Worker falha | a mensagem volta à fila (visibility timeout maior que a duração do job, com heartbeat); após 3 tentativas vai para a DLQ, dispara um alarme e o item fica `erro_processamento` na UI |
| Tarefa Spot da instância cloud interrompida | o ECS a recoloca; o Litestream restaura o SQLite do S3; os eventos não consumidos seguem na fila |
| Evento duplicado ou fora de ordem | idempotência por `event_id`; LWW por `updated_at`; `schema_version` para evoluir |

---

## 6. Segurança

- **Identidade do Mac:** IAM Roles Anywhere com **CA própria** (nunca a ACM Private CA,
  que custa cerca de US$ 400/mês). Chave privada no Keychain via `aws_signing_helper`,
  STS de 1 h. **No modo local não existe credencial em memória.**
- **Role do Mac (mínima):**
  - S3: `Put/Get/Head/AbortMultipart` em `uploads/*`; `Get` em `derivados/*`;
    `Put/Get` em `catalogo/snapshots/mac/*`;
  - SQS: receive/delete em `no-mac`, send em `jobs`;
  - SNS: publish em `catalogo`;
  - SSM: `GetParameter` em dois parâmetros;
  - **sem** `DeleteObjectVersion`, `PutBucket*`, `iam:*`, `ecs:*`.
- **Task roles (containers na AWS):**
  - worker: `Get uploads/*`, `Put derivados/*`, receive em `jobs`, publish em `catalogo`;
  - instância cloud: presign de `uploads/*`, `Get derivados/*`, chave do CloudFront,
    SQS `no-cloud`, snapshots/`cloud`.
  - Containers não-root, filesystem somente leitura, subnet pública **sem porta de
    entrada** (sem inbound no SG; a entrada da instância cloud é pela conexão de saída do Tailscale).
    Sem NAT, sem VPC endpoints.
- **Blast radius, nuvem → Mac:** um container comprometido (ou credenciais AWS roubadas)
  consegue no máximo:
  - publicar eventos falsos;
  - apagar ou alterar objetos no bucket (o **Versioning** com expiração de 30 dias
    permite desfazer).

  **Não consegue** apagar arquivos locais. O protocolo não tem verbo destrutivo local; o
  Mac valida o schema de todo evento e o trata como **dado não confiável**; "liberar
  espaço" exige ação local com verificação de hash. O kill switch isola o Mac na hora
  durante um incidente.
- **Blast radius, Mac → nuvem:** com o Mac comprometido, o atacante tem a role mínima por
  no máximo 1 h por credencial (revogação via CRL da CA). Não cria recursos, não altera
  políticas, não apaga versões.
- **Sem backup:** itens **só NUVEM têm uma única cópia**. O S3 é durável, mas o Versioning
  é a única proteção contra exclusão acidental ou maliciosa. Não é um sistema de backup,
  é o mínimo para não perder por engano.
- **Conta:** MFA no root, root sem access keys, Identity Center para você, Budgets
  (US$ 1/5/10), CloudTrail (1 trilha de gestão gratuita), IAM Access Analyzer.
- **No Mac (antes de tudo):** ligar o firewall; revisar MariaDB e PHP em `*`.

---

## 7. Observabilidade

- **Local (o que já existe em `/metricas`, estendido):** modo atual e histórico de
  trocas, `nuvem_bloqueadas_total`, lag do cursor de sincronização, tamanho do outbox,
  uploads e pins em andamento, itens por localização, jobs enviados para bursting.
- **Logs:** `slog` estruturado em `~/.nas/nas.log` (o caminho existe em `config.go:87` e
  nunca foi usado).
- **AWS, só o que é útil:**
  - métricas básicas do ECS/SQS (gratuitas);
  - alarmes em **DLQ > 0**, **worker com falhas seguidas** e **Budgets**;
  - CloudWatch Logs dos containers com **retenção de 7 dias**;
  - **sem** Container Insights (cobrado) e sem métricas de alta cardinalidade.

---

## 8. Custos (us-east-1, aproximados; confirme na Pricing Calculator)

| Item | Custo/mês estimado |
|---|---|
| S3 Intelligent-Tiering (200 GB de mídia + ~50% de derivados) | ~US$ 3 a 7, caindo sozinho quando a mídia esfria |
| CloudFront | US$ 0 dentro de 1 TB e 10M requisições |
| Instância cloud: opção A (Fargate Spot 0,25/0,5 + IPv4) ou opção B (Lambda) | ~US$ 6 (A) ou ~US$ 0 (B) |
| Workers Fargate Spot (escala a zero) | centavos por arquivo (um filme de 2 h em 4 vCPU Spot ≈ US$ 0,05 a 0,10) |
| SQS, SNS, SSM Standard, Roles Anywhere | ~US$ 0 (dentro dos níveis gratuitos permanentes ou sem custo) |
| ECR (lifecycle: últimas 5 imagens) | ~US$ 0,10 |
| Domínio | US$ 0 (não usado; Tailscale e Cloudflare têm planos gratuitos) |
| **Total típico** | **~US$ 10 a 13/mês com A, ~US$ 4 a 7/mês com B**, cobertos no início pelos créditos de conta nova |

**Armadilhas:**
- NAT Gateway (~US$ 33/mês): nunca use subnet privada;
- ALB (~US$ 16+/mês);
- multipart incompleto sem lifecycle (cobra para sempre);
- egress do S3 sem CloudFront (US$ 0,09/GB acima de 100 GB);
- versões antigas sem expiração;
- CloudWatch Logs sem retenção;
- Container Insights / GuardDuty após o trial;
- Fargate on-demand esquecido;
- região `sa-east-1` (~1,5×).

**Desligar a nuvem** (escalar `cloud` e `worker` para 0 via OpenTofu) é outro botão,
diferente do kill switch. O kill switch só corta o Mac.

---

## 9. Fases de implementação

**V0: Pré-requisitos (sem AWS ainda)**
- Commit e push das alterações pendentes.
- **Portar para Linux:** build tags em `internal/media/cache.go` (atime Darwin × Linux) e
  fallback real de encoder em `internal/media/receitas.go:45` usando o
  `EncoderDeVideo()` de `encoder.go:51`. Confirmar com `GOOS=linux go build ./...`.
- Renomear os conceitos na documentação: acesso LAN/Tunnel × modo de nuvem
  local/híbrido.
- Conta AWS endurecida + Budgets; criar a tailnet (plano gratuito); ligar o firewall do Mac.

**MVP: Modos + kill switch + upload direto (sem Docker, sem instância cloud)**
- `internal/cloud`: `port.go` (interfaces), `nop` (local), `aws` (S3 + credenciais),
  `guard.go` (RoundTripper), `switch.go` (estado atômico + contexto raiz). Build tag
  `nocloud`.
- Config: `modo` + bloco `nuvem` (região, bucket, ARNs do Roles Anywhere) em
  `internal/config/config.go`. CLI `nas modo`, flag `--sem-nuvem`, recarga por SIGHUP.
- Migração `0010`: `media_files.localizacao`, `nuvem_key`, `content_hash`; tabela
  `uploads` (estado do multipart).
- API: `GET/PUT /api/modo`, `POST /api/uploads` + `/concluir`; `stream.go`/`playback.go`
  redirecionam itens NUVEM para uma URL pré-assinada do S3.
- Probe e thumbnail de itens NUVEM pelo **ffprobe/ffmpeg lendo a URL pré-assinada** (leitura
  por Range, sem baixar o arquivo inteiro), ainda sem worker.
- Scanner ignora itens NUVEM na limpeza.
- Frontend: toggle de modo em `Settings.tsx`, selo de modo em `Layout.tsx`, dropzone de
  upload, estados de disponibilidade em `Poster.tsx`/`Title.tsx`.
- OpenTofu (`infra/`): bucket (Versioning, CORS, lifecycle de multipart e de versões,
  Intelligent-Tiering), Roles Anywhere, role mínima, Budgets.
- `nas push`.
- **Critério de pronto:**
  - alternar local ⇄ híbrido durante um upload sem perder nada;
  - no modo local, `nuvem_bloqueadas_total` cresce e `lsof -i` não mostra conexões com
    `amazonaws.com`;
  - um item só NUVEM aparece como "na nuvem, indisponível".

**V2: Localização completa + sincronização sem perda**
- Fixar/liberar espaço com verificação de hash; biblioteca espelhada.
- Outbox + journal + reconciliação ao ativar o híbrido; uploads e pins retomáveis.
- CloudFront com OAC + URLs assinadas (chave no SSM) no lugar das pré-assinadas.

**V3: Docker na AWS: processamento**
- Dockerfile multi-stage arm64 (Go + ffmpeg), ECR, `nas worker`.
- S3 event → SQS `jobs` (+DLQ) → ECS Fargate Spot, com service escalando de 0 a N pela
  profundidade da fila.
- SNS `catalogo` → SQS `no-mac`; o Mac consome eventos `item.adicionado`/`job.concluido`.
- Derivados (MP4 compatível, thumbnails, VTT) para itens NUVEM.

**V4: Instância cloud do Ozymandias + observabilidade**
- `nas serve --nuvem`: SQLite + Litestream → S3, sidecar Tailscale com Funnel (opção A)
  ou CloudFront + Lambda Web Adapter (opção B), SQS `no-cloud`.
- Federação: `usuario.upsert`, progresso/favoritos LWW, snapshots, estado "no Mac".
- Upload com o Mac dormindo; alarmes de DLQ e falhas; logs com retenção.

**V5: Bursting + endurecimento + extras**
- Scheduler consciente de energia (AC/bateria/térmico/fila) → jobs `preparar` para o
  worker.
- Revisão de menor privilégio com Access Analyzer, CloudTrail, testes de caos do kill
  switch (acionar durante upload, pin, long-poll e reprodução).
- Opcionais: endereço único `*.cloudfront.net` que tenta o Mac e cai para a nuvem; se
  você escolheu A, comparar com a opção B (Lambda); comparar SNS/SQS com IoT Core (MQTT/Shadow) no canal com
  o Mac.

### Arquivos críticos (resumo)
- **Novos:** `internal/cloud/{port,nop,switch,guard}.go`, `internal/cloud/aws/`,
  `internal/db/migrations/0010_*.sql`, `infra/` (OpenTofu), `Dockerfile`.
- **Alterados:** `internal/config/config.go`, `internal/cli/{serve,cli}.go` (+ `modo.go`,
  `push.go`, `worker.go`), `internal/api/{server,stream,playback}.go` (+ `uploads.go`,
  `modo.go`), `internal/media/{cache,receitas,plan}.go`, `internal/scan/scanner.go`,
  `web/src/pages/{Settings,Title}.tsx`, `web/src/components/{Layout,Poster}.tsx`,
  `web/src/lib/api.ts`.

---

## 10. Verificação

- **Desacoplamento:** `go build -tags nocloud ./...` compila sem o SDK AWS;
  `GOOS=linux GOARCH=arm64 go build ./...` passa (pré-requisito para o container).
- **Testes sem AWS:** o adapter S3 roda contra **MinIO nativo** (`brew install minio`, sem
  Docker no Mac). Testes unitários do guard (no modo local, toda requisição a
  `*.amazonaws.com` falha) e do switch (o cancelamento interrompe long-poll, upload e
  ffmpeg em menos de 100 ms).
- **E2E manual:** subir no híbrido → enviar um arquivo pela web → ver NUVEM → acionar o
  kill switch no meio de outro upload → confirmar "indisponível" e zero conexões
  (`lsof -i | grep amazonaws`) → voltar para o híbrido → upload retomado e item tocando.
- **Custos:** acompanhar o Cost Explorer na primeira semana de cada fase; Budgets
  disparando corretamente (teste com limite baixo).
