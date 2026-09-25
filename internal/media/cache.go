package media

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

func hashCurto(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// cabeNoDisco recusa o preparo quando não há espaço com folga.
//
// O cache mora no mesmo volume do nas.db e do seu WAL. Encher o disco não
// derrubaria só a reprodução: o SQLite passaria a devolver SQLITE_FULL e o
// login e o progresso parariam junto. Por isso existe uma reserva intocável.
func (p *Preparador) cabeNoDisco(pedido Pedido) error {
	// Estimativa grosseira: remux ocupa o tamanho do original; recodificação
	// costuma caber em metade. Sem duração, assume o tamanho do original.
	var estimado int64
	if info, err := os.Stat(pedido.Origem); err == nil {
		estimado = info.Size()
		if pedido.Receita == "video1080" {
			estimado /= 2
		}
	}

	livre, err := espacoLivre(p.dir)
	if err != nil {
		return nil // sem saber, não bloqueia
	}
	if livre-estimado < p.reservado {
		p.limparExcedente()
		livre, _ = espacoLivre(p.dir)
		if livre-estimado < p.reservado {
			return fmt.Errorf("espaço insuficiente: precisa de ~%.1f GB e o disco precisa manter %.1f GB livres",
				float64(estimado)/1e9, float64(p.reservado)/1e9)
		}
	}
	return nil
}

func espacoLivre(dir string) (int64, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(dir, &fs); err != nil {
		return 0, err
	}
	return int64(fs.Bavail) * int64(fs.Bsize), nil
}

type entradaCache struct {
	caminho string
	tamanho int64
	acesso  int64 // unix
}

// limparExcedente aplica o orçamento do cache removendo o que foi usado há mais
// tempo. Só apaga arquivos preparados: nunca toca no acervo.
func (p *Preparador) limparExcedente() {
	entradas, total := p.inventario()
	if total <= p.limite {
		return
	}

	// Mais antigo primeiro.
	sort.Slice(entradas, func(i, j int) bool { return entradas[i].acesso < entradas[j].acesso })

	for _, e := range entradas {
		if total <= p.limite {
			return
		}
		if err := os.Remove(e.caminho); err != nil {
			continue
		}
		total -= e.tamanho
		log.Printf("cache de preparo: removi %s (%.1f MB) para respeitar o orçamento",
			filepath.Base(e.caminho), float64(e.tamanho)/1e6)
	}
}

func (p *Preparador) inventario() ([]entradaCache, int64) {
	var entradas []entradaCache
	var total int64

	itens, err := os.ReadDir(p.dir)
	if err != nil {
		return nil, 0
	}
	for _, item := range itens {
		if item.IsDir() {
			continue
		}
		info, err := item.Info()
		if err != nil {
			continue
		}
		caminho := filepath.Join(p.dir, item.Name())
		entradas = append(entradas, entradaCache{
			caminho: caminho,
			tamanho: info.Size(),
			acesso:  atime(info),
		})
		total += info.Size()
	}
	return entradas, total
}

// LimparParciais roda no boot: um preparo interrompido pela queda do servidor
// deixa um temporário que ninguém mais vai terminar.
func (p *Preparador) LimparParciais() {
	itens, err := os.ReadDir(p.dir)
	if err != nil {
		return
	}
	removidos := 0
	for _, item := range itens {
		if item.IsDir() || !strings.HasPrefix(item.Name(), "preparo-") {
			continue
		}
		if err := os.Remove(filepath.Join(p.dir, item.Name())); err == nil {
			removidos++
		}
	}
	if removidos > 0 {
		log.Printf("cache de preparo: %d arquivo(s) incompleto(s) de execução anterior removido(s)", removidos)
	}
}

// Uso devolve o tamanho ocupado e o orçamento, para a tela de configurações.
func (p *Preparador) Uso() (usado, limite int64) {
	_, total := p.inventario()
	return total, p.limite
}

// Disco devolve o espaço livre e o total do volume onde o cache de preparo
// mora — o mesmo volume do nas.db, que é o motivo de a reserva existir.
func (p *Preparador) Disco() (livre, total int64) {
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return 0, 0
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(p.dir, &fs); err != nil {
		return 0, 0
	}
	return int64(fs.Bavail) * int64(fs.Bsize), int64(fs.Blocks) * int64(fs.Bsize)
}

// Reserva é o espaço livre que o cache nunca consome.
func (p *Preparador) Reserva() int64 { return p.reservado }

// EspacoLivre é o espaço disponível no volume de dir, para quem precisa
// decidir se cabe um arquivo antes de baixá-lo.
func EspacoLivre(dir string) (int64, error) { return espacoLivre(dir) }
