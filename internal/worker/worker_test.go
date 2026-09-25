package worker

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nas/internal/cloud"
	"nas/internal/scan"
)

// pasta é um bucket em disco: só o que Processar usa.
type pasta struct {
	cloud.Armazenamento
	dir string
	mu  sync.Mutex
}

func (p *pasta) caminho(key string) string { return filepath.Join(p.dir, filepath.FromSlash(key)) }

func (p *pasta) Info(_ context.Context, key string) (cloud.Objeto, error) {
	b, err := os.ReadFile(p.caminho(key))
	if err != nil {
		return cloud.Objeto{}, cloud.ErrNaoExiste
	}
	s := md5.Sum(b)
	return cloud.Objeto{Key: key, Tamanho: int64(len(b)), ETag: `"` + hex.EncodeToString(s[:]) + `"`}, nil
}
func (p *pasta) Baixar(_ context.Context, key string, _ int64) (io.ReadCloser, error) {
	return os.Open(p.caminho(key))
}
func (p *pasta) Gravar(_ context.Context, key string, corpo []byte, _ string) error {
	os.MkdirAll(filepath.Dir(p.caminho(key)), 0o755)
	return os.WriteFile(p.caminho(key), corpo, 0o644)
}

// Um MKV com H.264 + AAC + legenda SRT: o navegador não abre MKV, então o
// worker precisa produzir o MP4 compatível (remux), o quadro e o VTT.
func TestProcessarVideoIncompativel(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("sem ffmpeg")
	}
	if _, err := scan.FFprobePath(); err != nil {
		t.Skip("sem ffprobe")
	}
	dir := t.TempDir()
	srt := filepath.Join(dir, "l.srt")
	os.WriteFile(srt, []byte("1\n00:00:00,500 --> 00:00:02,000\nOlá, Ozymandias\n"), 0o644)

	b := &pasta{dir: filepath.Join(dir, "bucket")}
	key := "bibliotecas/1/abc/Teste (2024).mkv"
	os.MkdirAll(filepath.Dir(b.caminho(key)), 0o755)
	cmd := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=24:duration=3",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3", "-i", srt,
		"-map", "0", "-map", "1", "-map", "2",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-c:s", "srt", b.caminho(key))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("não consegui gerar o vídeo de teste: %s", out)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	man, err := Processar(ctx, b, t.TempDir(), key)
	if err != nil {
		t.Fatal(err)
	}

	tipos := map[string]cloud.Derivado{}
	for _, d := range man.Derivados {
		tipos[d.Tipo] = d
		if _, err := os.Stat(b.caminho(d.Key)); err != nil {
			t.Errorf("derivado %s anunciado mas ausente do bucket", d.Key)
		}
		if !strings.HasPrefix(d.Key, cloud.PrefixoDosDerivados(key)) {
			t.Errorf("derivado fora do prefixo: %s", d.Key)
		}
	}
	for _, tipo := range []string{"frame", "compat", "legenda"} {
		if _, ok := tipos[tipo]; !ok {
			t.Errorf("faltou o derivado %s (veio %v)", tipo, man.Derivados)
		}
	}
	vtt, _ := os.ReadFile(b.caminho(tipos["legenda"].Key))
	if !bytes.Contains(vtt, []byte("Olá, Ozymandias")) {
		t.Errorf("VTT sem o texto da legenda: %q", vtt)
	}
	var probe scan.ProbeResult
	if err := json.Unmarshal(man.Probe, &probe); err != nil || probe.VCodec != "h264" || probe.Duration < 2 {
		t.Errorf("probe no manifesto: %+v (%v)", probe, err)
	}
	if _, err := os.Stat(b.caminho(cloud.PrefixoDosDerivados(key) + "manifesto.json")); err != nil {
		t.Error("manifesto.json não foi gravado")
	}
}

func TestJobsDaMensagem(t *testing.T) {
	s3 := `{"Records":[{"eventName":"ObjectCreated:CompleteMultipartUpload","s3":{"object":{"key":"bibliotecas/1/ab/Meu+Filme+%282020%29.mkv"}}},
	                  {"eventName":"ObjectRemoved:Delete","s3":{"object":{"key":"x"}}}]}`
	jobs, err := cloud.JobsDaMensagem([]byte(s3))
	if err != nil || len(jobs) != 1 || jobs[0].Key != "bibliotecas/1/ab/Meu Filme (2020).mkv" {
		t.Fatalf("evento S3: %+v %v", jobs, err)
	}
	if jobs, _ := cloud.JobsDaMensagem([]byte(`{"Event":"s3:TestEvent"}`)); len(jobs) != 0 {
		t.Fatal("TestEvent virou job")
	}
	jobs, _ = cloud.JobsDaMensagem([]byte(`{"schema_version":1,"tipo":"processar","key":"k","origem":"mac"}`))
	if len(jobs) != 1 || jobs[0].Origem != "mac" {
		t.Fatalf("job do Mac: %+v", jobs)
	}
}
