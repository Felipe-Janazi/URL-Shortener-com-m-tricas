# ── Estágio 1: Compilação ──────────────────────────────────────────────────────
# Usa imagem com Go completo só para compilar — não vai para produção.
FROM golang:1.23-alpine AS builder
WORKDIR /app

# Copia os arquivos de dependências primeiro, separado do código.
# Motivo: o Docker cacheia cada layer. Se go.mod não mudou,
# 'go mod download' não roda de novo — build muito mais rápido.
COPY go.mod go.sum ./
RUN go mod download

# Agora copia o código e compila.
COPY . .
RUN CGO_ENABLED=0 \       
# binário estático, sem dependências de .so
    GOOS=linux \           
    # compila para Linux (necessário se você usa macOS/Windows)
    go build \
    -ldflags="-s -w" \     
    # -s: sem tabela de símbolos, -w: sem DWARF → binário menor
    -o api ./cmd/api

# ── Estágio 2: Runtime ─────────────────────────────────────────────────────────
# distroless/static: sem shell, sem apt, sem nada além do necessário.
# Superfície de ataque mínima e imagem final com ~10MB em vez de ~800MB.
FROM gcr.io/distroless/static-debian12

# Copia apenas o binário compilado do estágio anterior.
COPY --from=builder /app/api /api

EXPOSE 8080

# Roda o binário diretamente, sem shell wrapper.
ENTRYPOINT ["/api"]