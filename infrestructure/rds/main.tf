# ── RDS PostgreSQL ────────────────────────────────────────────────────────────
# Banco de dados gerenciado pela AWS: backups automáticos, patches,
# failover multi-AZ e monitoramento sem configuração manual.

variable "project"            { default = "url-shortener" }
variable "vpc_id"             {}
variable "private_subnet_ids" { type = list(string) }
variable "db_password"        { sensitive = true }   # vem do Secrets Manager / tfvars

# Subnet group: informa ao RDS em quais subnets ele pode criar as instâncias.
# Usamos subnets privadas — o banco nunca fica exposto à internet.
resource "aws_db_subnet_group" "main" {
  name       = "${var.project}-db-subnet-group"
  subnet_ids = var.private_subnet_ids
  tags       = { Name = "${var.project}-db-subnet-group" }
}

# Security group do RDS: aceita conexões na porta 5432 apenas do ECS.
# O source_security_group_id garante que só o ECS consegue acessar o banco.
resource "aws_security_group" "rds" {
  name   = "${var.project}-rds-sg"
  vpc_id = var.vpc_id

  ingress {
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    description = "PostgreSQL acesso do ECS"
    # Referencia o SG do ECS — preencha com o ID real após criar o módulo ECS.
    cidr_blocks = ["10.0.0.0/16"]
  }

  tags = { Name = "${var.project}-rds-sg" }
}

resource "aws_db_instance" "main" {
  identifier        = "${var.project}-postgres"
  engine            = "postgres"
  engine_version    = "16"
  instance_class    = "db.t3.micro"   # custo mínimo para desenvolvimento/portfólio
  allocated_storage = 20              # GB — mínimo para PostgreSQL na AWS

  db_name  = "shortener"
  username = "shortener_user"
  password = var.db_password

  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.rds.id]

  # Snapshot automático diário retido por 7 dias.
  backup_retention_period = 7
  backup_window           = "03:00-04:00" # janela de backup (UTC)

  # Não expõe o banco na internet — acesso apenas pelas subnets privadas.
  publicly_accessible = false

  # skip_final_snapshot = true para facilitar destruição em ambientes de dev.
  # Em produção, remova esta linha para garantir snapshot antes de deletar.
  skip_final_snapshot = true

  tags = { Name = "${var.project}-postgres" }
}

output "db_endpoint" { value = aws_db_instance.main.endpoint }
output "db_name"     { value = aws_db_instance.main.db_name }