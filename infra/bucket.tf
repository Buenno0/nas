resource "aws_s3_bucket" "midia" {
  bucket = var.bucket
}

resource "aws_s3_bucket_public_access_block" "midia" {
  bucket                  = aws_s3_bucket.midia.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "midia" {
  bucket = aws_s3_bucket.midia.id
  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "midia" {
  bucket = aws_s3_bucket.midia.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# Versioning protege contra sobrescrita e apagamento acidental. Sem backup no
# escopo, é a única rede de segurança do bucket.
resource "aws_s3_bucket_versioning" "midia" {
  bucket = aws_s3_bucket.midia.id
  versioning_configuration {
    status = "Enabled"
  }
}

# O navegador envia as partes direto ao S3 e precisa LER o ETag de cada uma
# para concluir o multipart: sem ExposeHeaders, o upload web não fecha.
resource "aws_s3_bucket_cors_configuration" "midia" {
  bucket = aws_s3_bucket.midia.id
  cors_rule {
    allowed_methods = ["PUT", "GET", "HEAD"]
    # Navegadores mandam o Origin com o host em minúsculas, e o S3 compara
    # diferenciando maiúsculas: "MacBook-Air.local" no tfvars recusava o
    # Safari com 403. As duas formas entram, mais qualquer Mac da rede local.
    allowed_origins = distinct(concat(var.origens_web, [for o in var.origens_web : lower(o)], ["http://*.local:8787"]))
    allowed_headers = ["*"]
    expose_headers  = ["ETag", "Content-Length", "Content-Range", "Accept-Ranges"]
    max_age_seconds = 3600
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "midia" {
  bucket = aws_s3_bucket.midia.id

  # Multipart abandonado (aba fechada, kill switch sem retorno) cobra por
  # armazenamento até ser abortado.
  rule {
    id     = "multipart-orfao"
    status = "Enabled"
    filter {}
    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }

  # Versões antigas somem depois de 30 dias.
  rule {
    id     = "versoes-antigas"
    status = "Enabled"
    filter {}
    noncurrent_version_expiration {
      noncurrent_days = 30
    }
  }

  # Mídia pessoal é lida em rajadas e depois esquecida: Intelligent-Tiering
  # desce o que não é tocado sem custo de recuperação.
  rule {
    id     = "tiering"
    status = "Enabled"
    filter {
      prefix = "bibliotecas/"
    }
    transition {
      days          = 0
      storage_class = "INTELLIGENT_TIERING"
    }
  }
}

# Só HTTPS.
resource "aws_s3_bucket_policy" "midia" {
  bucket = aws_s3_bucket.midia.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "SoTLS"
        Effect    = "Deny"
        Principal = "*"
        Action    = "s3:*"
        Resource  = [aws_s3_bucket.midia.arn, "${aws_s3_bucket.midia.arn}/*"]
        Condition = { Bool = { "aws:SecureTransport" = "false" } }
      },
      {
        # OAC: só esta distribuição lê, e só a mídia.
        Sid       = "CloudFrontLeMidia"
        Effect    = "Allow"
        Principal = { Service = "cloudfront.amazonaws.com" }
        Action    = "s3:GetObject"
        Resource  = ["${aws_s3_bucket.midia.arn}/bibliotecas/*", "${aws_s3_bucket.midia.arn}/derivados/*"]
        Condition = { StringEquals = { "AWS:SourceArn" = aws_cloudfront_distribution.midia.arn } }
      },
    ]
  })
  depends_on = [aws_s3_bucket_public_access_block.midia]
}
