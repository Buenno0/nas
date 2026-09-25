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
  default     = ["http://localhost:8787"]
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
