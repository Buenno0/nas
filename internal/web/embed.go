// Package web embute o build do frontend no binário, para o comando global
// funcionar de qualquer pasta sem depender de arquivos ao lado do executável.
package web

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var embedded embed.FS

// Available diz se há um frontend embutido de verdade (o `make build` sem
// `make web` produz só o marcador do diretório).
func Available() bool {
	_, err := embedded.Open("dist/index.html")
	return err == nil
}

// Rotas que o React resolve no cliente. Só elas recebem o index.html; o que
// não estiver aqui é 404 de verdade, com a tela de erro.
var spaRoutes = []string{"/", "/search", "/settings", "/ruinas"}
var spaPrefixes = []string{"/library/", "/title/", "/watch/"}

// IsSPARoute diz se o caminho pertence ao aplicativo.
func IsSPARoute(path string) bool {
	for _, r := range spaRoutes {
		if path == r {
			return true
		}
	}
	for _, p := range spaPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// ErrorPage devolve a tela de erro embutida (401, 404 ou 500).
func ErrorPage(code int) ([]byte, bool) {
	data, err := embedded.ReadFile(fmt.Sprintf("dist/telas-erro/%d.html", code))
	if err != nil {
		return nil, false
	}
	return data, true
}

// Handler serve os arquivos estáticos e, para qualquer rota desconhecida sem
// extensão, devolve o index.html — é o SPA que resolve a rota no cliente.
func Handler() http.Handler {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	files := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if clean == "." {
			clean = "index.html"
		}

		if _, err := fs.Stat(sub, clean); err != nil {
			if !IsSPARoute(r.URL.Path) {
				// Caminho que não é do app nem existe em disco: 404 de verdade,
				// com a tela de erro em vez de um redirecionamento silencioso.
				serveError(w, http.StatusNotFound)
				return
			}
			// Rota do React (/title/12): entrega o index e deixa o cliente decidir.
			serveIndex(w, r, sub)
			return
		}

		// Os assets do Vite têm hash no nome, então podem ser cacheados forte.
		// O index.html não: é ele que aponta para a versão nova.
		if strings.HasPrefix(clean, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

// ServeError escreve a tela de erro com o código HTTP correspondente. Se a
// página não estiver embutida, cai para um texto simples — nunca para uma
// resposta vazia.
func ServeError(w http.ResponseWriter, code int) { serveError(w, code) }

func serveError(w http.ResponseWriter, code int) {
	page, ok := ErrorPage(code)
	if !ok {
		http.Error(w, http.StatusText(code), code)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	w.Write(page)
}

func serveIndex(w http.ResponseWriter, r *http.Request, sub fs.FS) {
	data, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}
