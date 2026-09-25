#!/bin/sh
# Identidade do Mac para o IAM Roles Anywhere: uma CA privada e um certificado
# do Mac assinado por ela. A AWS confia na CA (o trust anchor do OpenTofu); o
# Mac troca o certificado por credenciais de 1 h. Nenhuma access key fixa.
#
#   sh scripts/roles-anywhere.sh            gera tudo e importa no Keychain
#   sh scripts/roles-anywhere.sh --renovar  só um certificado novo (a CA fica)
#
# O que sai em ~/.nas/pki (pasta 0700):
#   ca.pem   vai no terraform.tfvars (ca_pem). Público, pode colar.
#   ca.key   a chave da CA. Quem a tem emite certificados que a AWS aceita:
#            guarde num lugar offline (um pendrive, o gerenciador de senhas) e
#            apague daqui depois. Só é preciso de novo para renovar.
#   mac.pem  o certificado do Mac, válido por 1 ano. A chave privada dele vai
#            para o Keychain e é apagada do disco.
#
# Variáveis: NAS_PKI (pasta, padrão ~/.nas/pki), NAS_CN (padrão ozymandias-mac,
# o mesmo cn_do_mac do OpenTofu), NAS_SEM_KEYCHAIN=1 (não importa; para teste).
set -eu

OPENSSL="${OPENSSL:-openssl}"
case "$($OPENSSL version)" in
  LibreSSL*) echo "precisa do OpenSSL 3 (brew install openssl@3), não do LibreSSL do macOS"; exit 1 ;;
esac

DIR="${NAS_PKI:-$HOME/.nas/pki}"
CN="${NAS_CN:-ozymandias-mac}"
RENOVAR=0
[ "${1:-}" = "--renovar" ] && RENOVAR=1

umask 077
mkdir -p "$DIR"
chmod 700 "$DIR"
cd "$DIR"

if [ "$RENOVAR" -eq 0 ]; then
  if [ -e ca.key ] || [ -e ca.pem ]; then
    echo "já existe uma CA em $DIR. Para só trocar o certificado do Mac: --renovar"
    exit 1
  fi
  # P-256: aceito pelo Roles Anywhere e bem menor que RSA.
  "$OPENSSL" ecparam -name prime256v1 -genkey -noout -out ca.key
  "$OPENSSL" req -x509 -new -key ca.key -sha256 -days 3650 \
    -subj "/CN=Ozymandias CA/O=Ozymandias" \
    -addext "basicConstraints=critical,CA:TRUE" \
    -addext "keyUsage=critical,keyCertSign,cRLSign" \
    -addext "subjectKeyIdentifier=hash" \
    -out ca.pem
  echo "CA criada: $DIR/ca.pem (10 anos)"
else
  [ -e ca.key ] || { echo "sem ca.key em $DIR: traga a chave da CA de volta para renovar"; exit 1; }
fi

"$OPENSSL" ecparam -name prime256v1 -genkey -noout -out mac.key
"$OPENSSL" req -new -key mac.key -subj "/CN=$CN" -out mac.csr
cat > mac.ext <<EOF
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature
extendedKeyUsage=clientAuth
subjectKeyIdentifier=hash
authorityKeyIdentifier=keyid
EOF
"$OPENSSL" x509 -req -in mac.csr -CA ca.pem -CAkey ca.key -CAcreateserial \
  -days 365 -sha256 -extfile mac.ext -out mac.pem
"$OPENSSL" verify -CAfile ca.pem mac.pem >/dev/null
echo "certificado do Mac: $DIR/mac.pem (CN=$CN, 1 ano)"

if [ "${NAS_SEM_KEYCHAIN:-0}" = "1" ]; then
  echo "NAS_SEM_KEYCHAIN=1: a chave do Mac ficou em $DIR/mac.key (só para teste)"
  rm -f mac.csr mac.ext
  exit 0
fi

# Keychain: o aws_signing_helper acha o certificado pelo CN (--cert-selector)
# e assina com a chave sem ela existir em arquivo. A senha do .p12 é
# descartável: só vale para esta importação.
SENHA="$("$OPENSSL" rand -hex 16)"
"$OPENSSL" pkcs12 -export -legacy -inkey mac.key -in mac.pem -name "$CN" \
  -out mac.p12 -passout "pass:$SENHA"
security import mac.p12 -k "$HOME/Library/Keychains/login.keychain-db" -P "$SENHA"
rm -f mac.key mac.p12 mac.csr mac.ext
echo "chave do Mac importada no Keychain (login) e apagada do disco."
echo "Na primeira credencial o macOS pergunta se o aws_signing_helper pode usá-la: 'Sempre permitir'."
echo
echo "Próximo passo: cole o conteúdo de $DIR/ca.pem em ca_pem no infra/terraform.tfvars."
