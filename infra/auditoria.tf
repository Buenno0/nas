# V5: revisão de menor privilégio e trilha de auditoria.
#
# - IAM Access Analyzer (de graça): aponta qualquer recurso deste conjunto que
#   fique acessível de fora da conta — bucket, fila, tópico, role, chave.
# - Access Analyzer de acesso não usado (pago, ~US$ 0,20 por role/mês): lista
#   permissões que as roles do Ozymandias têm e nunca usaram, que é o insumo
#   para enxugar as policies depois de algumas semanas rodando.
# - CloudTrail: a primeira trilha de eventos de gestão é gratuita; paga-se só
#   o S3 dos logs, que expiram em 90 dias.

resource "aws_accessanalyzer_analyzer" "externo" {
  analyzer_name = "ozymandias-acesso-externo"
  type          = "ACCOUNT"
}

variable "analisar_acesso_nao_usado" {
  description = "Liga o analisador de permissões não usadas (pago, por role)."
  type        = bool
  default     = false
}

resource "aws_accessanalyzer_analyzer" "nao_usado" {
  count         = var.analisar_acesso_nao_usado ? 1 : 0
  analyzer_name = "ozymandias-acesso-nao-usado"
  type          = "ACCOUNT_UNUSED_ACCESS"
  configuration {
    unused_access {
      unused_access_age = 30
    }
  }
}

# Bucket próprio para a trilha: separar dos dados de mídia impede que a role
# do Mac (que escreve no bucket de mídia) possa mexer nos logs de auditoria.
resource "aws_s3_bucket" "trilha" {
  bucket = "${var.bucket}-trilha"
}

resource "aws_s3_bucket_public_access_block" "trilha" {
  bucket                  = aws_s3_bucket.trilha.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_lifecycle_configuration" "trilha" {
  bucket = aws_s3_bucket.trilha.id
  rule {
    id     = "expira"
    status = "Enabled"
    filter {}
    expiration {
      days = 90
    }
  }
}

resource "aws_s3_bucket_policy" "trilha" {
  bucket = aws_s3_bucket.trilha.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "CloudTrailConfereACL"
        Effect    = "Allow"
        Principal = { Service = "cloudtrail.amazonaws.com" }
        Action    = "s3:GetBucketAcl"
        Resource  = aws_s3_bucket.trilha.arn
        Condition = { StringEquals = { "aws:SourceArn" = "arn:aws:cloudtrail:${var.regiao}:${data.aws_caller_identity.atual.account_id}:trail/ozymandias" } }
      },
      {
        Sid       = "CloudTrailEscreve"
        Effect    = "Allow"
        Principal = { Service = "cloudtrail.amazonaws.com" }
        Action    = "s3:PutObject"
        Resource  = "${aws_s3_bucket.trilha.arn}/AWSLogs/${data.aws_caller_identity.atual.account_id}/*"
        Condition = {
          StringEquals = {
            "s3:x-amz-acl"  = "bucket-owner-full-control"
            "aws:SourceArn" = "arn:aws:cloudtrail:${var.regiao}:${data.aws_caller_identity.atual.account_id}:trail/ozymandias"
          }
        }
      },
      {
        Sid       = "SoTLS"
        Effect    = "Deny"
        Principal = "*"
        Action    = "s3:*"
        Resource  = [aws_s3_bucket.trilha.arn, "${aws_s3_bucket.trilha.arn}/*"]
        Condition = { Bool = { "aws:SecureTransport" = "false" } }
      },
    ]
  })
  depends_on = [aws_s3_bucket_public_access_block.trilha]
}

resource "aws_cloudtrail" "ozymandias" {
  name                          = "ozymandias"
  s3_bucket_name                = aws_s3_bucket.trilha.id
  is_multi_region_trail         = true
  include_global_service_events = true
  enable_log_file_validation    = true
  depends_on                    = [aws_s3_bucket_policy.trilha]

  # Eventos de dados do bucket de mídia custam por evento: só as escritas e
  # remoções, que são o que importa auditar (quem apagou o quê).
  advanced_event_selector {
    name = "gestao"
    field_selector {
      field  = "eventCategory"
      equals = ["Management"]
    }
  }
  advanced_event_selector {
    name = "escritas-na-midia"
    field_selector {
      field  = "eventCategory"
      equals = ["Data"]
    }
    field_selector {
      field  = "resources.type"
      equals = ["AWS::S3::Object"]
    }
    field_selector {
      field       = "resources.ARN"
      starts_with = ["${aws_s3_bucket.midia.arn}/bibliotecas/"]
    }
    field_selector {
      field  = "eventName"
      equals = ["DeleteObject", "DeleteObjects"]
    }
  }
}
