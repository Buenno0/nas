package media

import "fmt"

// Receita de ffmpeg. Allowlist fechada, pelo mesmo motivo das larguras de
// miniatura: o cliente escolhe entre opções conhecidas, nunca monta argumentos.
type receita struct {
	nome string
	args func(origem, destino string, audio int) []string
}

// Duas decisões abaixo parecem otimização óbvia e foram medidas nesta máquina
// como PIORES — estão anotadas para ninguém "consertar" depois:
//
//   - `-hwaccel videotoolbox` na decodificação: 15,4x tempo real contra 70,8x
//     em software, por causa da cópia dos quadros de volta para a memória.
//   - `h264_videotoolbox` em vez de `libx264 -preset veryfast`: 12% mais lento
//     em tempo de parede (9,1x contra 10,2x), mas 0,97 núcleo contra 8,15.
//     Numa máquina que também serve HTTP, escaneia e é o computador de trabalho
//     do dono, 7,5x menos CPU vale os 12%.
var receitas = map[string]receita{
	"remux": {
		nome: "trocar o container, sem recodificar",
		args: func(origem, destino string, audio int) []string {
			return append(comuns(origem, audio),
				"-c", "copy",
				"-movflags", "+faststart",
				"-f", "mp4", destino)
		},
	},
	"audio": {
		nome: "recodificar apenas o áudio",
		args: func(origem, destino string, audio int) []string {
			return append(comuns(origem, audio),
				"-c:v", "copy",
				"-c:a", encoderAAC(), "-ac", "2", "-b:a", "160k",
				"-movflags", "+faststart",
				"-f", "mp4", destino)
		},
	},
	"video1080": {
		nome: "recodificar a imagem em H.264 até 1080p",
		args: func(origem, destino string, audio int) []string {
			return append(comuns(origem, audio),
				"-c:v", EncoderDeVideo(),
				"-b:v", "4M", "-maxrate", "6M", "-bufsize", "8M",
				"-profile:v", "high",
				// scale só reduz; vídeo menor que 1080p não é ampliado.
				"-vf", "scale='min(1920,iw)':-2",
				"-c:a", encoderAAC(), "-ac", "2", "-b:a", "160k",
				"-movflags", "+faststart",
				"-f", "mp4", destino)
		},
	},
}

// comuns são os argumentos que toda receita compartilha.
//
// -progress pipe:1 é o que permite barra de progresso real; -nostdin evita que
// o ffmpeg consuma a entrada do processo pai.
//
// -sn -dn continuam descartando legenda e dados: MP4 não aceita legenda de
// matroska e o mux falharia. Legenda não se perde por isso — ela sai por fora,
// extraída em WebVTT e servida como <track>, que é o único jeito de o
// espectador poder ligar e desligar.
//
// audio é o índice ABSOLUTO da faixa dentro do arquivo, validado contra o banco
// antes de chegar aqui. -1 significa "a faixa padrão", e aí vale a primeira.
func comuns(origem string, audio int) []string {
	mapaAudio := "0:a:0?"
	if audio >= 0 {
		mapaAudio = fmt.Sprintf("0:%d", audio)
	}
	return []string{
		"-nostdin", "-y", "-v", "error",
		"-progress", "pipe:1",
		"-i", origem,
		"-map", "0:v:0?", "-map", mapaAudio,
		"-sn", "-dn",
	}
}

// Nome da receita, para log e interface.
func NomeDaReceita(chave string) string {
	if r, ok := receitas[chave]; ok {
		return r.nome
	}
	return chave
}

func argumentosDaReceita(chave, origem, destino string, audio int) ([]string, error) {
	r, ok := receitas[chave]
	if !ok {
		return nil, fmt.Errorf("receita desconhecida: %q", chave)
	}
	return r.args(origem, destino, audio), nil
}
