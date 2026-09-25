package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"nas/internal/db"
)

// TokenDeMidiaTTL é quanto vale uma credencial de URL. Precisa cobrir um filme
// inteiro com folga para pausa e AirPlay, e nada além disso: é a única
// credencial deste servidor que viaja à vista, em lugar nenhum protegida por
// HttpOnly.
const TokenDeMidiaTTL = 6 * time.Hour

// ErrTokenDeMidia é o que sai de qualquer forma de recusa — assinatura errada,
// prazo vencido, sessão derrubada. Não distingue os casos de propósito: quem
// está tentando adivinhar não merece a dica.
var ErrTokenDeMidia = errors.New("token de mídia inválido ou expirado")

// Tokens de mídia existem porque nem todo player é o navegador.
//
// O AVPlayer do iOS abre a URL do vídeo sozinho, num processo fora do
// URLSession do app: ou o cookie vai junto por AVURLAssetHTTPCookiesKey — que
// não sobrevive ao AirPlay nem ao download em segundo plano — ou a credencial
// viaja na própria URL. Esta é a segunda opção, desenhada para vazar pouco:
//
//   - vale horas, não trinta dias;
//   - abre a mídia (vídeo, preparado, capa, legenda) e nada da API;
//   - morre junto com a sessão que a gerou — sair da conta ou trocar a senha
//     apaga a sessão no banco, e a validação abaixo passa a falhar;
//   - morre junto com o processo, porque a chave que assina é sorteada no
//     boot e nunca é gravada.
//
// O corpo carrega o hash da sessão, que é a chave usada no banco. Ele não é
// uma credencial: o cookie leva o token em claro e o servidor o hasheia antes
// de procurar, então quem só tem o hash não consegue se passar por ninguém —
// e quem viu a URL já tinha o token de mídia inteiro de qualquer forma.

// TokenDeMidia assina uma credencial de URL amarrada à sessão que a pediu.
// sessionToken é o token em claro, o mesmo que veio no cookie ou no Bearer.
func (s *Service) TokenDeMidia(sessionToken string, ttl time.Duration) (string, time.Time) {
	expira := time.Now().Add(ttl)
	corpo := fmt.Sprintf("%s.%d", hashToken(sessionToken), expira.Unix())
	return corpo + "." + s.assinaMidia(corpo), expira
}

// UserFromMediaToken valida a credencial e devolve o dono dela.
func (s *Service) UserFromMediaToken(ctx context.Context, token string) (db.User, error) {
	sessao, expira, assinatura, ok := partesDoToken(token)
	if !ok {
		return db.User{}, ErrTokenDeMidia
	}

	// A assinatura primeiro: sem ela, nada do resto merece um round-trip no
	// banco. hmac.Equal para o tempo de resposta não contar o prefixo certo.
	corpo := sessao + "." + expira
	if !hmac.Equal([]byte(assinatura), []byte(s.assinaMidia(corpo))) {
		return db.User{}, ErrTokenDeMidia
	}

	unix, err := strconv.ParseInt(expira, 10, 64)
	if err != nil || time.Now().After(time.Unix(unix, 0)) {
		return db.User{}, ErrTokenDeMidia
	}

	u, err := s.db.UserBySession(ctx, sessao)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return db.User{}, ErrTokenDeMidia
		}
		return db.User{}, err
	}
	return u, nil
}

// partesDoToken separa <sessão>.<expiração>.<assinatura>. Nenhuma das três
// pode conter ponto: o hash é hex, a expiração é um inteiro e a assinatura é
// base64url sem preenchimento. Então três partes exatas, ou nada.
func partesDoToken(token string) (sessao, expira, assinatura string, ok bool) {
	partes := strings.Split(token, ".")
	if len(partes) != 3 || partes[0] == "" || partes[1] == "" || partes[2] == "" {
		return "", "", "", false
	}
	return partes[0], partes[1], partes[2], true
}

func (s *Service) assinaMidia(corpo string) string {
	m := hmac.New(sha256.New, s.chaveMidia)
	m.Write([]byte(corpo))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
