# 🔗 url-shortener

API de encurtamento de URLs construída em Go, com cache em Redis, persistência em PostgreSQL e deploy automatizado na AWS via ECS Fargate.

> Projeto desenvolvido para demonstrar boas práticas de backend e DevOps: arquitetura em camadas, cache-first, graceful shutdown, pipeline CI/CD com GitHub Actions e infraestrutura como código com Terraform.

---

## ✨ Funcionalidades

- Encurta qualquer URL em um código de 7 caracteres (`aB3xK7q`)
- Redireciona com **latência ~1ms** via cache Redis (cache-first)
- Contador de cliques por URL atualizado em background
- URLs com expiração configurável (padrão: 30 dias)
- Healthcheck endpoint para o ALB/ECS
- Graceful shutdown — termina requisições em andamento antes de parar

---

## 🏗️ Arquitetura
```
Client → ALB → ECS Fargate (Go API)
                    ├── Redis (ElastiCache) ← cache-first para GET /{code}
                    └── PostgreSQL (RDS)    ← fonte da verdade
```

### Fluxo de redirecionamento
```
GET /{code}
  │
  ├─ Redis hit? ──→ 302 redirect  (~1ms)
  │
  └─ Redis miss ──→ PostgreSQL ──→ preenche cache ──→ 302 redirect  (~10ms)
```

### Fluxo de criação
```
POST /shorten  {"url": "https://..."}
  │
  ├─ Valida URL
  ├─ Gera código único (crypto/rand, base64url)
  ├─ Salva no PostgreSQL
  └─ Pré-aquece o cache Redis
```

---

## 🛠️ Stack

| Camada | Tecnologia |
|---|---|
| API | Go 1.22+ (net/http nativo) |
| Cache | Redis 7 (ElastiCache) |
| Banco de dados | PostgreSQL 16 (RDS) |
| Container | Docker (multi-stage, distroless) |
| Orquestração | AWS ECS Fargate |
| Registry | AWS ECR |
| Infra como código | Terraform |
| CI/CD | GitHub Actions |
| Autenticação AWS | OIDC (sem chaves estáticas) |

---

## 📁 Estrutura do projeto
```
url-shortener/
├── cmd/api/
│   └── main.go               # entrypoint: wiring + graceful shutdown
├── internal/
│   ├── config/config.go      # env vars com fail-fast
│   ├── model/url.go          # structs compartilhadas entre camadas
│   ├── handler/url.go        # camada HTTP (decode → service → encode)
│   ├── service/url.go        # regras de negócio + cache-first
│   ├── repository/
│   │   ├── postgres.go       # pool de conexões + queries
│   │   └── redis.go          # get/set com TTL
│   └── middleware/
│       ├── logger.go         # request logging estruturado
│       └── ratelimit.go      # rate limiting via Redis
├── migrations/               # SQL versionado (up + down)
├── infra/                    # Terraform: VPC, ECS, RDS, ElastiCache, ECR
├── .github/workflows/
│   ├── ci.yml                # test + build no PR
│   └── deploy.yml            # push ECR + update ECS no merge
├── Dockerfile                # multi-stage, imagem final ~10MB
├── docker-compose.yml        # ambiente local completo
└── Makefile                  # atalhos de desenvolvimento
```

---

## 🚀 Rodando localmente

### Pré-requisitos

- [Go 1.22+](https://go.dev/dl/)
- [Docker](https://docs.docker.com/get-docker/) + Docker Compose
- [golang-migrate](https://github.com/golang-migrate/migrate) (`go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest`)

### Setup
```bash
# 1. Clone o repositório
git clone https://github.com/seu-user/url-shortener
cd url-shortener

# 2. Instale as dependências
go mod download

# 3. Sobe PostgreSQL + Redis e inicia a API
make run

# 4. Em outro terminal, aplica as migrations
make migrate
```

A API estará disponível em `http://localhost:8080`.

---

## 📡 Endpoints

### `POST /shorten` — Encurtar uma URL
```bash
curl -X POST http://localhost:8080/shorten \
  -H "Content-Type: application/json" \
  -d '{"url": "https://github.com/seu-user"}'
```

**Resposta `201 Created`:**
```json
{
  "short_url": "localhost:8080/aB3xK7q",
  "code": "aB3xK7q",
  "original_url": "https://github.com/seu-user",
  "expires_at": "2025-06-01"
}
```

---

### `GET /{code}` — Redirecionar
```bash
curl -L http://localhost:8080/aB3xK7q
# → 302 redirect para https://github.com/seu-user
```

---

### `GET /health` — Healthcheck
```bash
curl http://localhost:8080/health
# → {"status": "ok"}
```

---

## 🧪 Testes
```bash
# Todos os testes com race detector
make test

# Com cobertura
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

---

## 🗄️ Banco de dados

### Schema principal
```sql
CREATE TABLE urls (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    code         VARCHAR(20) UNIQUE NOT NULL,
    original_url TEXT        NOT NULL,
    clicks       INTEGER     NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_urls_code ON urls(code);
```

### Migrations
```bash
make migrate           # aplica todas as pendentes
make migrate-down      # reverte a última
```

---

## ☁️ Deploy na AWS

### Infraestrutura (Terraform)
```bash
cd infra
terraform init
terraform plan
terraform apply
```

Recursos provisionados: VPC com subnets públicas/privadas, ECS Cluster + Service + Task Definition, RDS PostgreSQL, ElastiCache Redis, ECR, ALB, IAM roles, Security Groups.

### CI/CD (GitHub Actions)

| Evento | Pipeline |
|---|---|
| Pull Request → `main` | Lint + Testes + Build da imagem |
| Push → `main` | Build + Push ECR + Update ECS (rolling deploy) |

A autenticação com a AWS usa **OIDC** — sem `AWS_ACCESS_KEY_ID` salva no repositório. Configure os secrets necessários:
```
AWS_ROLE_ARN   → ARN da role com permissão de deploy
```

---

## ⚙️ Variáveis de ambiente

| Variável | Obrigatória | Padrão | Descrição |
|---|---|---|---|
| `DATABASE_URL` | ✅ | — | Connection string do PostgreSQL |
| `REDIS_URL` | ✅ | — | `host:port` do Redis |
| `PORT` | ❌ | `8080` | Porta HTTP |
| `BASE_URL` | ❌ | `http://localhost:8080` | Domínio base para as URLs curtas |

---

## 🔧 Makefile
```bash
make run           # sobe dependências e inicia a API
make test          # roda os testes com race detector
make migrate       # aplica migrations pendentes
make migrate-down  # reverte a última migration
make build         # compila o binário para produção
make clean         # remove artefatos e derruba containers
```

---

## 🧠 Decisões técnicas

**Por que cache-first com Redis?**
O fluxo de redirecionamento é read-heavy. Um hit no Redis (~1ms) evita completamente a query ao PostgreSQL (~10ms), reduzindo latência e carga no banco.

**Por que `crypto/rand` para gerar códigos?**
`math/rand` é previsível e pode ser manipulado. `crypto/rand` usa o CSPRNG do SO — os códigos não são adivinháveis nem sequenciais.

**Por que `302` em vez de `301` no redirect?**
`301` é cacheado permanentemente pelo browser. Com `302`, se a URL expirar ou mudar, o próximo acesso sempre consulta a API em vez de usar o cache local.

**Por que imagem distroless?**
Sem shell, sem package manager, sem ferramentas desnecessárias. Superfície de ataque mínima e imagem final com ~10MB em vez de ~800MB.

**Por que OIDC em vez de chaves AWS no GitHub?**
Chaves estáticas podem vazar. O OIDC emite tokens temporários e assina a identidade do workflow — sem segredo para rotacionar ou vazar.

---

## 📄 Licença

MIT