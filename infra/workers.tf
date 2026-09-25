# V3: processamento na nuvem.
#
#   S3 ObjectCreated (bibliotecas/) ─► SQS jobs ─► ECS Fargate Spot (nas worker)
#   Mac (pedido explícito) ─────────────┘              │
#                                                      ▼
#                         SNS catalogo ─► SQS no-mac ─► Mac (no híbrido)
#
# O service fica em 0 réplicas sem fila: custo zero parado.

data "aws_caller_identity" "atual" {}

# --- Imagem -----------------------------------------------------------------

resource "aws_ecr_repository" "worker" {
  name                 = "ozymandias-worker"
  image_tag_mutability = "MUTABLE"
  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "aws_ecr_lifecycle_policy" "worker" {
  repository = aws_ecr_repository.worker.name
  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "guarda só as 5 imagens mais recentes"
      selection    = { tagStatus = "any", countType = "imageCountMoreThan", countNumber = 5 }
      action       = { type = "expire" }
    }]
  })
}

# --- Filas e tópico ---------------------------------------------------------

resource "aws_sqs_queue" "jobs_dlq" {
  name                      = "ozymandias-jobs-dlq"
  message_retention_seconds = 1209600 # 14 dias para investigar
}

resource "aws_sqs_queue" "jobs" {
  name = "ozymandias-jobs"
  # O worker estende a visibilidade enquanto trabalha; 15 min é a rede de
  # segurança se ele morrer sem conseguir (Spot interrompido).
  visibility_timeout_seconds = 900
  receive_wait_time_seconds  = 20
  message_retention_seconds  = 345600
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.jobs_dlq.arn
    maxReceiveCount     = 3
  })
}

resource "aws_sqs_queue_policy" "jobs" {
  queue_url = aws_sqs_queue.jobs.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "S3Notifica"
      Effect    = "Allow"
      Principal = { Service = "s3.amazonaws.com" }
      Action    = "sqs:SendMessage"
      Resource  = aws_sqs_queue.jobs.arn
      Condition = {
        ArnEquals    = { "aws:SourceArn" = aws_s3_bucket.midia.arn }
        StringEquals = { "aws:SourceAccount" = data.aws_caller_identity.atual.account_id }
      }
    }]
  })
}

resource "aws_s3_bucket_notification" "jobs" {
  bucket = aws_s3_bucket.midia.id
  queue {
    queue_arn     = aws_sqs_queue.jobs.arn
    events        = ["s3:ObjectCreated:*"]
    filter_prefix = "bibliotecas/"
  }
  depends_on = [aws_sqs_queue_policy.jobs]
}

resource "aws_sns_topic" "catalogo" {
  name = "ozymandias-catalogo"
}

resource "aws_sqs_queue" "mac_dlq" {
  name                      = "ozymandias-no-mac-dlq"
  message_retention_seconds = 1209600
}

# O Mac pode passar dias dormindo ou no modo local: os eventos esperam 14 dias.
resource "aws_sqs_queue" "mac" {
  name                      = "ozymandias-no-mac"
  message_retention_seconds = 1209600
  receive_wait_time_seconds = 20
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.mac_dlq.arn
    maxReceiveCount     = 5
  })
}

resource "aws_sqs_queue_policy" "mac" {
  queue_url = aws_sqs_queue.mac.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "sns.amazonaws.com" }
      Action    = "sqs:SendMessage"
      Resource  = aws_sqs_queue.mac.arn
      Condition = { ArnEquals = { "aws:SourceArn" = aws_sns_topic.catalogo.arn } }
    }]
  })
}

# Cada nó descarta os próprios eventos pelo filtro da assinatura.
resource "aws_sns_topic_subscription" "mac" {
  topic_arn            = aws_sns_topic.catalogo.arn
  protocol             = "sqs"
  endpoint             = aws_sqs_queue.mac.arn
  raw_message_delivery = true
  filter_policy        = jsonencode({ origem = ["worker", "nuvem"] })
}

# --- ECS --------------------------------------------------------------------

resource "aws_ecs_cluster" "ozymandias" {
  name = "ozymandias"
  # RunningTaskCount do alarme da instância cloud vem daqui.
  setting {
    name  = "containerInsights"
    value = "enabled"
  }
}

resource "aws_ecs_cluster_capacity_providers" "spot" {
  cluster_name       = aws_ecs_cluster.ozymandias.name
  capacity_providers = ["FARGATE_SPOT", "FARGATE"]
  default_capacity_provider_strategy {
    capacity_provider = "FARGATE_SPOT"
    weight            = 1
  }
}

resource "aws_cloudwatch_log_group" "worker" {
  name              = "/ozymandias/worker"
  retention_in_days = 14
}

resource "aws_iam_role" "execucao" {
  name = "ozymandias-worker-execucao"
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Principal = { Service = "ecs-tasks.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
}

resource "aws_iam_role_policy_attachment" "execucao" {
  role       = aws_iam_role.execucao.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# O worker lê o original, escreve derivados, consome jobs e publica no
# catálogo. Não apaga nada do bucket.
resource "aws_iam_role" "worker" {
  name = "ozymandias-worker"
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Principal = { Service = "ecs-tasks.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
}

resource "aws_iam_role_policy" "worker" {
  name = "processar"
  role = aws_iam_role.worker.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = ["s3:GetObject"], Resource = "${aws_s3_bucket.midia.arn}/bibliotecas/*" },
      { Effect = "Allow", Action = ["s3:ListBucket"], Resource = aws_s3_bucket.midia.arn },
      {
        Effect   = "Allow"
        Action   = ["s3:GetObject", "s3:PutObject", "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts"]
        Resource = "${aws_s3_bucket.midia.arn}/derivados/*"
      },
      {
        Effect   = "Allow"
        Action   = ["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:ChangeMessageVisibility", "sqs:GetQueueAttributes"]
        Resource = aws_sqs_queue.jobs.arn
      },
      { Effect = "Allow", Action = ["sns:Publish"], Resource = aws_sns_topic.catalogo.arn },
    ]
  })
}

resource "aws_ecs_task_definition" "worker" {
  family                   = "ozymandias-worker"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 2048
  memory                   = 4096
  execution_role_arn       = aws_iam_role.execucao.arn
  task_role_arn            = aws_iam_role.worker.arn

  # Graviton: a mesma arquitetura do M4, e mais barato que x86.
  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "ARM64"
  }
  # O original é baixado inteiro; 100 GiB cobre um remux 4K com folga.
  ephemeral_storage {
    size_in_gib = 100
  }

  container_definitions = jsonencode([{
    name      = "worker"
    image     = "${aws_ecr_repository.worker.repository_url}:${var.imagem_tag}"
    essential = true
    command   = ["nas", "worker"]
    environment = [
      { name = "NAS_BUCKET", value = aws_s3_bucket.midia.bucket },
      { name = "NAS_REGIAO", value = var.regiao },
      { name = "NAS_FILA_JOBS", value = aws_sqs_queue.jobs.url },
      { name = "NAS_TOPICO_CATALOGO", value = aws_sns_topic.catalogo.arn },
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.worker.name
        awslogs-region        = var.regiao
        awslogs-stream-prefix = "worker"
      }
    }
  }])
}

# Rede: VPC padrão, sub-redes públicas com IP público. Sem NAT Gateway, que
# custaria ~US$ 32/mês só por existir. Nenhuma porta de entrada é aberta.
data "aws_vpc" "padrao" {
  default = true
}

data "aws_subnets" "padrao" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.padrao.id]
  }
}

resource "aws_security_group" "worker" {
  name        = "ozymandias-worker"
  description = "so saida"
  vpc_id      = data.aws_vpc.padrao.id
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_ecs_service" "worker" {
  name            = "ozymandias-worker"
  cluster         = aws_ecs_cluster.ozymandias.id
  task_definition = aws_ecs_task_definition.worker.arn
  desired_count   = 0

  capacity_provider_strategy {
    capacity_provider = "FARGATE_SPOT"
    weight            = 1
  }
  network_configuration {
    subnets          = data.aws_subnets.padrao.ids
    security_groups  = [aws_security_group.worker.id]
    assign_public_ip = true
  }
  # O autoscaling mexe no desired_count; o OpenTofu não deve desfazer.
  lifecycle {
    ignore_changes = [desired_count]
  }
}

# --- Escala 0..N pela profundidade da fila ----------------------------------

resource "aws_appautoscaling_target" "worker" {
  service_namespace  = "ecs"
  resource_id        = "service/${aws_ecs_cluster.ozymandias.name}/${aws_ecs_service.worker.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  min_capacity       = 0
  max_capacity       = var.workers_max
}

resource "aws_appautoscaling_policy" "worker" {
  name               = "ozymandias-fila"
  service_namespace  = aws_appautoscaling_target.worker.service_namespace
  resource_id        = aws_appautoscaling_target.worker.resource_id
  scalable_dimension = aws_appautoscaling_target.worker.scalable_dimension
  policy_type        = "StepScaling"

  step_scaling_policy_configuration {
    adjustment_type         = "ExactCapacity"
    cooldown                = 120
    metric_aggregation_type = "Maximum"
    # Mensagens visíveis + em voo → réplicas: 0 → 0, 1–4 → 1, 5–19 → 2, 20+ → max.
    step_adjustment {
      metric_interval_upper_bound = 1
      scaling_adjustment          = 0
    }
    step_adjustment {
      metric_interval_lower_bound = 1
      metric_interval_upper_bound = 5
      scaling_adjustment          = 1
    }
    step_adjustment {
      metric_interval_lower_bound = 5
      metric_interval_upper_bound = 20
      scaling_adjustment          = min(2, var.workers_max)
    }
    step_adjustment {
      metric_interval_lower_bound = 20
      scaling_adjustment          = var.workers_max
    }
  }
}

# Um alarme sempre em ALARM (limite 0, >=) aciona a policy a cada minuto com
# o valor atual; os degraus acima escolhem a capacidade exata, inclusive zero.
resource "aws_cloudwatch_metric_alarm" "fila" {
  alarm_name          = "ozymandias-fila-de-jobs"
  comparison_operator = "GreaterThanOrEqualToThreshold"
  threshold           = 0
  evaluation_periods  = 1
  alarm_actions       = [aws_appautoscaling_policy.worker.arn]

  metric_query {
    id          = "total"
    expression  = "visiveis + emvoo"
    label       = "jobs pendentes"
    return_data = true
  }
  metric_query {
    id = "visiveis"
    metric {
      namespace   = "AWS/SQS"
      metric_name = "ApproximateNumberOfMessagesVisible"
      dimensions  = { QueueName = aws_sqs_queue.jobs.name }
      period      = 60
      stat        = "Maximum"
    }
  }
  metric_query {
    id = "emvoo"
    metric {
      namespace   = "AWS/SQS"
      metric_name = "ApproximateNumberOfMessagesNotVisible"
      dimensions  = { QueueName = aws_sqs_queue.jobs.name }
      period      = 60
      stat        = "Maximum"
    }
  }
}

# DLQ com mensagem = job que falhou 3 vezes. Avisa por e-mail.
resource "aws_sns_topic" "alertas" {
  name = "ozymandias-alertas"
}

resource "aws_sns_topic_subscription" "alertas" {
  topic_arn = aws_sns_topic.alertas.arn
  protocol  = "email"
  endpoint  = var.email_alerta
}

resource "aws_cloudwatch_metric_alarm" "dlq" {
  alarm_name          = "ozymandias-jobs-na-dlq"
  namespace           = "AWS/SQS"
  metric_name         = "ApproximateNumberOfMessagesVisible"
  dimensions          = { QueueName = aws_sqs_queue.jobs_dlq.name }
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 1
  comparison_operator = "GreaterThanThreshold"
  threshold           = 0
  alarm_actions       = [aws_sns_topic.alertas.arn]
}
