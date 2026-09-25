# Preparar a AWS para o modo híbrido

Passo a passo do zero até `nas modo hibrido`, em camadas. A primeira camada
(bucket + identidade do Mac) já liga o híbrido; CDN, workers e instância cloud
entram depois, uma de cada vez.

Tempo: cerca de 1 h na primeira vez. Custo típico: US$ 4 a 13 por mês, conforme
as camadas (detalhe no fim).

---

## 0. Ferramentas no Mac

```bash
brew install awscli opentofu openssl@3
```

O `aws_signing_helper` (quem troca o certificado do Mac por credenciais) não
está no Homebrew: baixe o binário para macOS arm64 na página
**IAM Roles Anywhere → Obtaining temporary security credentials** da
documentação da AWS, coloque em `/usr/local/bin/aws_signing_helper` e
`chmod +x`. Confira com `aws_signing_helper version`.

---

## 1. Conta e acesso de administrador

1. Crie a conta em aws.amazon.com. Use um e-mail só dela.
2. **Root com MFA**: no console, *Security credentials* → *Assign MFA device*.
   Depois disso o root não é mais usado no dia a dia.
3. **Região**: `us-east-1` (a mais barata; `sa-east-1` custa ~1,5×).
4. **Administrador sem chave fixa**, pelo IAM Identity Center (gratuito):
   1. console → *IAM Identity Center* → *Enable*;
   2. *Users* → crie o seu usuário e ative o MFA dele;
   3. *Permission sets* → *Predefined* → `AdministratorAccess`;
   4. *AWS accounts* → selecione a conta → *Assign users* → o seu usuário com
      esse permission set;
   5. anote a *AWS access portal URL* (algo como `https://d-xxxx.awsapps.com/start`).
5. No terminal:

   ```bash
   aws configure sso --profile ozymandias-admin
   ```

   Responda com a URL do portal, `us-east-1`, a conta e o `AdministratorAccess`.
   Teste:

   ```bash
   aws sts get-caller-identity --profile ozymandias-admin
   ```

   A sessão dura algumas horas; quando vencer, `aws sso login --profile ozymandias-admin`.

Este perfil é só para você administrar (OpenTofu, build da imagem). O
Ozymandias nunca o usa: o Mac entra por outro caminho, o do passo 2.

---

## 2. Identidade do Mac (IAM Roles Anywhere)

O Mac não recebe access key. Ele tem um certificado assinado por uma CA sua; a
AWS confia na CA e troca o certificado por credenciais de 1 hora, que só valem
para o bucket do Ozymandias.

```bash
sh scripts/roles-anywhere.sh
```

O script cria em `~/.nas/pki` (pasta `0700`):

| arquivo | o que é | o que fazer |
|---|---|---|
| `ca.pem` | certificado da CA, público | vai para o `terraform.tfvars` |
| `ca.key` | chave da CA | **guarde offline** (gerenciador de senhas, pendrive) e apague do disco. Quem a tem emite certificados que a AWS aceita. Só volta para renovar. |
| `mac.pem` | certificado do Mac, 1 ano | fica aí |

A chave do certificado do Mac vai para o Keychain e é apagada do disco. Na
primeira credencial o macOS pergunta se o `aws_signing_helper` pode usá-la:
responda **Sempre permitir**.

Daqui a um ano: traga o `ca.key` de volta para `~/.nas/pki` e rode
`sh scripts/roles-anywhere.sh --renovar`.

---

## 3. Primeira camada: bucket, identidade e orçamento

```bash
cp infra/terraform.tfvars.exemplo infra/terraform.tfvars
```

Preencha:

| chave | valor |
|---|---|
| `bucket` | nome único no mundo, só minúsculas e hífens: `ozymandias-midia-<seunome>` |
| `regiao` | `us-east-1` |
| `email_alerta` | seu e-mail (Budgets e alarmes) |
| `origens_web` | de onde a interface abre: `http://localhost:8787`, `http://<seu-mac>.local:8787` e `https://*.ts.net` se for usar a instância cloud |
| `ca_pem` | o conteúdo inteiro de `~/.nas/pki/ca.pem` |
| `orcamento_usd` | limite mensal para o alerta, ex.: `15` |

Deixe `tailscale_authkey` vazio por enquanto: sem ele a instância cloud não é
criada.

```bash
cd infra
export AWS_PROFILE=ozymandias-admin
tofu init
tofu plan -out plano
```

Leia o plano antes de aplicar: ele lista cada recurso que vai nascer. Depois:

```bash
tofu apply plano
```

Isso cria **tudo** o que está em `infra/`, inclusive CloudFront, filas e o
cluster (que fica em zero réplicas: parado não custa). Os dois itens com custo
fixo só existem com a instância cloud ligada.

**Confirme os e-mails** que a AWS manda: a inscrição nos alertas só vale depois
do clique.

---

## 4. O Mac usando a nuvem

1. O perfil que o Ozymandias usa:

   ```bash
   tofu -chdir=infra output -raw perfil_aws >> ~/.aws/config
   aws sts get-caller-identity --profile ozymandias
   ```

   A resposta mostra a role `ozymandias-mac`: o certificado funcionou.

2. A configuração do `nas`: o output traz os comandos prontos.

   ```bash
   tofu -chdir=infra output -raw configurar_nas
   ```

   Rode cada `nas config set …`. O último comando da lista, `nas modo hibrido`,
   liga a nuvem (e avisa o servidor, se ele estiver no ar).

3. Teste o básico, nesta ordem:
   - Configurações → *Enviar para a nuvem*: envie um arquivo pequeno;
   - toque o arquivo (ele vem do bucket pela CDN);
   - no título, *Disponível offline*, depois *Liberar espaço*;
   - *Cortar a nuvem agora*: o item vira "na nuvem, indisponível";
   - `lsof -i | grep -i amazonaws` não mostra nada no modo local.

---

## 5. Workers (processamento na nuvem)

A imagem é construída na própria AWS (CodeBuild); o Mac não roda Docker.

```bash
make publicar-imagem
```

Leva uns 5 minutos. A partir daí, cada arquivo novo no bucket gera miniatura,
legendas e, quando precisa, o MP4 compatível. O serviço fica em zero réplicas e
sobe sozinho com a fila.

---

## 6. Instância cloud (opcional)

Para o Ozymandias continuar no ar com o Mac dormindo, em
`https://ozymandias-nuvem.<sua-tailnet>.ts.net` (HTTPS de graça, sem domínio).
Por padrão ela fica **na internet** pelo Tailscale Funnel: abre em qualquer
navegador, sem app. A senha e o limite de tentativas de login continuam
valendo.

1. Conta gratuita no Tailscale (tailscale.com). O app só é preciso para a
   variante privada (`funnel = false`).
2. Em *Access controls*, no JSON da política, declare a tag e libere o Funnel
   para ela:

   ```json
   "tagOwners": { "tag:ozymandias": ["autogroup:admin"] },
   "nodeAttrs": [ { "target": ["tag:ozymandias"], "attr": ["funnel"] } ],
   ```

3. *Settings* → *Keys* → *Generate auth key*: **reusable**, **ephemeral** e com a
   tag `tag:ozymandias`.
4. Em `infra/terraform.tfvars`: `tailscale_authkey = "tskey-auth-…"`. Para
   deixar só na sua tailnet (cada aparelho com o app): `funnel = false`.
5. `tofu apply` de novo. Em alguns minutos ela responde no endereço acima.

## Custos e como desligar

| camada | custo/mês típico |
|---|---|
| bucket (200 GB, Intelligent-Tiering) | US$ 3 a 7 |
| CloudFront | US$ 0 até 1 TB |
| SQS, SNS, SSM, Roles Anywhere, Access Analyzer, CloudTrail (gestão) | ~US$ 0 |
| workers | centavos por arquivo processado |
| builds da imagem | centavos por build |
| instância cloud (Fargate Spot 0,5 vCPU + IPv4 público) | ~US$ 7 a 9 |
| Container Insights (alarme da instância cloud) | ~US$ 1 a 3 depois do período gratuito |

- O **kill switch** (`nas modo local`) só corta o Mac; a AWS continua existindo.
- Para parar a instância cloud: `tailscale_authkey = ""` e `tofu apply`.
- Para apagar tudo: `tofu destroy`. O bucket com mídia não é apagado se tiver
  objetos; esvazie antes, sabendo que isso apaga os arquivos que só estão lá.
- Na primeira semana, olhe o *Cost Explorer* a cada dois dias.
