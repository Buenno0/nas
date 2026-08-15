// Package scan percorre as bibliotecas, lê metadados e mantém o índice do
// banco em dia.
package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"nas/internal/db"
	"nas/internal/scan/nameparse"
)

// Extensões reconhecidas. O que não estiver aqui é ignorado.
var extTypes = map[string]db.MediaType{
	".mp4": db.TypeVideo, ".mkv": db.TypeVideo, ".mov": db.TypeVideo,
	".avi": db.TypeVideo, ".webm": db.TypeVideo, ".m4v": db.TypeVideo,
	".mpg": db.TypeVideo, ".mpeg": db.TypeVideo, ".wmv": db.TypeVideo,
	".ts": db.TypeVideo, ".m2ts": db.TypeVideo,

	".mp3": db.TypeAudio, ".flac": db.TypeAudio, ".m4a": db.TypeAudio,
	".aac": db.TypeAudio, ".ogg": db.TypeAudio, ".opus": db.TypeAudio,
	".wav": db.TypeAudio, ".wma": db.TypeAudio,

	".jpg": db.TypePhoto, ".jpeg": db.TypePhoto, ".png": db.TypePhoto,
	".webp": db.TypePhoto, ".gif": db.TypePhoto, ".heic": db.TypePhoto,
	".heif": db.TypePhoto, ".avif": db.TypePhoto, ".bmp": db.TypePhoto,
}

// Pastas que nunca contêm mídia do usuário.
var skipDirs = map[string]bool{
	"@eaDir": true, "node_modules": true, "#recycle": true,
	"$RECYCLE.BIN": true, "System Volume Information": true,
	".Trash": true, "lost+found": true,
}

// Stats resume o resultado de um scan.
type Stats struct {
	Files   int
	Added   int
	Updated int
	Removed int
	Titles  int
}

func (s Stats) String() string {
	return fmt.Sprintf("%d arquivos (%d novos, %d atualizados, %d removidos), %d títulos",
		s.Files, s.Added, s.Updated, s.Removed, s.Titles)
}

// Progress é emitido durante o scan para a CLI e para o SSE da interface.
type Progress struct {
	Library string `json:"library"`
	Current int    `json:"current"`
	Total   int    `json:"total"`
	File    string `json:"file"`
}

// ProgressFunc recebe atualizações; pode ser nil.
type ProgressFunc func(Progress)

type Scanner struct {
	db      *db.DB
	Workers int // paralelismo do ffprobe
	// Force reprocessa todos os arquivos, mesmo sem mudança de tamanho ou
	// data. Útil depois de uma atualização que passa a ler mais metadados.
	Force bool
}

func New(database *db.DB) *Scanner {
	return &Scanner{db: database, Workers: 4}
}

// ScanAll indexa todas as bibliotecas habilitadas.
func (s *Scanner) ScanAll(ctx context.Context, onProgress ProgressFunc) (Stats, error) {
	libs, err := s.db.Libraries(ctx)
	if err != nil {
		return Stats{}, err
	}

	var total Stats
	for _, lib := range libs {
		if !lib.Enabled {
			continue
		}
		st, err := s.ScanLibrary(ctx, lib, onProgress)
		if err != nil {
			// Uma biblioteca com problema (disco externo desconectado, por
			// exemplo) não pode impedir o scan das outras.
			log.Printf("scan da biblioteca %q falhou: %v", lib.Name, err)
			continue
		}
		total.Files += st.Files
		total.Added += st.Added
		total.Updated += st.Updated
		total.Removed += st.Removed
		total.Titles += st.Titles
	}
	return total, nil
}

type foundFile struct {
	path    string
	rel     string
	ext     string
	size    int64
	mtime   int64
	mtype   db.MediaType
	probe   ProbeResult
	probed  bool
	changed bool
}

// ScanLibrary indexa uma biblioteca: descobre arquivos, detecta o que mudou,
// lê metadados do que é novo e reagrupa tudo em títulos.
func (s *Scanner) ScanLibrary(ctx context.Context, lib db.Library, onProgress ProgressFunc) (Stats, error) {
	var stats Stats

	files, err := walk(lib.Path)
	if err != nil {
		return stats, err
	}
	stats.Files = len(files)

	existing, err := s.db.FileStamps(ctx, lib.ID)
	if err != nil {
		return stats, err
	}

	// 1. Quem mudou? Só esses passam pelo ffprobe.
	seen := make(map[string]bool, len(files))
	var toProbe []*foundFile
	for i := range files {
		f := &files[i]
		seen[f.path] = true
		st, indexed := existing[f.path]
		switch {
		case !indexed:
			f.changed = true
			stats.Added++
		// Foto não passa pelo ffprobe, então nunca fica com probed_at: exigir
		// isso dela marcaria a foto como "mudou" em todo scan.
		case s.Force || st.Size != f.size || st.MTime != f.mtime || (!st.Probed && needsProbe(f)):
			f.changed = true
			stats.Updated++
		}
		if f.changed && needsProbe(f) {
			toProbe = append(toProbe, f)
		}
	}

	s.probeAll(ctx, lib.Name, toProbe, onProgress)

	// 2. Grava o que mudou e associa aos títulos.
	titleIDs := make(map[string]int64)
	for i := range files {
		f := &files[i]
		st, indexed := existing[f.path]

		var fileID int64
		if f.changed || !indexed {
			id, err := s.db.UpsertFile(ctx, mediaFileFrom(lib, f), f.probed)
			if err != nil {
				return stats, err
			}
			fileID = id
		} else {
			fileID = st.ID
			if st.HasTitle {
				continue // nada mudou e já está agrupado
			}
		}

		titleID, isNew, err := s.attachTitle(ctx, lib, f, fileID, titleIDs)
		if err != nil {
			return stats, err
		}
		if isNew {
			stats.Titles++
		}
		_ = titleID
	}

	// 3. Some do índice o que sumiu do disco.
	var gone []int64
	for path, st := range existing {
		if !seen[path] {
			gone = append(gone, st.ID)
		}
	}
	if err := s.db.DeleteFilesByID(ctx, gone); err != nil {
		return stats, err
	}
	stats.Removed = len(gone)

	if _, err := s.db.PruneEmptyTitles(ctx, lib.ID); err != nil {
		return stats, err
	}
	if err := s.db.MarkLibraryScanned(ctx, lib.ID, time.Now()); err != nil {
		return stats, err
	}
	return stats, nil
}

// needsProbe diz se vale chamar o ffprobe para o arquivo.
func needsProbe(f *foundFile) bool {
	return f.mtype == db.TypeVideo || f.mtype == db.TypeAudio
}

// probeAll roda o ffprobe em paralelo — é I/O de processo, então vale a pena.
func (s *Scanner) probeAll(ctx context.Context, libName string, files []*foundFile, onProgress ProgressFunc) {
	if len(files) == 0 {
		return
	}
	if _, err := FFprobePath(); err != nil {
		log.Printf("ffprobe indisponível: metadados de duração/resolução ficarão vazios")
		return
	}

	workers := s.Workers
	if workers < 1 {
		workers = 1
	}

	var (
		mu    sync.Mutex
		done  int
		wg    sync.WaitGroup
		queue = make(chan *foundFile)
	)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range queue {
				res, err := Probe(ctx, f.path)
				if err == nil {
					f.probe = res
					f.probed = true
				} else if !errors.Is(err, context.Canceled) {
					log.Printf("ffprobe falhou em %s: %v", f.rel, err)
				}

				mu.Lock()
				done++
				current := done
				mu.Unlock()
				if onProgress != nil {
					onProgress(Progress{Library: libName, Current: current, Total: len(files), File: f.rel})
				}
			}
		}()
	}

	for _, f := range files {
		select {
		case <-ctx.Done():
			close(queue)
			wg.Wait()
			return
		case queue <- f:
		}
	}
	close(queue)
	wg.Wait()
}

func mediaFileFrom(lib db.Library, f *foundFile) db.MediaFile {
	return db.MediaFile{
		LibraryID: lib.ID,
		Path:      f.path,
		RelPath:   f.rel,
		Ext:       f.ext,
		Size:      f.size,
		MTime:     f.mtime,
		Type:      f.mtype,
		Duration:  f.probe.Duration,
		Width:     f.probe.Width,
		Height:    f.probe.Height,
		VCodec:    f.probe.VCodec,
		ACodec:    f.probe.ACodec,
		Track:     f.probe.Track,
		// Só música costuma trazer título nas tags; em vídeo esse campo vem
		// preenchido com lixo do encoder mais vezes do que ajuda.
		DisplayName: audioTitle(f),
	}
}

func audioTitle(f *foundFile) string {
	if f.mtype == db.TypeAudio {
		return f.probe.Title
	}
	return ""
}

// attachTitle decide a que título o arquivo pertence e cria o vínculo. O cache
// titleIDs evita reprocessar o mesmo título a cada arquivo da mesma pasta.
func (s *Scanner) attachTitle(ctx context.Context, lib db.Library, f *foundFile, fileID int64, cache map[string]int64) (int64, bool, error) {
	title, ep := titleFor(lib, f)
	key := string(title.Kind) + "|" + title.SortName + "|" + fmt.Sprint(title.Year)

	titleID, cached := cache[key]
	if !cached {
		id, err := s.db.UpsertTitle(ctx, title)
		if err != nil {
			return 0, false, err
		}
		titleID = id
		cache[key] = id
	}

	if err := s.db.SetFileTitle(ctx, fileID, titleID); err != nil {
		return 0, false, err
	}
	if ep != nil {
		ep.TitleID = titleID
		ep.MediaFileID = fileID
		if err := s.db.UpsertEpisode(ctx, *ep); err != nil {
			return 0, false, err
		}
	}
	return titleID, !cached, nil
}

// titleFor traduz um arquivo no título que o representa na interface.
func titleFor(lib db.Library, f *foundFile) (db.Title, *db.Episode) {
	t := db.Title{LibraryID: lib.ID, MetaState: "pending"}

	switch lib.Kind {
	case db.KindTV:
		parsed := nameparse.ParsePath(f.rel)
		t.Kind = db.TitleTV
		t.Name = fallbackName(parsed.Title, f)
		t.Year = parsed.Year
		t.SortName = nameparse.SortName(t.Name)
		if parsed.IsEpisode {
			return t, &db.Episode{
				Season:  parsed.Season,
				Episode: parsed.Episode,
				Name:    parsed.EpisodeName,
			}
		}
		return t, nil

	case db.KindMusic:
		album := f.probe.Album
		if album == "" {
			album = parentFolder(f.rel)
		}
		if album == "" {
			album = lib.Name
		}
		t.Kind = db.TitleAlbum
		t.Name = album
		t.Artist = f.probe.Artist
		t.SortName = nameparse.SortName(album)
		return t, nil

	case db.KindPhoto:
		folder := parentFolder(f.rel)
		if folder == "" {
			folder = lib.Name
		}
		t.Kind = db.TitlePhotos
		t.Name = folder
		t.SortName = nameparse.SortName(folder)
		return t, nil

	default: // filmes
		parsed := nameparse.ParsePath(f.rel)
		t.Kind = db.TitleMovie
		t.Name = fallbackName(parsed.Title, f)
		t.Year = parsed.Year
		t.SortName = nameparse.SortName(t.Name)
		return t, nil
	}
}

// fallbackName garante que nenhum título fique sem nome, mesmo quando o parser
// não consegue extrair nada do arquivo.
func fallbackName(parsed string, f *foundFile) string {
	if strings.TrimSpace(parsed) != "" {
		return parsed
	}
	base := strings.TrimSuffix(filepath.Base(f.rel), f.ext)
	if strings.TrimSpace(base) != "" {
		return base
	}
	return f.rel
}

func parentFolder(rel string) string {
	dir := filepath.Dir(rel)
	if dir == "." || dir == string(filepath.Separator) {
		return ""
	}
	return filepath.Base(dir)
}

// walk lista os arquivos de mídia sob root.
func walk(root string) ([]foundFile, error) {
	var files []foundFile

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Permissão negada em uma subpasta não invalida o resto do scan.
			log.Printf("ignorando %s: %v", path, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		name := d.Name()
		if d.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || skipDirs[name]) {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "._") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(name))
		mtype, ok := extTypes[ext]
		if !ok {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = name
		}

		files = append(files, foundFile{
			path:  path,
			rel:   rel,
			ext:   ext,
			size:  info.Size(),
			mtime: info.ModTime().Unix(),
			mtype: mtype,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("percorrendo %s: %w", root, err)
	}
	return files, nil
}
