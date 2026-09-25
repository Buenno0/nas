# Imagem da nuvem: o mesmo binário faz o worker (nas worker) e a instância
# cloud (ozymandias-nuvem, que é nas serve --nuvem sob o Litestream). Docker só existe aqui, na AWS: no Mac o
# Ozymandias continua nativo. arm64 serve ao Graviton do Fargate e ao M4.
#
#   make imagem            constrói linux/arm64
#   make publicar-imagem   envia ao ECR (precisa de ECR_URL)

FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build
ARG TARGETOS=linux TARGETARCH=arm64
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG VERSAO=dev
# CGO desligado: modernc.org/sqlite é Go puro, e a imagem final não precisa
# de libc de build. O frontend já está em internal/web/dist (embutido).
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X nas/internal/cli.Version=${VERSAO}" -o /out/nas ./cmd/nas

FROM --platform=$BUILDPLATFORM debian:bookworm-slim AS litestream
ARG TARGETARCH=arm64
ARG LITESTREAM=0.3.13
ADD https://github.com/benbjohnson/litestream/releases/download/v${LITESTREAM}/litestream-v${LITESTREAM}-linux-${TARGETARCH}.tar.gz /tmp/litestream.tar.gz
RUN tar -xzf /tmp/litestream.tar.gz -C /usr/local/bin litestream

FROM debian:bookworm-slim
# ffmpeg do Debian: libx264 para a imagem, aac nativo para o som. Sem
# VideoToolbox aqui; EncoderDeVideo() cai no libx264 sozinho.
RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --home /trabalho --create-home ozymandias
COPY --from=build /out/nas /usr/local/bin/nas
COPY --from=litestream /usr/local/bin/litestream /usr/local/bin/litestream
COPY deploy/litestream.yml /etc/litestream.yml
COPY deploy/ozymandias-nuvem /usr/local/bin/ozymandias-nuvem
USER ozymandias
ENV NAS_TRABALHO=/trabalho HOME=/trabalho
WORKDIR /trabalho
# Worker por padrão; a task da instância cloud troca o comando.
CMD ["nas", "worker"]
