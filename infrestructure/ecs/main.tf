# ── ECS Fargate ───────────────────────────────────────────────────────────────
# Orquestra os containers da aplicação sem precisar gerenciar servidores EC2.
# O Fargate é serverless: você define CPU/memória e a AWS cuida do resto.

variable "project"           { default = "url-shortener" }
variable "vpc_id"            {}
variable "public_subnet_ids" { type = list(string) }
variable "private_subnet_ids"{ type = list(string) }
variable "ecr_image_url"     {}    # URL da imagem no ECR
variable "database_url"      { sensitive = true }
variable "redis_url"         {}
variable "aws_region"        { default = "us-east-1" }

# ── Cluster ECS ───────────────────────────────────────────────────────────────
# O cluster é o agrupamento lógico dos serviços e tasks.
resource "aws_ecs_cluster" "main" {
  name = "${var.project}-cluster"

  # Container Insights: métricas detalhadas de CPU/memória por container no CloudWatch.
  setting {
    name  = "containerInsights"
    value = "enabled"
  }
}

# ── IAM Role para a Task ──────────────────────────────────────────────────────
# As tasks ECS precisam de uma role para: puxar imagens do ECR,
# escrever logs no CloudWatch e acessar o Secrets Manager.
resource "aws_iam_role" "ecs_task_execution" {
  name = "${var.project}-ecs-task-execution-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })
}

# Política gerenciada pela AWS com as permissões mínimas para execução de tasks.
resource "aws_iam_role_policy_attachment" "ecs_task_execution" {
  role       = aws_iam_role.ecs_task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# ── Log Group no CloudWatch ───────────────────────────────────────────────────
# Todos os logs do container (stdout/stderr) vão para cá.
# Retenção de 7 dias para economizar custo em ambiente de portfólio.
resource "aws_cloudwatch_log_group" "app" {
  name              = "/ecs/${var.project}"
  retention_in_days = 7
}

# ── Task Definition ───────────────────────────────────────────────────────────
# Define como o container deve ser executado: imagem, CPU, memória, env vars e logs.
resource "aws_ecs_task_definition" "app" {
  family                   = var.project
  network_mode             = "awsvpc"      # cada task recebe seu próprio ENI
  requires_compatibilities = ["FARGATE"]
  cpu                      = "256"         # 0.25 vCPU — suficiente para portfólio
  memory                   = "512"         # 512MB RAM
  execution_role_arn       = aws_iam_role.ecs_task_execution.arn

  container_definitions = jsonencode([{
    name  = var.project
    image = var.ecr_image_url   # URL da imagem no ECR com a tag do commit

    # Mapeia a porta 8080 do container para o ALB.
    portMappings = [{
      containerPort = 8080
      protocol      = "tcp"
    }]

    # Variáveis de ambiente injetadas no container em runtime.
    # DATABASE_URL e REDIS_URL apontam para os recursos na VPC privada.
    environment = [
      { name = "PORT",         value = "8080" },
      { name = "DATABASE_URL", value = var.database_url },
      { name = "REDIS_URL",    value = var.redis_url },
      { name = "BASE_URL",     value = "https://seu-dominio.com" },
    ]

    # Envia todos os logs (stdout/stderr) para o CloudWatch.
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.app.name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "ecs"
      }
    }

    # Healthcheck interno do container.
    # O ECS substitui o container se o healthcheck falhar 3 vezes seguidas.
    healthCheck = {
      command     = ["CMD-SHELL", "wget -qO- http://localhost:8080/health || exit 1"]
      interval    = 30
      timeout     = 5
      retries     = 3
      startPeriod = 10  # aguarda 10s após o start antes de checar
    }
  }])
}

# ── Security Group do ECS ─────────────────────────────────────────────────────
resource "aws_security_group" "ecs" {
  name   = "${var.project}-ecs-sg"
  vpc_id = var.vpc_id

  # Aceita tráfego na porta 8080 apenas do ALB.
  ingress {
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
    description     = "Tráfego do ALB"
  }

  # Permite saída para qualquer destino: necessário para acessar RDS, Redis e ECR.
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${var.project}-ecs-sg" }
}

# ── ALB (Application Load Balancer) ──────────────────────────────────────────
resource "aws_security_group" "alb" {
  name   = "${var.project}-alb-sg"
  vpc_id = var.vpc_id

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "HTTP da internet"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${var.project}-alb-sg" }
}

resource "aws_lb" "main" {
  name               = "${var.project}-alb"
  internal           = false              # público — recebe tráfego da internet
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = var.public_subnet_ids
  tags               = { Name = "${var.project}-alb" }
}

resource "aws_lb_target_group" "app" {
  name        = "${var.project}-tg"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip"   # obrigatório para Fargate com awsvpc

  # O ALB checa /health a cada 30s para saber quais containers estão saudáveis.
  health_check {
    path                = "/health"
    interval            = 30
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.main.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app.arn
  }
}

# ── ECS Service ───────────────────────────────────────────────────────────────
# O Service garante que sempre haverá 1 task rodando.
# Se uma task morrer, o ECS sobe outra automaticamente.
resource "aws_ecs_service" "app" {
  name            = "${var.project}-service"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.app.arn
  desired_count   = 1          # número de tasks simultâneas
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = var.private_subnet_ids
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = false   # tasks ficam em subnets privadas
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.app.arn
    container_name   = var.project
    container_port   = 8080
  }

  # Durante o deploy, o ECS sobe as novas tasks antes de derrubar as antigas.
  # Isso garante zero downtime no rolling update.
  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200

  depends_on = [aws_lb_listener.http]
}

output "alb_dns" { value = aws_lb.main.dns_name }