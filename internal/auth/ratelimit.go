package auth

import (
	"sync"
	"time"
)

// Limiter aplica backoff progressivo por IP nas tentativas de login. O tunnel
// deixa a tela de login exposta na internet, então força bruta é cenário real.
//
// As três primeiras falhas passam batido (erro de digitação acontece); a partir
// daí o bloqueio dobra: 2s, 4s, 8s… até 5 minutos.
type Limiter struct {
	mu      sync.Mutex
	entries map[string]*entry
}

type entry struct {
	failures int
	until    time.Time
	seen     time.Time
}

const (
	freeAttempts = 3
	maxBlock     = 5 * time.Minute
	entryTTL     = time.Hour
)

func NewLimiter() *Limiter {
	return &Limiter{entries: make(map[string]*entry)}
}

// Blocked informa se o IP está de castigo e por quanto tempo ainda.
func (l *Limiter) Blocked(ip string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.evictLocked()

	e, ok := l.entries[ip]
	if !ok {
		return 0, false
	}
	if remaining := time.Until(e.until); remaining > 0 {
		return remaining, true
	}
	return 0, false
}

// Fail registra uma tentativa malsucedida.
func (l *Limiter) Fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	e, ok := l.entries[ip]
	if !ok {
		e = &entry{}
		l.entries[ip] = e
	}
	e.failures++
	e.seen = time.Now()

	if e.failures > freeAttempts {
		block := time.Duration(1<<(e.failures-freeAttempts)) * time.Second
		if block > maxBlock {
			block = maxBlock
		}
		e.until = time.Now().Add(block)
	}
}

// Reset limpa o histórico do IP após um login bem-sucedido.
func (l *Limiter) Reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, ip)
}

// evictLocked descarta IPs parados há mais de uma hora, para o mapa não virar
// um vazamento de memória em um servidor exposto.
func (l *Limiter) evictLocked() {
	cutoff := time.Now().Add(-entryTTL)
	for ip, e := range l.entries {
		if e.seen.Before(cutoff) && e.until.Before(time.Now()) {
			delete(l.entries, ip)
		}
	}
}
