# ── ElastiCache Redis ─────────────────────────────────────────────────────────
# Cache gerenciado pela AWS. Nosso app usa Redis como camada cache-first:
# antes de ir ao PostgreSQL, consulta o Redis (~1ms de latência).

variable "project"            { default = "url-shortener" }
variable "vpc_id"             {}
variable "private_subnet_ids" { type = list(string) }

# Subnet group para o ElastiCache — mesma lógica do RDS, subnets privadas.
resource "aws_elasticache_subnet_group" "main" {
  name       = "${var.project}-redis-subnet-group"
  subnet_ids = var.private_subnet_ids
}

# Security group do Redis: aceita conexões na porta 6379 apenas da VPC.
resource "aws_security_group" "redis" {
  name   = "${var.project}-redis-sg"
  vpc_id = var.vpc_id

  ingress {
    from_port   = 6379
    to_port     = 6379
    protocol    = "tcp"
    cidr_blocks = ["10.0.0.0/16"] # apenas tráfego interno da VPC
    description = "Redis acesso do ECS"
  }

  tags = { Name = "${var.project}-redis-sg" }
}

resource "aws_elasticache_cluster" "main" {
  cluster_id           = "${var.project}-redis"
  engine               = "redis"
  node_type            = "cache.t3.micro"  # menor instância disponível
  num_cache_nodes      = 1                 # single-node (suficiente para portfólio)
  parameter_group_name = "default.redis7"
  port                 = 6379

  subnet_group_name  = aws_elasticache_subnet_group.main.name
  security_group_ids = [aws_security_group.redis.id]

  tags = { Name = "${var.project}-redis" }
}

output "redis_endpoint" {
  # O endpoint no formato "host:port" é o que a app espera em REDIS_URL.
  value = "${aws_elasticache_cluster.main.cache_nodes[0].address}:${aws_elasticache_cluster.main.port}"
}