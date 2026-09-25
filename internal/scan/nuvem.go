package scan

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"nas/internal/db"
)

// CaminhoNuvem é o "caminho" de um item que só existe no bucket. Precisa ser
// estável (é a chave do cache de miniaturas e a coluna UNIQUE do índice) e
// nunca colidir com um caminho real do disco.
func CaminhoNuvem(key string) string { return "nuvem:" + key }

// TipoPorExtensao diz se o Ozymandias indexa esse arquivo, e como.
func TipoPorExtensao(nome string) (db.MediaType, bool) {
	t, ok := extTypes[strings.ToLower(filepath.Ext(nome))]
	return t, ok
}

// IndexarNuvem coloca no catálogo um arquivo que só existe no bucket.
//
// leitura é uma URL assinada: o ffprobe lê por Range só o cabeçalho e o
// índice, sem baixar o arquivo. rel decide o título do mesmo jeito que o
// caminho relativo decide no disco ("Série/Temporada 1/S01E02.mkv").
func (s *Scanner) IndexarNuvem(ctx context.Context, lib db.Library, key, rel string, tamanho int64, leitura string) (int64, error) {
	ext := strings.ToLower(filepath.Ext(rel))
	mtype, ok := extTypes[ext]
	if !ok {
		return 0, fmt.Errorf("extensão %q não é mídia que o Ozymandias indexa", ext)
	}
	f := &foundFile{
		path:  CaminhoNuvem(key),
		rel:   filepath.Clean(rel),
		ext:   ext,
		size:  tamanho,
		mtime: time.Now().Unix(),
		mtype: mtype,
	}
	if needsProbe(f) && leitura != "" {
		if res, err := Probe(ctx, leitura); err == nil {
			f.probe = res
			f.probed = true
		}
	}

	id, err := s.db.UpsertFile(ctx, mediaFileFrom(lib, f), f.probed)
	if err != nil {
		return 0, err
	}
	if err := s.db.MarcaNaNuvem(ctx, id, db.LocalNuvem, key); err != nil {
		return 0, err
	}
	if f.probed {
		if err := s.gravaFaixas(ctx, id, f, nil); err != nil {
			return id, err
		}
	}
	if _, _, err := s.attachTitle(ctx, lib, f, id, map[string]int64{}); err != nil {
		return id, err
	}
	return id, nil
}

// AplicarProbe grava metadados que vieram de fora (o manifesto do worker) num
// arquivo já indexado: duração, codecs, faixas. É o ffprobe que o Mac não
// precisou rodar.
func (s *Scanner) AplicarProbe(ctx context.Context, fileID int64, res ProbeResult) error {
	f, err := s.db.FileByID(ctx, fileID)
	if err != nil {
		return err
	}
	lib, err := s.db.Library(ctx, f.LibraryID)
	if err != nil {
		return err
	}
	ff := &foundFile{path: f.Path, rel: f.RelPath, ext: f.Ext, size: f.Size, mtime: f.MTime,
		mtype: f.Type, probe: res, probed: true}
	if _, err := s.db.UpsertFile(ctx, mediaFileFrom(lib, ff), true); err != nil {
		return err
	}
	return s.gravaFaixas(ctx, fileID, ff, nil)
}
