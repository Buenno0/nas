# IAM Roles Anywhere: o Mac troca um certificado X.509 (chave privada no
# Keychain) por credenciais STS de 1 h. Nenhuma access key fixa existe.
resource "aws_rolesanywhere_trust_anchor" "mac" {
  name    = "ozymandias-mac"
  enabled = true
  source {
    source_type = "CERTIFICATE_BUNDLE"
    source_data {
      x509_certificate_data = var.ca_pem
    }
  }
}

resource "aws_iam_role" "mac" {
  name                 = "ozymandias-mac"
  max_session_duration = 3600
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "rolesanywhere.amazonaws.com" }
      Action    = ["sts:AssumeRole", "sts:TagSession", "sts:SetSourceIdentity"]
      Condition = {
        ArnEquals    = { "aws:SourceArn" = aws_rolesanywhere_trust_anchor.mac.arn }
        StringEquals = { "aws:PrincipalTag/x509Subject/CN" = var.cn_do_mac }
      }
    }]
  })
}

# Menor privilégio: só este bucket e só os prefixos do Ozymandias. Apagar
# existe porque "tirar da nuvem" existe, mas com versioning a versão anterior
# fica 30 dias recuperável.
resource "aws_iam_role_policy" "mac" {
  name = "bucket-de-midia"
  role = aws_iam_role.mac.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "Bucket"
        Effect   = "Allow"
        Action   = ["s3:ListBucket", "s3:ListBucketMultipartUploads"]
        Resource = aws_s3_bucket.midia.arn
      },
      {
        Sid    = "Objetos"
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "s3:DeleteObject",
          "s3:AbortMultipartUpload",
          "s3:ListMultipartUploadParts",
        ]
        Resource = "${aws_s3_bucket.midia.arn}/bibliotecas/*"
      },
      {
        # O journal só cresce: o Mac escreve, nunca lê nem apaga.
        Sid      = "Journal"
        Effect   = "Allow"
        Action   = ["s3:PutObject"]
        Resource = "${aws_s3_bucket.midia.arn}/eventos/mac/*"
      },
      {
        # Derivados do worker: o Mac só lê (assina URLs para o player).
        Sid      = "Derivados"
        Effect   = "Allow"
        Action   = ["s3:GetObject"]
        Resource = "${aws_s3_bucket.midia.arn}/derivados/*"
      },
      {
        Sid      = "PedirJobs"
        Effect   = "Allow"
        Action   = ["sqs:SendMessage"]
        Resource = aws_sqs_queue.jobs.arn
      },
      {
        Sid      = "EventosDoCatalogo"
        Effect   = "Allow"
        Action   = ["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:ChangeMessageVisibility"]
        Resource = aws_sqs_queue.mac.arn
      },
      {
        Sid      = "ChaveDaCDN"
        Effect   = "Allow"
        Action   = ["ssm:GetParameter"]
        Resource = aws_ssm_parameter.chave_cdn.arn
      },
    ]
  })
}

resource "aws_rolesanywhere_profile" "mac" {
  name           = "ozymandias-mac"
  enabled        = true
  role_arns      = [aws_iam_role.mac.arn]
  duration_seconds = 3600
}
