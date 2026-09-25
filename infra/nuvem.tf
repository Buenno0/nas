# V4: instância cloud do Ozymandias (opção A do plano).
#
# Uma task Fargate Spot de 0,5 vCPU com dois containers no mesmo localhost:
#   - ozymandias: nas serve --nuvem sob o Litestream (SQLite → S3);
#   - tailscale: HTTPS em ozymandias-nuvem.<tailnet>.ts.net, proxy para
#     127.0.0.1:8787, Funnel opcional. Nenhuma porta de entrada na AWS.

resource "aws_ssm_parameter" "tailscale" {
  count = var.tailscale_authkey == "" ? 0 : 1
  name  = "/ozymandias/tailscale/authkey"
  type  = "SecureString"
  value = var.tailscale_authkey
}

resource "aws_sqs_queue" "nuvem_dlq" {
  name                      = "ozymandias-no-cloud-dlq"
  message_retention_seconds = 1209600
}

resource "aws_sqs_queue" "nuvem" {
  name                      = "ozymandias-no-cloud"
  message_retention_seconds = 1209600
  receive_wait_time_seconds = 20
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.nuvem_dlq.arn
    maxReceiveCount     = 5
  })
}

resource "aws_sqs_queue_policy" "nuvem" {
  queue_url = aws_sqs_queue.nuvem.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "sns.amazonaws.com" }
      Action    = "sqs:SendMessage"
      Resource  = aws_sqs_queue.nuvem.arn
      Condition = { ArnEquals = { "aws:SourceArn" = aws_sns_topic.catalogo.arn } }
    }]
  })
}

resource "aws_sns_topic_subscription" "nuvem" {
  topic_arn            = aws_sns_topic.catalogo.arn
  protocol             = "sqs"
  endpoint             = aws_sqs_queue.nuvem.arn
  raw_message_delivery = true
  filter_policy        = jsonencode({ origem = ["worker", "mac"] })
}

resource "aws_iam_role" "nuvem" {
  name = "ozymandias-nuvem"
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Principal = { Service = "ecs-tasks.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
}

resource "aws_iam_role_policy" "nuvem" {
  name = "instancia-cloud"
  role = aws_iam_role.nuvem.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        # Acordar o worker quando um arquivo chega, sem esperar o autoscaling.
        Sid      = "AcordarWorker"
        Effect   = "Allow"
        Action   = ["ecs:DescribeServices", "ecs:UpdateService"]
        Resource = "arn:aws:ecs:${var.regiao}:${data.aws_caller_identity.atual.account_id}:service/${aws_ecs_cluster.ozymandias.name}/ozymandias-worker"
      },
      {
        # Canal do ECS Exec (o agente do SSM que o Fargate injeta).
        Sid      = "ECSExec"
        Effect   = "Allow"
        Action   = ["ssmmessages:CreateControlChannel", "ssmmessages:CreateDataChannel", "ssmmessages:OpenControlChannel", "ssmmessages:OpenDataChannel"]
        Resource = "*"
      },
      { Effect = "Allow", Action = ["s3:ListBucket", "s3:ListBucketMultipartUploads"], Resource = aws_s3_bucket.midia.arn },
      {
        # Uploads com o Mac dormindo e leitura do acervo.
        Effect   = "Allow"
        Action   = ["s3:GetObject", "s3:PutObject", "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts"]
        Resource = "${aws_s3_bucket.midia.arn}/bibliotecas/*"
      },
      {
        Effect   = "Allow"
        Action   = ["s3:GetObject"]
        Resource = ["${aws_s3_bucket.midia.arn}/derivados/*", "${aws_s3_bucket.midia.arn}/catalogo/*"]
      },
      { Effect = "Allow", Action = ["s3:PutObject"], Resource = "${aws_s3_bucket.midia.arn}/eventos/nuvem/*" },
      {
        # Litestream: réplica do SQLite (inclui apagar segmentos antigos).
        Effect   = "Allow"
        Action   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
        Resource = "${aws_s3_bucket.midia.arn}/estado/nuvem/*"
      },
      { Effect = "Allow", Action = ["sqs:SendMessage"], Resource = aws_sqs_queue.jobs.arn },
      {
        Effect   = "Allow"
        Action   = ["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:ChangeMessageVisibility"]
        Resource = aws_sqs_queue.nuvem.arn
      },
      { Effect = "Allow", Action = ["sns:Publish"], Resource = aws_sns_topic.catalogo.arn },
      { Effect = "Allow", Action = ["ssm:GetParameter"], Resource = aws_ssm_parameter.chave_cdn.arn },
    ]
  })
}

# A execução precisa ler a auth key do Tailscale para injetá-la no sidecar.
resource "aws_ssm_parameter" "tmdb" {
  count = var.tailscale_authkey != "" && var.tmdb_key != "" ? 1 : 0
  name  = "/ozymandias/tmdb/chave"
  type  = "SecureString"
  value = var.tmdb_key
}

resource "aws_iam_role_policy" "execucao_segredos" {
  count = var.tailscale_authkey == "" ? 0 : 1
  name  = "segredos-da-instancia"
  role  = aws_iam_role.execucao.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["ssm:GetParameters"]
      Resource = concat([aws_ssm_parameter.tailscale[0].arn], aws_ssm_parameter.tmdb[*].arn)
    }]
  })
}

resource "aws_cloudwatch_log_group" "nuvem" {
  name              = "/ozymandias/nuvem"
  retention_in_days = 14
}

locals {
  # Configuração do tailscale serve: HTTPS no nome da máquina, proxy para o
  # Ozymandias no loopback; Funnel só se pedido. ${TS_CERT_DOMAIN} é trocado
  # pelo próprio containerboot.
  serve_json = jsonencode({
    TCP         = { "443" = { HTTPS = true } }
    Web         = { "$${TS_CERT_DOMAIN}:443" = { Handlers = { "/" = { Proxy = "http://127.0.0.1:8787" } } } }
    AllowFunnel = { "$${TS_CERT_DOMAIN}:443" = var.funnel }
  })
  log = { logDriver = "awslogs", options = {
    awslogs-group = aws_cloudwatch_log_group.nuvem.name, awslogs-region = var.regiao, awslogs-stream-prefix = "nuvem"
  } }
}

resource "aws_ecs_task_definition" "nuvem" {
  count                    = var.tailscale_authkey == "" ? 0 : 1
  family                   = "ozymandias-nuvem"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 512
  memory                   = 1024
  execution_role_arn       = aws_iam_role.execucao.arn
  task_role_arn            = aws_iam_role.nuvem.arn
  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "ARM64"
  }

  volume {
    name = "tailscale"
    efs_volume_configuration {
      file_system_id     = aws_efs_file_system.tailscale[0].id
      transit_encryption = "ENABLED"
    }
  }

  container_definitions = jsonencode([
    {
      name      = "ozymandias"
      image     = "${aws_ecr_repository.worker.repository_url}:${var.imagem_tag}"
      essential = true
      command   = ["ozymandias-nuvem"]
      environment = [
        { name = "NAS_BUCKET", value = aws_s3_bucket.midia.bucket },
        { name = "NAS_REGIAO", value = var.regiao },
        { name = "NAS_FILA_JOBS", value = aws_sqs_queue.jobs.url },
        { name = "NAS_FILA_EVENTOS", value = aws_sqs_queue.nuvem.url },
        { name = "NAS_TOPICO_CATALOGO", value = aws_sns_topic.catalogo.arn },
        { name = "NAS_CDN_DOMINIO", value = aws_cloudfront_distribution.midia.domain_name },
        { name = "NAS_CDN_CHAVE_ID", value = aws_cloudfront_public_key.cdn.id },
        { name = "NAS_CDN_PARAMETRO", value = aws_ssm_parameter.chave_cdn.name },
      ]
      secrets          = [for p in aws_ssm_parameter.tmdb : { name = "NAS_TMDB_KEY", valueFrom = p.arn }]
      logConfiguration = local.log
    },
    {
      name      = "tailscale"
      image     = "tailscale/tailscale:stable"
      essential = true
      # O containerboot só aceita a configuração de serve como arquivo.
      entryPoint = ["/bin/sh", "-c"]
      command    = ["printf '%s' \"$SERVE_JSON\" > /tmp/serve.json && exec /usr/local/bin/containerboot"]
      environment = [
        { name = "TS_HOSTNAME", value = var.nome_na_tailnet },
        # No EFS: identidade e certificado sobrevivem à troca da task.
        { name = "TS_STATE_DIR", value = "/var/lib/tailscale" },
        { name = "TS_USERSPACE", value = "true" }, # Fargate não tem /dev/net/tun
        { name = "TS_SERVE_CONFIG", value = "/tmp/serve.json" },
        { name = "TS_EXTRA_ARGS", value = "--advertise-tags=tag:ozymandias" },
        { name = "SERVE_JSON", value = local.serve_json },
      ]
      secrets          = [{ name = "TS_AUTHKEY", valueFrom = aws_ssm_parameter.tailscale[0].arn }]
      mountPoints      = [{ sourceVolume = "tailscale", containerPath = "/var/lib/tailscale", readOnly = false }]
      logConfiguration = local.log
    },
  ])
}

resource "aws_security_group" "nuvem" {
  name        = "ozymandias-nuvem"
  description = "so saida: a entrada e o Tailscale, por conexao de saida"
  vpc_id      = data.aws_vpc.padrao.id
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_ecs_service" "nuvem" {
  count           = var.tailscale_authkey == "" ? 0 : 1
  name            = "ozymandias-nuvem"
  cluster         = aws_ecs_cluster.ozymandias.id
  task_definition = aws_ecs_task_definition.nuvem[0].arn
  desired_count   = 1
  # ECS Exec: terminal no container pelo console (Connect) ou por
  # `aws ecs execute-command`. Sem SSH nem porta aberta; sessões no CloudTrail.
  enable_execute_command = true
  depends_on             = [aws_efs_mount_target.tailscale]
  # Um só escritor no SQLite: nunca duas tasks ao mesmo tempo.
  deployment_minimum_healthy_percent = 0
  deployment_maximum_percent         = 100

  capacity_provider_strategy {
    capacity_provider = "FARGATE_SPOT"
    weight            = 1
  }
  network_configuration {
    subnets          = data.aws_subnets.padrao.ids
    security_groups  = [aws_security_group.nuvem.id]
    assign_public_ip = true
  }
}

resource "aws_cloudwatch_metric_alarm" "dlq_eventos" {
  for_each = {
    mac   = aws_sqs_queue.mac_dlq.name
    nuvem = aws_sqs_queue.nuvem_dlq.name
  }
  alarm_name          = "ozymandias-eventos-na-dlq-${each.key}"
  namespace           = "AWS/SQS"
  metric_name         = "ApproximateNumberOfMessagesVisible"
  dimensions          = { QueueName = each.value }
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 1
  comparison_operator = "GreaterThanThreshold"
  threshold           = 0
  alarm_actions       = [aws_sns_topic.alertas.arn]
}

# A instância cloud deveria estar sempre no ar: sem task rodando por 10 min,
# avisa (Spot interrompido e sem capacidade, imagem quebrada, auth key vencida).
resource "aws_cloudwatch_metric_alarm" "nuvem_fora" {
  count               = var.tailscale_authkey == "" ? 0 : 1
  alarm_name          = "ozymandias-instancia-cloud-fora"
  namespace           = "ECS/ContainerInsights"
  metric_name         = "RunningTaskCount"
  dimensions          = { ClusterName = aws_ecs_cluster.ozymandias.name, ServiceName = aws_ecs_service.nuvem[0].name }
  statistic           = "Minimum"
  period              = 300
  evaluation_periods  = 2
  comparison_operator = "LessThanThreshold"
  threshold           = 1
  treat_missing_data  = "breaching"
  alarm_actions       = [aws_sns_topic.alertas.arn]
}
