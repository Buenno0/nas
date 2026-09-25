package scan

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"time"
)

// Data de captura das fotos.
//
// O ffprobe não devolve EXIF de imagem — testado em JPEG desta máquina:
// format.tags vem vazio. E o projeto não traz dependência para isso. Então a
// leitura é feita aqui, direto nos bytes.
//
// A abordagem é procurar a assinatura "Exif\0\0" no começo do arquivo e ler o
// TIFF que vem logo depois. Parece grosseiro comparado a implementar JPEG, HEIC
// e PNG separadamente, mas é o mesmo bloco EXIF em todos eles — um caminho só
// cobre a foto do iPhone (HEIC), a da câmera (JPEG) e a exportada (PNG/WebP).
//
// O risco da busca por assinatura é falso positivo dentro dos dados da imagem.
// Ele é fechado por três validações em sequência: a ordem de bytes tem de ser
// II ou MM, o número mágico do TIFF tem de ser 42, e a data tem de ser uma data
// plausível. Passar pelas três por acaso é improvável a ponto de não valer mais
// código.

// bytesParaProcurar limita a busca ao cabeçalho. EXIF, por especificação, mora
// no começo do arquivo; varrer uma foto de 40 MB inteira seria pagar I/O caro
// por um caso que não existe.
const bytesParaProcurar = 256 << 10

var assinaturaExif = []byte("Exif\x00\x00")

var errSemExif = errors.New("sem data de captura")

// DataDeCaptura devolve quando a foto foi tirada, em unix. Zero quando o
// arquivo não diz — e aí quem chama decide o que fazer, em vez de receber um
// palpite disfarçado de dado.
func DataDeCaptura(caminho string) int64 {
	f, err := os.Open(caminho)
	if err != nil {
		return 0
	}
	defer f.Close()

	cabecalho := make([]byte, bytesParaProcurar)
	n, err := io.ReadFull(f, cabecalho)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return 0
	}
	cabecalho = cabecalho[:n]

	// Pode haver mais de uma ocorrência (miniatura embutida, por exemplo):
	// a primeira que produzir uma data válida vale.
	inicio := 0
	for {
		pos := bytes.Index(cabecalho[inicio:], assinaturaExif)
		if pos < 0 {
			return 0
		}
		tiff := cabecalho[inicio+pos+len(assinaturaExif):]
		if quando, err := dataDoTIFF(tiff); err == nil {
			return quando
		}
		inicio += pos + len(assinaturaExif)
	}
}

// Tags que interessam. DateTimeOriginal é o momento do clique; DateTimeDigitized
// é quando virou arquivo; DateTime é a última modificação da imagem. A ordem de
// preferência é essa mesma.
const (
	tagExifIFD          = 0x8769
	tagDateTime         = 0x0132
	tagDateTimeOriginal = 0x9003
	tagDateTimeDigital  = 0x9004
)

func dataDoTIFF(b []byte) (int64, error) {
	if len(b) < 8 {
		return 0, errSemExif
	}

	var ordem binary.ByteOrder
	switch {
	case b[0] == 'I' && b[1] == 'I':
		ordem = binary.LittleEndian
	case b[0] == 'M' && b[1] == 'M':
		ordem = binary.BigEndian
	default:
		return 0, errSemExif
	}
	if ordem.Uint16(b[2:4]) != 42 {
		return 0, errSemExif
	}

	achadas := map[uint16]string{}
	lerIFD(b, ordem, int(ordem.Uint32(b[4:8])), achadas, 0)

	for _, tag := range []uint16{tagDateTimeOriginal, tagDateTimeDigital, tagDateTime} {
		if bruto, ok := achadas[tag]; ok {
			if quando, err := parseDataExif(bruto); err == nil {
				return quando, nil
			}
		}
	}
	return 0, errSemExif
}

// lerIFD percorre um diretório de tags. profundidade existe para o ponteiro do
// Exif IFD não poder criar um ciclo — um arquivo corrompido ou malicioso
// apontando para si mesmo travaria o scan inteiro.
func lerIFD(b []byte, ordem binary.ByteOrder, offset int, achadas map[uint16]string, profundidade int) {
	if profundidade > 2 || offset <= 0 || offset+2 > len(b) {
		return
	}
	n := int(ordem.Uint16(b[offset : offset+2]))
	base := offset + 2

	for i := range n {
		entrada := base + i*12
		if entrada+12 > len(b) {
			return
		}
		tag := ordem.Uint16(b[entrada : entrada+2])
		tipo := ordem.Uint16(b[entrada+2 : entrada+4])
		quantidade := int(ordem.Uint32(b[entrada+4 : entrada+8]))
		valor := b[entrada+8 : entrada+12]

		if tag == tagExifIFD && tipo == 4 { // LONG: ponteiro para o Exif IFD
			lerIFD(b, ordem, int(ordem.Uint32(valor)), achadas, profundidade+1)
			continue
		}
		if tipo != 2 || quantidade < 19 { // ASCII, tamanho de "AAAA:MM:DD HH:MM:SS"
			continue
		}
		// Valor maior que 4 bytes mora fora da entrada, apontado por offset.
		pos := int(ordem.Uint32(valor))
		if pos <= 0 || pos+quantidade > len(b) {
			continue
		}
		achadas[tag] = string(bytes.TrimRight(b[pos:pos+quantidade], "\x00 "))
	}
}

// parseDataExif lê "2019:07:14 18:32:05". O EXIF não guarda fuso: a hora é a
// que aparecia no relógio da câmera, então é lida como local — que é como a
// pessoa se lembra dela.
func parseDataExif(bruto string) (int64, error) {
	t, err := time.ParseInLocation("2006:01:02 15:04:05", bruto, time.Local)
	if err != nil {
		return 0, err
	}
	// Câmera sem relógio ajustado grava 1970 ou 0000. Uma data assim não é
	// informação, é ruído — melhor cair no mtime do arquivo.
	if t.Year() < 1900 || t.After(time.Now().AddDate(1, 0, 0)) {
		return 0, errSemExif
	}
	return t.Unix(), nil
}
