BINARY   := nas
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X nas/internal/cli.Version=$(VERSION)
PREFIX   ?= $(HOME)/.local/bin

.PHONY: help build install uninstall run test fmt vet web web-dev clean

help:
	@echo "make build      compila bin/$(BINARY)"
	@echo "make install    instala o comando global em $(PREFIX)"
	@echo "make run        compila e sobe o servidor local"
	@echo "make test       roda os testes Go"
	@echo "make web        build do frontend (web/dist)"
	@echo "make web-dev    dev server do Vite com proxy para o Go"

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

web-dev:
	cd web && npm run dev

clean:
	rm -rf bin web/dist
