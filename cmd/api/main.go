package main

import (
    "context"
    "log/slog"        // logger estruturado nativo do Go 1.21+
    "net/http"
    "os"
    "os/signal"       // captura sinais do SO (SIGINT, SIGTERM)
    "syscall"
    "time"

    "github.com/Felipe-Janazi/url-shortener/internal/config"
    "github.com/Felipe-Janazi/url-shortener/internal/handler"
    "github.com/Felipe-Janazi/url-shortener/internal/repository"
    "github.com/Felipe-Janazi/url-shortener/internal/service"
)

func main() {
    // PASSO 1: Carrega todas as variáveis de ambiente e valida.
    // Se faltar DATABASE_URL ou REDIS_URL, a app aborta aqui e não sobe.
    cfg := config.Load()

    // PASSO 2: Cria os repositórios — camadas de acesso a dados.
    // pgRepo fala com o PostgreSQL (escrita + leitura persistente).
    // redisRepo fala com o Redis (cache de redirecionamentos).
    pgRepo    := repository.NewPostgres(cfg.DatabaseURL)
    redisRepo := repository.NewRedis(cfg.RedisURL)

    // PASSO 3: Injeta os repositórios no serviço (regras de negócio).
    // O service não sabe se está falando com Postgres ou um mock — só usa a interface.
    svc := service.NewURLService(pgRepo, redisRepo)

    // PASSO 4: Injeta o serviço nos handlers (camada HTTP).
    h := handler.NewURLHandler(svc)

    // PASSO 5: Registra as rotas usando o roteador padrão do Go 1.22+.
    // A sintaxe 'METHOD /path' faz matching exato por método HTTP.
    mux := http.NewServeMux()
    mux.HandleFunc("POST /shorten", h.Shorten)   // cria URL curta
    mux.HandleFunc("GET /{code}",   h.Redirect)  // redireciona
    mux.HandleFunc("GET /health",   h.Health)    // healthcheck para o ECS

    srv := &http.Server{
        Addr:         ":" + cfg.Port,
        Handler:      mux,
        ReadTimeout:  5 * time.Second,  // evita conexões presas lendo body
        WriteTimeout: 10 * time.Second, // evita conexões presas escrevendo
    }

    // PASSO 6: Sobe o servidor em uma goroutine separada.
    // Isso libera a goroutine principal para esperar pelo sinal de shutdown.
    go func() {
        slog.Info("server starting", "port", cfg.Port)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            slog.Error("server error", "err", err)
            os.Exit(1)
        }
    }()

    // PASSO 7: Graceful shutdown.
    // Aguarda SIGINT (Ctrl+C) ou SIGTERM (enviado pelo ECS ao fazer deploy).
    // Quando chega, dá 10 segundos para as requisições em andamento finalizarem.
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit // bloqueia aqui até receber o sinal

    slog.Info("shutting down server...")
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    srv.Shutdown(ctx)
}