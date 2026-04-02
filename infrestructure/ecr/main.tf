# ── ECR (Elastic Container Registry) ─────────────────────────────────────────
# Repositório privado para armazenar as imagens Docker da aplicação.
# O GitHub Actions faz push aqui; o ECS faz pull daqui.

variable "project" { default = "url-shortener" }

resource "aws_ecr_repository" "app" {
  name                 = var.project
  image_tag_mutability = "MUTABLE" # permite sobrescrever tags (ex: "latest")

  # Scan automático de vulnerabilidades em cada push de imagem.
  image_scanning_configuration {
    scan_on_push = true
  }

  tags = { Name = var.project }
}

# Política de lifecycle: mantém apenas as 10 imagens mais recentes.
# Evita que o repositório cresça indefinidamente e gere custo de armazenamento.
resource "aws_ecr_lifecycle_policy" "app" {
  repository = aws_ecr_repository.app.name

  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Manter apenas as 10 imagens mais recentes"
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = 10
      }
      action = { type = "expire" }
    }]
  })
}

output "repository_url" { value = aws_ecr_repository.app.repository_url }