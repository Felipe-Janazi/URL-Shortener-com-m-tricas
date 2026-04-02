package service

import (
    "context"
    "crypto/rand"       // geração criptograficamente segura — nunca use math/rand para isso
    "encoding/base64"
    "errors"
    "fmt"
    "net/url"           // validação de URL
    "time"

    "github.com/Felipe-Janazi/url-shortener/internal/model"
)

// ErrInvalidURL é retornado quando a URL fornecida não é válida.
// Tipado para o handler conseguir diferenciar erro de cliente (400) de erro interno (500).
var ErrInvalidURL = errors.New("URL inválida")

// URLRepository define o contrato que qualquer repositório de dados deve cumprir.
// Usando interface: o service não sabe (nem precisa saber) que é Postgres por baixo.
// Isso facilita trocar o banco no futuro e escrever testes com mocks.
type URLRepository interface {
    Save(ctx context.Context, url model.URL) error
    FindByCode(ctx context.Context, code string) (*model.URL, error)
    IncrementClicks(ctx context.Context, code string) error
}

// CacheRepository define o contrato para a camada de cache (Redis).
type CacheRepository interface {
    Get(ctx context.Context, code string) (string, error)
    Set(ctx context.Context, code, originalURL string, ttl time.Duration) error
}

type URLService struct {
    repo  URLRepository
    cache CacheRepository
}

func NewURLService(repo URLRepository, cache CacheRepository) *URLService {
    return &URLService{repo: repo, cache: cache}
}

// Shorten recebe uma URL original e retorna o registro com o código curto.
// Fluxo: validar URL → gerar código → salvar no Postgres → pré-aquecer cache.
func (s *URLService) Shorten(ctx context.Context, rawURL string) (*model.URL, error) {
    // Valida que a URL é bem formada antes de qualquer coisa.
    if _, err := url.ParseRequestURI(rawURL); err != nil {
        return nil, fmt.Errorf("%w: %s", ErrInvalidURL, rawURL)
    }

    // Gera um código de 7 caracteres base64url (ex: "aB3xK7q").
    // 7 chars = ~3.5 trilhões de combinações
    code, err := generateCode(7)
    if err != nil {
        return nil, fmt.Errorf("erro ao gerar código: %w", err)
    }

    record := model.URL{
        Code:        code,
        OriginalURL: rawURL,
        CreatedAt:   time.Now(),
        ExpiresAt:   time.Now().Add(30 * 24 * time.Hour), // expira em 30 dias
    }

    // Persiste no PostgreSQL — esta é a fonte da verdade.
    if err := s.repo.Save(ctx, record); err != nil {
        return nil, fmt.Errorf("erro ao salvar URL: %w", err)
    }

    // Pré-aquece o cache para que o primeiro redirecionamento já seja rápido.
    // Ignoramos o erro do cache: se o Redis estiver fora, a app ainda funciona.
    _ = s.cache.Set(ctx, code, rawURL, 24*time.Hour)

    return &record, nil
}

// Resolve retorna a URL original a partir do código curto.
// Estratégia cache-first: Redis → PostgreSQL (fallback).
// Um cache hit evita completamente uma query ao banco.
func (s *URLService) Resolve(ctx context.Context, code string) (string, error) {
    // Tenta o Redis primeiro (latência ~1ms vs ~5-10ms do Postgres).
    if cached, err := s.cache.Get(ctx, code); err == nil {
        // Incrementa o contador em background para não bloquear o redirecionamento.
        go s.repo.IncrementClicks(context.Background(), code)
        return cached, nil
    }

    // Cache miss: busca no PostgreSQL.
    record, err := s.repo.FindByCode(ctx, code)
    if err != nil {
        return "", fmt.Errorf("URL não encontrada: %w", err)
    }

    // Repovoar o cache para as próximas requisições (lazy caching).
    _ = s.cache.Set(ctx, code, record.OriginalURL, 24*time.Hour)
    go s.repo.IncrementClicks(context.Background(), code)

    return record.OriginalURL, nil
}

// generateCode gera uma string aleatória URL-safe de n caracteres.
// Usa crypto/rand (criptograficamente seguro) — nunca use math/rand para isso.
func generateCode(n int) (string, error) {
    b := make([]byte, n)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    // RawURLEncoding: sem '=' de padding, usa '-' e '_' em vez de '+' e '/'.
    return base64.RawURLEncoding.EncodeToString(b)[:n], nil
}