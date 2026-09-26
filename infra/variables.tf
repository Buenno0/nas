variable "regiao" {
  type    = string
  default = "us-east-1"
}

variable "bucket" {
  description = "Nome globalmente único do bucket de mídia."
  type        = string
}

variable "origens_web" {
  description = "Origens que podem enviar direto ao bucket (CORS do upload do navegador). Ex.: http://macbook.local:8787, https://*.trycloudflare.com"
  type        = list(string)
  default     = ["http://localhost:8787", "https://*.ts.net"]
}

variable "ca_pem" {
  description = "Certificado (PEM) da CA privada do Roles Anywhere. Gere com scripts/ou openssl; a chave privada da CA não sai do Mac."
  type        = string
}

variable "cn_do_mac" {
  description = "Common Name do certificado do Mac, conferido na trust policy."
  type        = string
  default     = "ozymandias-mac"
}

variable "orcamento_usd" {
  description = "Limite mensal do AWS Budgets."
  type        = number
  default     = 10
}

variable "email_alerta" {
  type = string
}

variable "workers_max" {
  description = "Réplicas máximas do worker. Zero sem fila; sobe com a profundidade."
  type        = number
  default     = 3
}

variable "imagem_tag" {
  description = "Tag da imagem do worker no ECR."
  type        = string
  default     = "latest"
}

variable "tailscale_authkey" {
  description = "Auth key do Tailscale para a instância cloud: reutilizável, efêmera, com tag (tag:ozymandias). Vai para o SSM como SecureString."
  type        = string
  sensitive   = true
  default     = ""
}

variable "funnel" {
  description = "Expor a instância cloud na internet pelo Tailscale Funnel: abre em qualquer navegador, sem o app. A senha e o limite de tentativas continuam valendo. false = só a sua tailnet acessa."
  type        = bool
  default     = true
}

variable "tmdb_key" {
  description = "Chave do TMDB para a instância cloud dar capa na hora ao que for enviado por ela. Vai para o SSM como SecureString. Vazio: a capa chega depois, pelo snapshot do Mac."
  type        = string
  sensitive   = true
  default     = ""
}

variable "nome_na_tailnet" {
  description = "Nome da instância cloud na tailnet: o endereço vira https://<nome>.<tailnet>.ts.net. Trocar de nome pede um certificado novo."
  type        = string
  default     = "ozymandias-nuvem"
}

variable "aceleracao" {
  description = "S3 Transfer Acceleration nos envios (partes do multipart). +US$ 0,04/GB enviado."
  type        = bool
  default     = true
}
