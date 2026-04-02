package service

import (
    "context"
    "crypto/rand"
    "encoding/base64"
    "time"

    "github.com/seu-user/url-shortener/internal/model"
)

type URLRepository interface {
    Save(ctx context.Context, url model.URL) error
    FindByCode(ctx context.Context, code string) (*model.URL, error)
}

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

func (s *URLService) Shorten(ctx context.Context, originalURL string) (*model.URL, error) {
    code, err := generateCode(7)
    if err != nil {
        return nil, err
    }

    url := model.URL{
        Code:        code,
        OriginalURL: originalURL,
        CreatedAt:   time.Now(),
        ExpiresAt:   time.Now().Add(30 * 24 * time.Hour),
    }

    if err := s.repo.Save(ctx, url); err != nil {
        return nil, err
    }

    // pré-aquece o cache já na criação
    s.cache.Set(ctx, code, originalURL, 24*time.Hour)

    return &url, nil
}

func (s *URLService) Resolve(ctx context.Context, code string) (string, error) {
    // cache-first: Redis é consultado antes do Postgres
    if cached, err := s.cache.Get(ctx, code); err == nil {
        return cached, nil
    }

    url, err := s.repo.FindByCode(ctx, code)
    if err != nil {
        return "", err
    }

    s.cache.Set(ctx, code, url.OriginalURL, 24*time.Hour)
    return url.OriginalURL, nil
}

func generateCode(n int) (string, error) {
    b := make([]byte, n)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    return base64.URLEncoding.EncodeToString(b)[:n], nil
}