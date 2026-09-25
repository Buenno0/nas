package energia

import "testing"

func TestInterpretar(t *testing.T) {
	bat := "Now drawing from 'Battery Power'\n -InternalBattery-0 (id=1)\t71%; discharging; 10:51 remaining present: true\n"
	if na, carga := InterpretarBateria(bat); !na || carga != 71 {
		t.Fatalf("bateria: %v %d", na, carga)
	}
	if na, _ := InterpretarBateria("Now drawing from 'AC Power'\n -InternalBattery-0\t100%; charged;"); na {
		t.Fatal("na tomada lido como bateria")
	}
	frio := "Note: No thermal warning level has been recorded\nNote: No performance warning level has been recorded\n"
	if InterpretarTermica(frio) {
		t.Fatal("sem aviso lido como quente")
	}
	if !InterpretarTermica("CPU_Scheduler_Limit \t= 100\nCPU_Available_CPUs \t= 10\nCPU_Speed_Limit \t= 70\n") {
		t.Fatal("CPU limitada a 70% não lida como quente")
	}
	if !InterpretarTermica("Thermal Warning Level = 2") {
		t.Fatal("aviso térmico não lido")
	}
}
