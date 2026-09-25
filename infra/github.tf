# Deploy contínuo: a cada push na master, o GitHub Actions publica a imagem e
# troca a instância cloud. Sem chave fixa: o GitHub apresenta um token OIDC
# assinado, e a AWS só aceita o desta repo, deste branch.

variable "repo_github" {
  description = "Repositório que pode publicar (dono/nome). Vazio desliga o deploy pelo GitHub."
  type        = string
  default     = "Buenno0/nas"
}

variable "repo_github_imutavel" {
  description = "Prefixo do sub no formato imutável do GitHub (dono@id/repo@id). Veja em /repos/<dono>/<repo>/actions/oidc/customization/sub."
  type        = string
  default     = "repo:Buenno0@160802402/nas@1334700693"
}

resource "aws_iam_openid_connect_provider" "github" {
  count          = var.repo_github == "" ? 0 : 1
  url            = "https://token.actions.githubusercontent.com"
  client_id_list = ["sts.amazonaws.com"]
  # A AWS valida o GitHub pela CA dela; a impressão só é exigida pela API.
  thumbprint_list = ["6938fd4d98bab03faadb97b34396831e3780aea1"]
}

resource "aws_iam_role" "github" {
  count = var.repo_github == "" ? 0 : 1
  name  = "ozymandias-github"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Federated = aws_iam_openid_connect_provider.github[0].arn }
      Action    = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "token.actions.githubusercontent.com:aud" = "sts.amazonaws.com"
          # Só push na master: PR de fork, outro branch ou tag não entram. O
          # formato imutável (com os IDs numéricos) não muda se a repo for
          # renomeada ou recriada com o mesmo nome por outra pessoa.
          "token.actions.githubusercontent.com:sub" = compact([
            "repo:${var.repo_github}:ref:refs/heads/master",
            var.repo_github_imutavel == "" ? "" : "${var.repo_github_imutavel}:ref:refs/heads/master",
          ])
        }
      }
    }]
  })
  max_session_duration = 3600
}

resource "aws_iam_role_policy" "github" {
  count = var.repo_github == "" ? 0 : 1
  name  = "publicar"
  role  = aws_iam_role.github[0].id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Sid = "Fonte", Effect = "Allow", Action = ["s3:PutObject"], Resource = "${aws_s3_bucket.build.arn}/fontes/*" },
      {
        Sid      = "Build"
        Effect   = "Allow"
        Action   = ["codebuild:StartBuild", "codebuild:BatchGetBuilds"]
        Resource = aws_codebuild_project.imagem.arn
      },
      {
        Sid      = "TrocarInstancia"
        Effect   = "Allow"
        Action   = ["ecs:UpdateService", "ecs:DescribeServices"]
        Resource = "arn:aws:ecs:${var.regiao}:${data.aws_caller_identity.atual.account_id}:service/${aws_ecs_cluster.ozymandias.name}/ozymandias-nuvem"
      },
    ]
  })
}

output "github_role_arn" {
  description = "Vai na variável AWS_ROLE_ARN do repositório no GitHub."
  value       = var.repo_github == "" ? "" : aws_iam_role.github[0].arn
}
