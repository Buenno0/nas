package auth

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nas/internal/db"
)

// preparaServico devolve um serviço com um usuário logado e o token da sessão
// em claro — o mesmo que o cookie levaria.
func preparaServico(t *testing.T) (*Service, string) {
	t.Helper()
	database, err := db.OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("abrindo banco: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	s := NewService(database)
	ctx := context.Background()
	if err := s.CreateUser(ctx, "espectador", "senha-do-espectador", false); err != nil {
		t.Fatalf("criando usuário: %v", err)
	}
	sessao, _, _, err := s.Login(ctx, "127.0.0.1", "espectador", "senha-do-espectador", "teste", true)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return s, sessao
}

func TestTokenDeMidiaValido(t *testing.T) {
	s, sessao := preparaServico(t)
	ctx := context.Background()

	token, expira := s.TokenDeMidia(sessao, TokenDeMidiaTTL)
	if !expira.After(time.Now()) {
		t.Fatalf("token já nasceu vencido: expira em %s", expira)
	}

	u, err := s.UserFromMediaToken(ctx, token)
	if err != nil {
		t.Fatalf("token recém-assinado foi recusado: %v", err)
	}
	if u.Username != "espectador" {
		t.Errorf("token abriu como %q, quero espectador", u.Username)
	}

	// A credencial vai numa URL: precisa caber nela sem escapar nada.
	if strings.ContainsAny(token, "/+= &?#%") {
		t.Errorf("token tem caractere que a URL precisaria escapar: %q", token)
	}
	// E o token da sessão, esse, não pode aparecer em lugar nenhum dele.
	if strings.Contains(token, sessao) {
		t.Error("o token de sessão em claro vazou dentro do token de mídia")
	}
}

// Sem a chave, não se fabrica credencial. Cada caso aqui é uma tentativa que o
// servidor tem de recusar sem pensar duas vezes.
func TestTokenDeMidiaRecusado(t *testing.T) {
	s, sessao := preparaServico(t)
	ctx := context.Background()
	bom, _ := s.TokenDeMidia(sessao, TokenDeMidiaTTL)
	partes := strings.Split(bom, ".")

	casos := map[string]string{
		"vazio":              "",
		"sem partes":         "aaaa",
		"partes de menos":    partes[0] + "." + partes[1],
		"partes demais":      bom + ".extra",
		"assinatura trocada": partes[0] + "." + partes[1] + ".YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY3OA",
		// O clássico: esticar a validade e torcer para ninguém conferir a
		// assinatura sobre o corpo inteiro.
		"prazo esticado": partes[0] + "." + "99999999999" + "." + partes[2],
		// E o inverso: assinatura boa de outro corpo.
		"sessão trocada": strings.Repeat("a", 64) + "." + partes[1] + "." + partes[2],
	}

	for nome, token := range casos {
		t.Run(nome, func(t *testing.T) {
			if _, err := s.UserFromMediaToken(ctx, token); err == nil {
				t.Error("token forjado foi aceito")
			}
		})
	}
}

func TestTokenDeMidiaVence(t *testing.T) {
	s, sessao := preparaServico(t)
	// TTL negativo: já nasce no passado, assinado corretamente.
	token, _ := s.TokenDeMidia(sessao, -time.Minute)
	if _, err := s.UserFromMediaToken(context.Background(), token); err == nil {
		t.Error("token vencido foi aceito")
	}
}

// A credencial está amarrada à sessão de propósito: é o que faz "sair da
// conta" valer também para o link que estava aberto no player.
func TestTokenDeMidiaMorreComASessao(t *testing.T) {
	s, sessao := preparaServico(t)
	ctx := context.Background()
	token, _ := s.TokenDeMidia(sessao, TokenDeMidiaTTL)

	if _, err := s.UserFromMediaToken(ctx, token); err != nil {
		t.Fatalf("token válido recusado antes do logout: %v", err)
	}
	if err := s.Logout(ctx, sessao); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := s.UserFromMediaToken(ctx, token); err == nil {
		t.Error("o token continuou valendo depois de sair da conta")
	}
}

// Dois servidores são dois segredos. Reiniciar invalida o que estava à vista.
func TestTokenDeMidiaNaoAtravessaBoot(t *testing.T) {
	s, sessao := preparaServico(t)
	token, _ := s.TokenDeMidia(sessao, TokenDeMidiaTTL)

	outro := NewService(s.db) // mesmo banco, chave nova
	if _, err := outro.UserFromMediaToken(context.Background(), token); err == nil {
		t.Error("token assinado por outra chave foi aceito")
	}
}
