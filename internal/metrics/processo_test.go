package metrics

import (
	"testing"
	"time"
)

func TestAmostraPrimeiraLeituraSemCPUInventada(t *testing.T) {
	a := NovoAmostradorProcesso()
	am := a.Amostra()

	// Sem leitura anterior não existe delta: zero é a única resposta honesta.
	if am.CPUPercent != 0 || am.CPUNucleos != 0 {
		t.Fatalf("primeira amostra devia ter CPU zero, veio %+v", am)
	}
	if am.Goroutines <= 0 {
		t.Fatalf("goroutines = %d, o próprio teste já é uma", am.Goroutines)
	}
	if am.Nucleos <= 0 {
		t.Fatalf("nucleos = %d", am.Nucleos)
	}
	if am.HeapBytes == 0 {
		t.Fatal("heap zerado é impossível com o runtime rodando")
	}
}

func TestAmostraSegundaLeituraNaoFicaNegativa(t *testing.T) {
	a := NovoAmostradorProcesso()
	a.Amostra()

	// Queima um pouco de CPU para o delta não ser degenerado.
	fim := time.Now().Add(30 * time.Millisecond)
	x := 0
	for time.Now().Before(fim) {
		x++
	}
	_ = x

	am := a.Amostra()
	if am.CPUPercent < 0 || am.CPUNucleos < 0 {
		t.Fatalf("CPU negativa: %+v", am)
	}
	if am.CPUPercent > 100 {
		t.Fatalf("CPU acima de 100%% da máquina: %v", am.CPUPercent)
	}
}

func TestRssEmBytes(t *testing.T) {
	if got := rssEmBytes(-1); got != 0 {
		t.Fatalf("valor negativo devia virar 0, veio %d", got)
	}
	if got := rssEmBytes(1024); got == 0 {
		t.Fatal("1024 não devia virar 0")
	}
}
