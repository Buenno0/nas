# Ozymandias Estante: PS5 e TV no mesmo painel

> Plano. Fases 0 e 1 feitas (sonda `nas estante sondar`; página `/estante`).
> Escopo: status do PS5, volume/mudo da TV e mandar mídia para a TV por DLNA.
> Ligar o PS5 e o modo cinema ficaram para depois (Fase 3).
> **Sem streaming do PS5** (fica fora do binário).

## Contexto

A ideia veio do rascunho "Central da Sala": uma página que comanda a sala. O Ozymandias
já mora lá (player, Chromecast, assistir junto, pareamento de TV, DNS-SD, PWA, push), e o
Mac está na mesma rede do PS5 e da TV — então o painel é um módulo do Ozymandias, não um
app novo.

### Nome

"Sala" já é o nome das salas do assistir junto (`internal/juntos`, `/api/juntos`,
`PainelDaSala.tsx`, `useSala.ts`, `VoltarParaSala.tsx`). Usar "sala" de novo confundiria
código e interface. Proposta: **Estante** — onde o PS5 e a TV ficam.
Pacote `internal/estante`, rotas `/api/estante/*`, página `/estante`.

### Fatos que moldam o desenho

- **PS5 — status sem nada:** o protocolo de descoberta (DDP, UDP) responde `SRCH` com
  "ligado" (200) ou "em descanso" (620), nome, e o jogo aberto. Não exige pareamento.
- **PS5 — ligar exige credencial:** o `WAKEUP` do DDP leva um `user-credential` que só
  existe depois de registrar um dispositivo no Remote Play (PIN de 8 dígitos no PS5 +
  ID da conta PSN). O rascunho dizia que ligar não precisava de nada — não é verdade.
  No PS5: *Remote Play ativado* e, em Economia de energia, *continuar conectado à
  internet* + *permitir ligar o PS5 pela rede*.
- **Alternativa sem pareamento (a testar):** ligar a TV e trocar a entrada para o HDMI do
  PS5 pode acordá-lo por HDMI-CEC (*Ativar link de dispositivo HDMI* no PS5). Se
  funcionar, o modo cinema dispensa o pareamento.
- ~~**Bravia — REST oficial:**~~ *(não se aplica: a TV da sala não tem essa API, ver Resultados da Fase 0)* JSON-RPC por HTTP com o cabeçalho `X-Auth-PSK`.
  `/sony/system` (`getPowerStatus`, `setPowerStatus`), `/sony/audio`
  (`getVolumeInformation`, `setAudioVolume`, `setAudioMute`), `/sony/avContent`
  (`setPlayContent` com `extInput:hdmi?port=N`). Configuração única na TV: *Rede →
  Controle por IP → Autenticação: Normal e chave pré-compartilhada*, definir a chave, e
  ligar *Inicialização remota* (senão a TV não acorda pela rede).
- **"Mandar vídeo/foto pra TV" já existe:** é o Chromecast do player (`web/src/lib/cast.ts`).
  Fora do escopo daqui.
- **Segredos não podem ir para o banco:** o estado do Mac chega à instância cloud pelo
  snapshot do `nas.db`. Credencial do PS5 e chave da TV ficam no `config.json` (0600, só
  no Mac), como a chave do TMDB.
- **Só no Mac:** a instância cloud não enxerga a LAN. Tudo passa por `soNoMac`.
- **Go puro, sem cgo:** nada de chiaki-ng linkado (C, AGPL). O DDP e o cliente UPnP da TV
  são pequenos o bastante para escrever do zero; a build Linux/AWS não muda.

---

## Resultados da Fase 0 (18/10/2026)

O que a sonda viu na rede de casa (`192.168.15.0/24`), com o PS5 e a TV ligados:

| Aparelho | Endereço | O que responde |
|---|---|---|
| PS5 `PS5-858` | `192.168.15.129` | DDP na 9302, versão `00030010`, `200 Ok` ligado; `host-id` `B41F4D4BB4B5`, sistema `14100003`, Remote Play na 997 (registro na 9295, aberta) |
| TV Sony **KDL-32W655D** | `192.168.15.6` | **Só DLNA** (`http://192.168.15.6:2870/dmr.xml`): RenderingControl e AVTransport, sem senha |
| TP-Link TL-WR829N | `192.168.15.64` | um segundo roteador/repetidor na rede (anotado, não muda nada aqui) |

**A TV muda o plano.** A KDL-32W655D é uma Bravia de 2016 sem Android: porta 80 fechada,
**não tem a API REST** (`/sony/*`), nem IRCC, nem PSK. O que ela tem:

- **Volume e mudo por UPnP RenderingControl**, sem autenticação: `GetVolume`/`SetVolume`
  (0–100, passo 1) e `GetMute`/`SetMute`. Leitura e escrita confirmadas (regravei o mesmo
  volume, 2).
- **AVTransport**: dá para mandar uma URL de vídeo/foto/música para a TV tocar — o
  "mandar vídeo pra TV" do rascunho, que o Chromecast do player **não** alcança (esta TV
  não tem Cast).
- **Some da rede quando desliga.** "TV ligada" = o renderizador responde.
- **Não liga pela rede e não troca de entrada.** A única forma de ligá-la sem controle é o
  HDMI-CEC vindo do PS5 (*Ativar link de dispositivo HDMI* + *ligar a TV junto com o PS5*).

Consequências:

- O cliente da Bravia REST saiu; no lugar, `internal/estante/tv.go` (UPnP/DLNA).
- Não há PSK para guardar. A config da TV é só o endereço da descrição UPnP (ou nada,
  achando por SSDP).
- **O teste "TV liga o PS5 por CEC" não é possível** (a TV não liga nem troca de entrada
  pela rede). Então **ligar o PS5 exige o pareamento do Remote Play** — e é o PS5 que,
  acordando, liga a TV por CEC.
- A permissão de Rede Local do macOS não atrapalhou rodando pelo terminal. Rodando como
  serviço em segundo plano ainda não foi testado.

**Falta testar (precisa de você na sala).** Com ligar o PS5 adiado, só o item 2 importa
agora; os itens 1 e 3 ficam para quando a Fase 3 voltar à mesa.

1. PS5 em **descanso**. A primeira rodada da sonda foi a linha de base, com os dois
   desligados: **o PS5 não respondeu ao DDP** (nem por broadcast, nem direto no IP),
   embora a porta 9295 (registro do Remote Play) estivesse aberta — ou seja, a rede dele
   estava de pé. A TV também não apareceu, o que é o esperado (o DLNA some com ela
   desligada). Um PS5 em descanso que atende a rede deveria responder `620`; o silêncio
   aponta para *Economia de energia → Recursos disponíveis no modo descanso → Ativar
   ligar o PS5 pela rede* desligado. Ligar essa opção e repetir a sonda com o PS5 em
   descanso. Sem a resposta `620`, o status "descanso" vira "não responde" e o WAKEUP
   da Fase 2 também não vai funcionar.
2. TV desligada: confirmar que o renderizador some (e volta em quanto tempo ao ligar).
3. Ligar o PS5 pelo controle com a TV desligada: a TV liga e troca para o PS5 sozinha?
   (Valida o CEC do lado PS5 → TV, que o modo cinema vai usar.)

---

## Fase 0 — Spike nos aparelhos reais (plano original)

Um subcomando descartável, `nas estante sondar`, que imprime o que vê:

1. `SRCH` em broadcast no DDP — confirmar porta do PS5, códigos 200/620, campos
   (`host-id`, `host-name`, `running-app-name`).
2. Bravia: `getPowerStatus` e `getVolumeInformation` com a PSK; `setAudioVolume "+1"`;
   confirmar a versão de cada método (`getMethodTypes`), porque o `setAudioVolume` muda
   entre 1.0 e 1.2 conforme o modelo.
3. Com o PS5 em descanso: TV ligada + `setPlayContent` no HDMI do PS5 → ele acorda?
4. Rodar o binário como ele roda de verdade (LaunchAgent, se for o caso) e checar a
   permissão de **Rede Local** do macOS: sem ela, o acesso a IPs da LAN falha em silêncio.

**Saída:** decidir se a Fase 2 precisa do pareamento do Remote Play ou se o CEC basta.

## Fase 1 — Status + TV (sem pareamento nenhum) ✔

**Feito em 18/10/2026**, com estas diferenças do planejado:

- Sem bloco em Configurações: os dois aparelhos são achados sozinhos na rede. Para fixar,
  `nas config set estante.ps5 <IP>` e `nas config set estante.tv <URL da descrição>`
  (valem depois de reiniciar).
- O multicast do Wi-Fi perde pacotes: a TV responde ao SSDP em ~10 ms ou não responde.
  O M-SEARCH sai três vezes e a Central lembra a última TV achada, reabrindo-a por HTTP
  direto na sondagem seguinte.
- O link da Estante aparece na barra lateral e na barra de baixo do celular (6 itens) em
  todo servidor que não é a instância cloud. Ele usa o papel que já está em cache: abrir
  o app não dispara sondagem; só a página aberta (o SSE) faz o Mac falar com a rede.
- Um comando que falha devolve o último estado visto, sem sondar de novo.

### Backend: `internal/estante/`

| Arquivo | O que faz |
|---|---|
| `ps5.go` | `Sondar(ctx, host)` → `PS5{Estado, Nome, Jogo}`; `Descobrir(ctx)` por broadcast. Parse do DDP em função pura. |
| `tv.go` | **Feito na Fase 0.** `TV` (UPnP MediaRenderer): `ProcurarTVs` por SSDP, `AbrirTV(descrição)`, `Volume`/`DefinirVolume`, `Mudo`/`DefinirMudo`, `Transporte`. |
| `estante.go` | `Central`: lê a config atual por função (como `ConfigNuvem`), guarda o último estado, faz polling **só enquanto há alguém assistindo** os eventos (5 s), e expõe `Assinar()` para o SSE. |
| `*_test.go` | Respostas DDP em fixture (como `energia_test.go`); a TV contra um `httptest.Server` que imita a KDL-32W655D. |

### Config (`internal/config`)

```go
Estante struct {
    PS5 struct { Host string }
    TV  struct { Descricao string; VolumeCinema int } // Descricao vazia = SSDP
} `json:"estante"`
```

`nas config estante.ps5.host|estante.tv.descricao|…` em `internal/cli/config.go`. Não há
segredo nenhum para guardar enquanto ligar o PS5 estiver adiado.

### Rotas (`internal/api/estante.go`)

| Rota | Proteção |
|---|---|
| `GET /api/estante` | `protected` — estado; na nuvem responde `{disponivel:false}` |
| `GET /api/estante/eventos` | `protected` — SSE, no padrão de `/api/modo/events` |
| `POST /api/estante/tv/volume` `{valor}` ou `{passo}` | `soNoMac(protected)` |
| `POST /api/estante/tv/mudo` | `soNoMac(protected)` |
| `PUT /api/estante/config` | `soNoMac(adminOnly)` — segredos são só de escrita |

Volume e mudo para qualquer conta (é o caso "abaixa o volume enquanto conversa");
configuração, só admin — pelo túnel isso fica acessível pela internet.

### Frontend

- `web/src/pages/Estante.tsx`, pensada primeiro para o celular: cartão do PS5 (pílula
  "ligado" ou "desligado" — com o PS5 configurado para não atender a rede em descanso,
  descanso e desligado são indistinguíveis, e tudo bem; jogo aberto quando houver), cartão da TV (− / slider / +, mudo; "desligada" quando o renderizador some).
- `web/src/lib/useEstante.ts`: SSE com reconexão, no molde de `useSala.ts`.
- `lib/api.ts`: tipos e chamadas.
- Item na `Sidebar` (`components/Layout.tsx`) só quando `disponivel` e algo configurado.
- Bloco "Estante" em `Settings.tsx` (admin): PS5 e TV achados na rede, volume do modo
  cinema (se um dia existir), botão "Procurar na rede" (usa `Descobrir` e `ProcurarTVs`).

**Pronto quando:** do celular, na LAN e pelo túnel, dá para ver o PS5 e mexer no volume;
na instância cloud a página não aparece.

## Fase 2 — Mandar para a TV por DLNA

A TV da sala não tem Cast, então o "Transmitir" do player hoje não chega nela. O
AVTransport que ela expõe resolve isso sem app nenhum na TV.

- `tv.go`: `Tocar(ctx, url, metadados)` = `SetAVTransportURI` + `Play`; `Pausar`,
  `Parar`, `Buscar(posição)`, `Posicao` (`GetPositionInfo`).
- A URL é a do stream do Ozymandias com **token de mídia** (`/stream/{id}` já aceita),
  apontando para o IP do Mac na LAN — a TV não alcança o túnel.
- Antes de codar: `GetProtocolInfo` na TV para saber que contêineres/codecs ela aceita
  (TV de 2016: provavelmente MP4/H.264 + AAC, MP3, JPEG). O que não bater passa pelo
  preparo (transcodificação) que o servidor já tem.
- No player (`Watch.tsx`): o botão "Transmitir" lista também a TV DLNA, ao lado do
  Chromecast; o progresso volta por polling de `GetPositionInfo`.
- Fotos: mandar a foto aberta para a TV (JPEG direto).
- Rota: `POST /api/estante/tv/tocar {file_id}` em `soNoMac(protected)`.

## Fase 3 — Extras (opcionais, cada um independente)

- **Ligar o PS5 + modo cinema (adiado).** Você mantém o PS5 sem atender a rede em
  descanso, por consumo de energia, e ligar o PS5 é a função menos importante. Se um dia
  valer a pena: ativar *Ativar ligar o PS5 pela rede*, parear o Remote Play (importar a
  chave do chiaki-ng ou pareamento nativo em Go, sem copiar código AGPL), `Acordar` por
  WAKEUP, e um `ModoCinema` que acorda o PS5 → espera a TV aparecer (CEC do PS5) →
  ajusta o volume.
- Atalho do PWA para a página da Estante (`shortcuts` em `web/public/manifest.webmanifest`).
- Push "a TV ficou ligada sem ninguém" (`internal/push`).

---

## Riscos

- **Mac dormindo:** se o MacBook dorme, a Estante some junto com o resto do servidor.
  Já é uma limitação conhecida; não piora.
- **IPs mudando:** recomendar reserva de DHCP no roteador; como plano B, guardar o
  `host-id` do PS5 e redescobrir por broadcast se o IP salvo não responder.
- **DLNA é básico:** sem eventos assinados no v1, só polling de `GetVolume`; alguns
  renderizadores recusam argumentos fora da ordem do SCPD (o cliente manda na ordem).
- **Polling:** só com alguém olhando a página; sem ouvintes, nenhum pacote sai.

## Ordem sugerida de commits

1. ✔ `internal/estante` (DDP + TV por UPnP) com testes, e o subcomando `nas estante sondar`.
2. ✔ Config + rotas + SSE.
3. ✔ Página e navegação (sidebar e barra do celular).
4. Fase 2 (DLNA no player).
