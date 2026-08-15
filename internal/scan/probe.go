package scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// ErrNoFFprobe indica que o binário não está instalado. O scan continua sem
// duração/resolução em vez de falhar.
var ErrNoFFprobe = errors.New("ffprobe não encontrado no PATH")

var (
	ffprobeOnce sync.Once
	ffprobePath string
)

// FFprobePath resolve o binário uma única vez por processo.
func FFprobePath() (string, error) {
	ffprobeOnce.Do(func() {
		if p, err := exec.LookPath("ffprobe"); err == nil {
			ffprobePath = p
		}
	})
	if ffprobePath == "" {
		return "", ErrNoFFprobe
	}
	return ffprobePath, nil
}

// ProbeResult é o subconjunto dos metadados do ffprobe que o NAS usa.
type ProbeResult struct {
	Duration float64
	Width    int
	Height   int
	VCodec   string
	ACodec   string

	// Tags de áudio, usadas para agrupar músicas em álbuns.
	Title  string
	Artist string
	Album  string
	Track  int
}

type ffprobeOutput struct {
	Format struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
	Streams []struct {
		CodecType string            `json:"codec_type"`
		CodecName string            `json:"codec_name"`
		Width     int               `json:"width"`
		Height    int               `json:"height"`
		Tags      map[string]string `json:"tags"`
	} `json:"streams"`
}

// Probe lê metadados de um arquivo de mídia.
func Probe(ctx context.Context, path string) (ProbeResult, error) {
	bin, err := FFprobePath()
	if err != nil {
		return ProbeResult{}, err
	}

	cmd := exec.CommandContext(ctx, bin,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return ProbeResult{}, fmt.Errorf("ffprobe em %s: %w", path, err)
	}

	var parsed ffprobeOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return ProbeResult{}, fmt.Errorf("saída do ffprobe inválida: %w", err)
	}

	res := ProbeResult{}
	if d, err := strconv.ParseFloat(parsed.Format.Duration, 64); err == nil {
		res.Duration = d
	}
	for _, st := range parsed.Streams {
		switch st.CodecType {
		case "video":
			// Capa embutida em MP3 aparece como stream de vídeo: ignoramos
			// codecs de imagem para não confundir com um vídeo de verdade.
			if st.CodecName == "mjpeg" || st.CodecName == "png" {
				continue
			}
			if res.VCodec == "" {
				res.VCodec = st.CodecName
				res.Width, res.Height = st.Width, st.Height
			}
		case "audio":
			if res.ACodec == "" {
				res.ACodec = st.CodecName
			}
		}
	}

	tags := parsed.Format.Tags
	res.Title = firstTag(tags, "title", "TITLE")
	res.Artist = firstTag(tags, "artist", "ARTIST", "album_artist", "ALBUM_ARTIST")
	res.Album = firstTag(tags, "album", "ALBUM")
	if t := firstTag(tags, "track", "TRACK"); t != "" {
		// Vem como "3" ou "3/12".
		if n, err := strconv.Atoi(strings.SplitN(t, "/", 2)[0]); err == nil {
			res.Track = n
		}
	}
	return res, nil
}

func firstTag(tags map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := tags[k]; ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
