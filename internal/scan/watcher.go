package scan

import (
	"context"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"nas/internal/db"
)

// maxWatchedDirs limita quantos diretórios entram no watcher. No macOS o
// fsnotify usa kqueue, que gasta um descritor por pasta; um acervo enorme
// estouraria o limite do processo. Passando disso, o scan periódico assume.
const maxWatchedDirs = 2000

// Watcher observa as pastas das bibliotecas e avisa quando algo muda.
type Watcher struct {
	db       *db.DB
	debounce time.Duration
	onChange func()
}

func NewWatcher(database *db.DB, onChange func()) *Watcher {
	return &Watcher{db: database, debounce: 5 * time.Second, onChange: onChange}
}

// Run bloqueia até o contexto ser cancelado. Erros de watcher não são fatais:
// sem ele, o scan periódico continua cobrindo as mudanças.
func (w *Watcher) Run(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("watcher indisponível (%v): as mudanças serão vistas no scan periódico", err)
		return
	}
	defer watcher.Close()

	libs, err := w.db.Libraries(ctx)
	if err != nil {
		log.Printf("watcher: %v", err)
		return
	}

	watched := 0
	for _, lib := range libs {
		if !lib.Enabled {
			continue
		}
		watched += addRecursive(watcher, lib.Path, maxWatchedDirs-watched)
	}
	if watched == 0 {
		return
	}
	log.Printf("observando %d pastas por mudanças", watched)

	// Um timer parado que só dispara quando as mudanças param de chegar —
	// copiar 200 arquivos gera um scan, não duzentos.
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	pending := false

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Pasta nova entra na observação na hora.
			if event.Has(fsnotify.Create) && watched < maxWatchedDirs {
				if fi, err := os.Stat(event.Name); err == nil && fi.IsDir() {
					watched += addRecursive(watcher, event.Name, maxWatchedDirs-watched)
				}
			}
			if !relevant(event.Name) {
				continue
			}
			if pending && !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.debounce)
			pending = true

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("watcher: %v", err)

		case <-timer.C:
			pending = false
			w.onChange()
		}
	}
}

// relevant filtra o ruído do sistema de arquivos (arquivos temporários do
// Finder, downloads pela metade).
func relevant(path string) bool {
	name := filepath.Base(path)
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "~") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".part" || ext == ".crdownload" || ext == ".tmp" {
		return false
	}
	if _, ok := extTypes[ext]; ok {
		return true
	}
	// Sem extensão conhecida ainda pode ser uma pasta nova.
	return ext == ""
}

func addRecursive(watcher *fsnotify.Watcher, root string, budget int) int {
	if budget <= 0 {
		return 0
	}
	added := 0
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		name := d.Name()
		if path != root && (strings.HasPrefix(name, ".") || skipDirs[name]) {
			return fs.SkipDir
		}
		if added >= budget {
			return fs.SkipAll
		}
		if err := watcher.Add(path); err == nil {
			added++
		}
		return nil
	})
	return added
}
