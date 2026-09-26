// Package web embute o build do frontend no binário, para o comando global
// funcionar de qualquer pasta sem depender de arquivos ao lado do executável.
package web

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"sync"
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
var spaRoutes = []string{"/", "/search", "/settings", "/metricas", "/tecnico", "/enviar", "/armazenamento", "/artistas", "/colecoes", "/ruinas", "/conectar"}
var spaPrefixes = []string{"/library/", "/title/", "/watch/", "/artista/", "/colecao/"}

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

// ErrorPage devolve a tela de erro embutida (401, 404, 500, 502 ou 503).
func ErrorPage(code int) ([]byte, bool) { return pagina(fmt.Sprint(code)) }

// pagina lê uma tela pelo nome: "404", "503-no-mac".
func pagina(nome string) ([]byte, bool) {
	data, err := embedded.ReadFile("dist/telas-erro/" + nome + ".html")
	if err != nil {
		return nil, false
	}
	return data, true
}

// errorPageGzip devolve a variante comprimida da tela de erro, se existir.
// São 15 KB de HTML animado que caem para 4,5 KB — vale mesmo em página que
// aparece pouco, porque quem cai no 401 pelo tunnel costuma estar no celular.
func errorPageGzip(nome string) ([]byte, bool) {
	data, err := embedded.ReadFile("dist/telas-erro/" + nome + ".html.gz")
	if err != nil {
		return nil, false
	}
	return data, true
}

// aceitaGzip diz se o cliente declarou entender gzip.
//
// Checagem literal por substring basta aqui: nenhum navegador manda
// "gzip;q=0" (a forma de recusar explicitamente), e um falso positivo só
// aconteceria com um cliente que anuncia o que não sabe ler.
func aceitaGzip(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
}

// tipoPor devolve o Content-Type a partir da extensão do arquivo ORIGINAL.
// Necessário porque o que vai pelo fio é um .gz: deixar o net/http farejar o
// conteúdo renderia "application/gzip" e o navegador baixaria o arquivo em vez
// de executá-lo.
func tipoPor(nome string) string {
	if t := mime.TypeByExtension(filepath.Ext(nome)); t != "" {
		return t
	}
	return "application/octet-stream"
}

// serveComprimido entrega a variante .gz de um arquivo. Devolve false quando
// ela não existe, para o chamador cair na versão normal.
func serveComprimido(w http.ResponseWriter, sub fs.FS, nome string) bool {
	f, err := sub.Open(nome + ".gz")
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return false
	}

	w.Header().Set("Content-Type", tipoPor(nome))
	w.Header().Set("Content-Encoding", "gzip")
	// Sem Vary, um cache intermediário poderia entregar bytes comprimidos a um
	// cliente que pediu texto puro.
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Content-Length", fmt.Sprint(info.Size()))
	// Range sobre corpo já codificado é legal mas ninguém usa para asset, e
	// anunciar suporte convidaria a um pedido que não vale a pena implementar.
	w.Header().Set("Accept-Ranges", "none")

	io.Copy(w, f)
	return true
}

// Handler serve os arquivos estáticos e, para qualquer rota desconhecida sem
// extensão, devolve o index.html — é o SPA que resolve a rota no cliente.
//
// Assets são pré-comprimidos no build (scripts/comprimir-assets.sh) e servidos
// como .gz quando o cliente aceita: o JS cai de 343 KB para 103 KB e o CSS de
// 56 KB para 10 KB. Comprimir em tempo real gastaria CPU repetindo o mesmo
// trabalho, já que o nome do arquivo tem hash e o conteúdo nunca muda.
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

		// Pedido direto por .gz não é serviço nosso: a negociação é por
		// cabeçalho, e expor os dois nomes duplicaria a URL de cada asset.
		if strings.HasSuffix(clean, ".gz") {
			serveError(w, r, http.StatusNotFound)
			return
		}

		if _, err := fs.Stat(sub, clean); err != nil {
			if !IsSPARoute(r.URL.Path) {
				// Caminho que não é do app nem existe em disco: 404 de verdade,
				// com a tela de erro em vez de um redirecionamento silencioso.
				serveError(w, r, http.StatusNotFound)
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

		if aceitaGzip(r) && serveComprimido(w, sub, clean) {
			return
		}
		files.ServeHTTP(w, r)
	})
}

// ServeError escreve a tela de erro com o código HTTP correspondente. Se a
// página não estiver embutida, cai para um texto simples — nunca para uma
// resposta vazia.
func ServeError(w http.ResponseWriter, r *http.Request, code int) { serveError(w, r, code) }

// ServeTela é o ServeError para telas com nome próprio: duas telas podem
// compartilhar o código (503 do modo local e 503 do "no Mac").
func ServeTela(w http.ResponseWriter, r *http.Request, nome string, code int) {
	servePagina(w, r, nome, code)
}

// QuerTela diz se quem pediu é um navegador abrindo a URL, e não o SPA (que
// pede com Accept: */* e espera JSON).
func QuerTela(r *http.Request) bool { return strings.Contains(r.Header.Get("Accept"), "text/html") }

func serveError(w http.ResponseWriter, r *http.Request, code int) {
	servePagina(w, r, fmt.Sprint(code), code)
}

func servePagina(w http.ResponseWriter, r *http.Request, nome string, code int) {
	page, ok := pagina(nome)
	if !ok {
		http.Error(w, http.StatusText(code), code)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Accept-Encoding")

	// O status vem antes do corpo, comprimido ou não: a tela de 404 tem de
	// chegar COM 404, que é o ponto das ruínas.
	if r != nil && aceitaGzip(r) {
		if gz, ok := errorPageGzip(nome); ok {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Length", fmt.Sprint(len(gz)))
			w.WriteHeader(code)
			w.Write(gz)
			return
		}
	}
	w.Header().Set("Content-Length", fmt.Sprint(len(page)))
	w.WriteHeader(code)
	w.Write(page)
}

// O index é o arquivo mais pedido do servidor (toda navegação direta a uma rota
// do SPA passa por aqui), e nunca muda em execução: lê uma vez e guarda.
var (
	indexUmaVez sync.Once
	indexPlano  []byte
	indexGzip   []byte
)

func carregaIndex(sub fs.FS) {
	indexUmaVez.Do(func() {
		indexPlano, _ = fs.ReadFile(sub, "index.html")
		indexGzip, _ = fs.ReadFile(sub, "index.html.gz")
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, sub fs.FS) {
	carregaIndex(sub)
	if indexPlano == nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Vary", "Accept-Encoding")

	if aceitaGzip(r) && indexGzip != nil {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", fmt.Sprint(len(indexGzip)))
		w.Write(indexGzip)
		return
	}
	w.Header().Set("Content-Length", fmt.Sprint(len(indexPlano)))
	w.Write(indexPlano)
}
