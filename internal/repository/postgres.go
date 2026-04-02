package repository

import (
    "context"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/Felipe-Janazi/url-shortener/internal/model"
)

type PostgresRepo struct {
    db *pgxpool.Pool // pool de conexões (thread-safe, reutilizável entre requests)
}

// NewPostgres cria o pool e verifica a conexão na inicialização.
// Se o banco estiver inacessível, a aplicação falha imediatamente (fail-fast).
func NewPostgres(connStr string) *PostgresRepo {
    // ParseConfig permite ajustar os parâmetros do pool antes de conectar.
    cfg, err := pgxpool.ParseConfig(connStr)
    if err != nil {
        panic(fmt.Sprintf("erro ao parsear DATABASE_URL: %v", err))
    }

    // Configurações do pool de conexões.
    cfg.MaxConns = 20               // máximo de 20 conexões abertas simultâneas
    cfg.MinConns = 5                // mantém 5 conexões sempre ativas (warm pool)
    cfg.MaxConnLifetime = time.Hour // recria conexões velhas para evitar leaks

    pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
    if err != nil {
        panic(fmt.Sprintf("erro ao criar pool Postgres: %v", err))
    }

    // Ping garante que a conexão está funcional antes de aceitar requisições.
    if err := pool.Ping(context.Background()); err != nil {
        panic(fmt.Sprintf("postgres não acessível: %v", err))
    }

    return &PostgresRepo{db: pool}
}

// Save insere um novo registro de URL no banco.
// ON CONFLICT DO NOTHING tolera colisão de código (extremamente raro, mas possível).
func (r *PostgresRepo) Save(ctx context.Context, u model.URL) error {
    q := `
        INSERT INTO urls (code, original_url, created_at, expires_at)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (code) DO NOTHING
    `
    _, err := r.db.Exec(ctx, q, u.Code, u.OriginalURL, u.CreatedAt, u.ExpiresAt)
    return err
}

// FindByCode busca uma URL pelo código curto.
// A cláusula expires_at > NOW() rejeita URLs expiradas direto no banco.
func (r *PostgresRepo) FindByCode(ctx context.Context, code string) (*model.URL, error) {
    q := `
        SELECT id, code, original_url, clicks, created_at, expires_at
        FROM urls
        WHERE code = $1 AND expires_at > NOW()
    `
    var u model.URL
    err := r.db.QueryRow(ctx, q, code).Scan(
        &u.ID, &u.Code, &u.OriginalURL, &u.Clicks, &u.CreatedAt, &u.ExpiresAt,
    )
    if err != nil {
        return nil, fmt.Errorf("código %q não encontrado: %w", code, err)
    }
    return &u, nil
}

// IncrementClicks atualiza o contador de acessos.
// É sempre chamado em goroutine background para não bloquear o redirecionamento.
func (r *PostgresRepo) IncrementClicks(ctx context.Context, code string) error {
    _, err := r.db.Exec(ctx,
        "UPDATE urls SET clicks = clicks + 1 WHERE code = $1", code,
    )
    return err
}