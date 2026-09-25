# Imagem do worker da nuvem. Docker só existe aqui, na AWS: no Mac o
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

FROM debian:bookworm-slim
# ffmpeg do Debian: libx264 para a imagem, aac nativo para o som. Sem
# VideoToolbox aqui; EncoderDeVideo() cai no libx264 sozinho.
RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --home /trabalho --create-home ozymandias
COPY --from=build /out/nas /usr/local/bin/nas
USER ozymandias
ENV NAS_TRABALHO=/trabalho
WORKDIR /trabalho
ENTRYPOINT ["nas", "worker"]
