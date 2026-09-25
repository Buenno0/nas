# A imagem da nuvem é construída na AWS: o Mac não roda Docker. `make
# publicar-imagem` sobe o código (git archive) para este bucket e dispara o
# CodeBuild, numa máquina ARM (a mesma arquitetura do Fargate), que envia a
# imagem ao ECR.

resource "aws_s3_bucket" "build" {
  bucket = "${var.bucket}-build"
}

resource "aws_s3_bucket_public_access_block" "build" {
  bucket                  = aws_s3_bucket.build.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# Fonte de build é descartável: uma semana basta para investigar um build.
resource "aws_s3_bucket_lifecycle_configuration" "build" {
  bucket = aws_s3_bucket.build.id
  rule {
    id     = "expira"
    status = "Enabled"
    filter {}
    expiration {
      days = 7
    }
  }
}

resource "aws_cloudwatch_log_group" "build" {
  name              = "/ozymandias/build"
  retention_in_days = 14
}

resource "aws_iam_role" "build" {
  name = "ozymandias-build"
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Principal = { Service = "codebuild.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
}

resource "aws_iam_role_policy" "build" {
  name = "construir-imagem"
  role = aws_iam_role.build.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = ["s3:GetObject", "s3:GetObjectVersion"], Resource = "${aws_s3_bucket.build.arn}/fontes/*" },
      { Effect = "Allow", Action = ["ecr:GetAuthorizationToken"], Resource = "*" },
      {
        Effect = "Allow"
        Action = [
          "ecr:BatchCheckLayerAvailability", "ecr:BatchGetImage", "ecr:GetDownloadUrlForLayer",
          "ecr:InitiateLayerUpload", "ecr:UploadLayerPart", "ecr:CompleteLayerUpload", "ecr:PutImage",
        ]
        Resource = aws_ecr_repository.worker.arn
      },
      {
        Effect   = "Allow"
        Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
        Resource = "${aws_cloudwatch_log_group.build.arn}:*"
      },
    ]
  })
}

resource "aws_codebuild_project" "imagem" {
  name          = "ozymandias-imagem"
  description   = "Constrói a imagem arm64 do worker e da instância cloud"
  service_role  = aws_iam_role.build.arn
  build_timeout = 30

  artifacts {
    type = "NO_ARTIFACTS"
  }

  environment {
    type         = "ARM_CONTAINER"
    compute_type = "BUILD_GENERAL1_SMALL"
    image        = "aws/codebuild/amazonlinux-aarch64-standard:3.0"
    # O daemon do Docker só roda em modo privilegiado. Ele vive neste
    # container efêmero da AWS, nunca no Mac.
    privileged_mode = true
    environment_variable {
      name  = "ECR_URL"
      value = aws_ecr_repository.worker.repository_url
    }
    environment_variable {
      name  = "VERSAO"
      value = "dev"
    }
  }

  source {
    type     = "S3"
    location = "${aws_s3_bucket.build.bucket}/fontes/vazio.zip" # trocado a cada build
    buildspec = yamlencode({
      version = "0.2"
      phases = {
        pre_build = { commands = [
          "aws ecr get-login-password --region $AWS_REGION | docker login --username AWS --password-stdin $${ECR_URL%%/*}",
        ] }
        build = { commands = [
          "docker build --build-arg VERSAO=$VERSAO -t $ECR_URL:$VERSAO -t $ECR_URL:latest .",
        ] }
        post_build = { commands = [
          "docker push $ECR_URL:$VERSAO",
          "docker push $ECR_URL:latest",
        ] }
      }
    })
  }

  logs_config {
    cloudwatch_logs {
      group_name = aws_cloudwatch_log_group.build.name
    }
  }
}
