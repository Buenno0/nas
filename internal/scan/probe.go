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
	Duration float64 `json:"duration,omitempty"`
	Width    int     `json:"width,omitempty"`
	Height   int     `json:"height,omitempty"`
	VCodec   string  `json:"vcodec,omitempty"`
	ACodec   string  `json:"acodec,omitempty"`

	// Detalhes que decidem se o navegador toca o arquivo. "h264" sozinho não
	// decide: High 10 (yuv420p10le) é h264 e não toca em lugar nenhum.
	PixFmt   string `json:"pix_fmt,omitempty"`
	VProfile string `json:"vprofile,omitempty"`
	Channels int    `json:"channels,omitempty"`
	VBitrate int    `json:"vbitrate,omitempty"`

	// Tags de áudio, usadas para agrupar músicas em álbuns.
	Title  string `json:"title,omitempty"`
	Artist string `json:"artist,omitempty"`
	Album  string `json:"album,omitempty"`
	Track  int    `json:"track,omitempty"`

	// Todas as faixas de áudio e legenda do arquivo, na ordem em que aparecem.
	// ACodec/Channels acima continuam sendo os da faixa padrão — é o que decide
	// se o arquivo toca direto, e é o que o acervo antigo já gravava.
	Streams []Stream `json:"streams,omitempty"`
}

// Stream é uma faixa de áudio ou legenda dentro do arquivo.
type Stream struct {
	Index    int    `json:"index"`          // como o ffmpeg endereça em -map 0:N
	Kind     string `json:"kind,omitempty"` // "audio" | "subtitle"
	Codec    string `json:"codec,omitempty"`
	Lang     string `json:"lang,omitempty"`
	Title    string `json:"title,omitempty"`
	Channels int    `json:"channels,omitempty"`
	Default  bool   `json:"default,omitempty"`
	Forced   bool   `json:"forced,omitempty"`
}

type ffprobeOutput struct {
	Format struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
	Streams []struct {
		Index       int               `json:"index"`
		CodecType   string            `json:"codec_type"`
		CodecName   string            `json:"codec_name"`
		Width       int               `json:"width"`
		Height      int               `json:"height"`
		PixFmt      string            `json:"pix_fmt"`
		Profile     string            `json:"profile"`
		Channels    int               `json:"channels"`
		BitRate     string            `json:"bit_rate"`
		Tags        map[string]string `json:"tags"`
		Disposition struct {
			Default int `json:"default"`
			Forced  int `json:"forced"`
		} `json:"disposition"`
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
				res.PixFmt = st.PixFmt
				res.VProfile = st.Profile
				if n, err := strconv.Atoi(st.BitRate); err == nil {
					res.VBitrate = n
				}
			}
		case "audio":
			if res.ACodec == "" {
				res.ACodec = st.CodecName
				res.Channels = st.Channels
			}
			res.Streams = append(res.Streams, Stream{
				Index:    st.Index,
				Kind:     "audio",
				Codec:    st.CodecName,
				Lang:     firstTag(st.Tags, "language", "LANGUAGE"),
				Title:    firstTag(st.Tags, "title", "TITLE"),
				Channels: st.Channels,
				Default:  st.Disposition.Default == 1,
				Forced:   st.Disposition.Forced == 1,
			})
		case "subtitle":
			res.Streams = append(res.Streams, Stream{
				Index:   st.Index,
				Kind:    "subtitle",
				Codec:   st.CodecName,
				Lang:    firstTag(st.Tags, "language", "LANGUAGE"),
				Title:   firstTag(st.Tags, "title", "TITLE"),
				Default: st.Disposition.Default == 1,
				Forced:  st.Disposition.Forced == 1,
			})
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
