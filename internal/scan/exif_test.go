package scan

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// jpegComExif monta um JPEG mínimo com um bloco EXIF de verdade: APP1,
// assinatura, cabeçalho TIFF, IFD0 apontando para o Exif IFD e a tag da data.
// Gerar em vez de versionar um binário mantém o teste legível e nas duas ordens
// de bytes — câmeras Canon gravam II, Nikon grava MM, e trocar as duas é o erro
// clássico de quem lê EXIF.
func jpegComExif(t *testing.T, data string, bigEndian bool, tag uint16) string {
	t.Helper()

	var ordem binary.ByteOrder = binary.LittleEndian
	bom := []byte{'I', 'I', 0x2a, 0x00}
	if bigEndian {
		ordem = binary.BigEndian
		bom = []byte{'M', 'M', 0x00, 0x2a}
	}
	u16 := func(v uint16) []byte { b := make([]byte, 2); ordem.PutUint16(b, v); return b }
	u32 := func(v uint32) []byte { b := make([]byte, 4); ordem.PutUint32(b, v); return b }

	valor := append([]byte(data), 0)
	const ifd0Off = 8
	tamIFD0 := 2 + 12 + 4
	exifOff := ifd0Off + tamIFD0
	valOff := exifOff + 2 + 12 + 4

	var ifd0 []byte
	ifd0 = append(ifd0, u16(1)...)
	ifd0 = append(ifd0, u16(tagExifIFD)...)
	ifd0 = append(ifd0, u16(4)...)
	ifd0 = append(ifd0, u32(1)...)
	ifd0 = append(ifd0, u32(uint32(exifOff))...)
	ifd0 = append(ifd0, u32(0)...)

	var exifIFD []byte
	exifIFD = append(exifIFD, u16(1)...)
	exifIFD = append(exifIFD, u16(tag)...)
	exifIFD = append(exifIFD, u16(2)...) // ASCII
	exifIFD = append(exifIFD, u32(uint32(len(valor)))...)
	exifIFD = append(exifIFD, u32(uint32(valOff))...)
	exifIFD = append(exifIFD, u32(0)...)

	tiff := append(append(append(append(bom, u32(ifd0Off)...), ifd0...), exifIFD...), valor...)
	app1 := append([]byte("Exif\x00\x00"), tiff...)

	jpeg := []byte{0xff, 0xd8, 0xff, 0xe1}
	tamanho := make([]byte, 2)
	binary.BigEndian.PutUint16(tamanho, uint16(len(app1)+2))
	jpeg = append(append(jpeg, tamanho...), app1...)
	jpeg = append(jpeg, 0xff, 0xd9)

	caminho := filepath.Join(t.TempDir(), "foto.jpg")
	if err := os.WriteFile(caminho, jpeg, 0o644); err != nil {
		t.Fatal(err)
	}
	return caminho
}

func TestDataDeCaptura(t *testing.T) {
	casos := []struct {
		nome      string
		data      string
		bigEndian bool
		tag       uint16
		quero     string // vazio = tem de devolver 0
	}{
		{"little endian (II)", "2019:07:14 18:32:05", false, tagDateTimeOriginal, "2019-07-14 18:32:05"},
		{"big endian (MM)", "2011:03:05 09:01:02", true, tagDateTimeOriginal, "2011-03-05 09:01:02"},
		{"digitalizada serve de reserva", "2015:01:02 03:04:05", false, tagDateTimeDigital, "2015-01-02 03:04:05"},
		{"modificação serve por último", "2001:12:31 23:59:59", false, tagDateTime, "2001-12-31 23:59:59"},

		// Câmera com relógio zerado grava isto. Não é data, é ruído: devolver
		// 1970 colocaria a foto no começo de qualquer linha do tempo.
		{"relógio zerado não vira 1970", "0000:00:00 00:00:00", false, tagDateTimeOriginal, ""},
		{"texto que não é data", "nao sou uma data!!", false, tagDateTimeOriginal, ""},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got := DataDeCaptura(jpegComExif(t, c.data, c.bigEndian, c.tag))
			if c.quero == "" {
				if got != 0 {
					t.Fatalf("queria 0 (dado inútil), veio %d (%s)", got, time.Unix(got, 0))
				}
				return
			}
			quero, err := time.ParseInLocation("2006-01-02 15:04:05", c.quero, time.Local)
			if err != nil {
				t.Fatal(err)
			}
			if got != quero.Unix() {
				t.Fatalf("data = %s, quero %s", time.Unix(got, 0), quero)
			}
		})
	}
}

// Arquivo sem EXIF nenhum não pode travar nem inventar data.
func TestDataDeCapturaSemExif(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "vazio.jpg")
	os.WriteFile(caminho, []byte{0xff, 0xd8, 0xff, 0xd9}, 0o644)
	if got := DataDeCaptura(caminho); got != 0 {
		t.Fatalf("veio %d de um arquivo sem EXIF", got)
	}
	if got := DataDeCaptura("/caminho/que/nao/existe.jpg"); got != 0 {
		t.Fatalf("veio %d de um arquivo inexistente", got)
	}
}
