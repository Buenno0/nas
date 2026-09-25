BINARY   := nas
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X nas/internal/cli.Version=$(VERSION)
PREFIX   ?= $(HOME)/.local/bin

.PHONY: help build install uninstall run test fmt vet web comprimir web-dev clean contraste provas publicar-imagem

help:
	@echo "make build      compila bin/$(BINARY)"
	@echo "make install    instala o comando global em $(PREFIX)"
	@echo "make run        compila e sobe o servidor local"
	@echo "make test       roda os testes Go"
	@echo "make web        build do frontend (web/dist) + compressão"
	@echo "make comprimir  só recomprime os assets já construídos"
	@echo "make web-dev    dev server do Vite com proxy para o Go"
	@echo "make contraste  mede os pares de cor do design system (WCAG)"
	@echo "make provas     sobe a folha de provas do design system"

build:
	go build -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/nas

install: build
	@mkdir -p $(PREFIX)
	install -m 0755 bin/$(BINARY) $(PREFIX)/$(BINARY)
	@echo "instalado em $(PREFIX)/$(BINARY)"
	@case ":$$PATH:" in *":$(PREFIX):"*) ;; \
		*) echo "AVISO: $(PREFIX) não está no PATH. Adicione ao ~/.zshrc:"; \
		   echo '  export PATH="$$HOME/.local/bin:$$PATH"';; esac

uninstall:
	rm -f $(PREFIX)/$(BINARY)

run: build
	./bin/$(BINARY) serve --local

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

web:
	cd web && npm install && npm run build
	./scripts/comprimir-assets.sh

comprimir:
	./scripts/comprimir-assets.sh

web-dev:
	cd web && npm run dev

# Sai com erro se algum par de cor reprovar em contraste — ver DESIGN.md §8.
contraste:
	node scripts/contraste.mjs

# Precisa de 'make web' antes: a folha lê o CSS compilado, não o fonte.
provas:
	./scripts/folha-de-provas.sh

clean:
	rm -rf bin web/dist

# --- Imagem da nuvem ----------------------------------------------------------
# Construída na AWS (CodeBuild, ARM), nunca no Mac: aqui só sai o git archive
# do último commit. AWS_PROFILE precisa poder disparar o build.
AWS_PROFILE ?= ozymandias-admin

publicar-imagem:
	AWS_PROFILE=$(AWS_PROFILE) go run ./cmd/publicar-imagem \
		--bucket $$(tofu -chdir=infra output -raw bucket_build) \
		--projeto $$(tofu -chdir=infra output -raw projeto_build)
