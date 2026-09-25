# Leitura pela CDN: o bucket continua privado (OAC) e só aceita o CloudFront;
# o CloudFront só serve URLs assinadas por uma chave cujo par privado mora no
# SSM e só é lido para a memória do Ozymandias ao entrar no híbrido.
#
# A chave é gerada aqui para dispensar openssl à mão. Consequência: ela fica
# também no tfstate. O .gitignore protege o state; se preferir, gere a chave
# fora e troque tls_private_key por uma variável.
resource "tls_private_key" "cdn" {
  algorithm = "RSA"
  rsa_bits  = 2048
}

resource "aws_ssm_parameter" "chave_cdn" {
  name  = "/ozymandias/cdn/chave-privada"
  type  = "SecureString"
  value = tls_private_key.cdn.private_key_pem
}

resource "aws_cloudfront_public_key" "cdn" {
  name        = "ozymandias"
  encoded_key = tls_private_key.cdn.public_key_pem
}

resource "aws_cloudfront_key_group" "cdn" {
  name  = "ozymandias"
  items = [aws_cloudfront_public_key.cdn.id]
}

resource "aws_cloudfront_origin_access_control" "midia" {
  name                              = "ozymandias-midia"
  origin_access_control_origin_type = "s3"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
}

data "aws_cloudfront_cache_policy" "otimizado" {
  name = "Managed-CachingOptimized"
}

resource "aws_cloudfront_distribution" "midia" {
  enabled         = true
  comment         = "Ozymandias: leitura de mídia com URL assinada"
  price_class     = "PriceClass_100" # América do Norte e Europa: o mais barato
  is_ipv6_enabled = true

  origin {
    origin_id                = "s3"
    domain_name              = aws_s3_bucket.midia.bucket_regional_domain_name
    origin_access_control_id = aws_cloudfront_origin_access_control.midia.id
  }

  default_cache_behavior {
    target_origin_id       = "s3"
    viewer_protocol_policy = "https-only"
    allowed_methods        = ["GET", "HEAD"]
    cached_methods         = ["GET", "HEAD"]
    cache_policy_id        = data.aws_cloudfront_cache_policy.otimizado.id
    trusted_key_groups     = [aws_cloudfront_key_group.cdn.id]
  }

  # O journal (eventos/) não é lido pela CDN: a bucket policy só libera
  # bibliotecas/* para ela.

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  # Sem domínio pago: o *.cloudfront.net gratuito, com o certificado padrão.
  viewer_certificate {
    cloudfront_default_certificate = true
  }
}
