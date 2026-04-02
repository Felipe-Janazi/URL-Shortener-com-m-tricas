package handler

import (
    "encoding/json"
    "errors"
    "log/slog"
    "net/http"

    "github.com/Felipe-Janazi/url-shortener/internal/model"
    "github.com/Felipe-Janazi/url-shortener/internal/service"
)

// URLHandler só conhece HTTP — nenhuma regra de negócio vive aqui.
// Responsabilidades: decodificar request → chamar service → escrever response.
type URLHandler struct {
    svc *service.URLService
}

func NewURLHandler(svc *service.URLService) *URLHandler {
    return &URLHandler{svc: svc}
}

// Shorten trata POST /shorten.
// Espera: {"url": "https://..."} → Retorna: {"short_url": "...", "code": "..."}
func (h *URLHandler) Shorten(w http.ResponseWriter, r *http.Request) {
    var req model.ShortenRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "body inválido", http.StatusBadRequest)
        return
    }

    record, err := h.svc.Shorten(r.Context(), req.URL)
    if err != nil {
        // Diferencia erro de validação (400) de erro interno (500).
        // errors.Is percorre a cadeia de wrapping do fmt.Errorf("%w", ...).
        if errors.Is(err, service.ErrInvalidURL) {
            http.Error(w, err.Error(), http.StatusBadRequest)
        } else {
            slog.Error("erro ao encurtar URL", "err", err)
            http.Error(w, "erro interno", http.StatusInternalServerError)
        }
        return
    }

    resp := model.ShortenResponse{
        ShortURL:    r.Host + "/" + record.Code,
        Code:        record.Code,
        OriginalURL: record.OriginalURL,
        ExpiresAt:   record.ExpiresAt.Format("2006-01-02"),
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated) // 201: recurso criado com sucesso
    json.NewEncoder(w).Encode(resp)
}

// Redirect trata GET /{code}.
// Usa 302 (temporário) em vez de 301 (permanente) para que o browser
// sempre consulte a API — caso a URL mude ou expire, o redirect se atualiza.
func (h *URLHandler) Redirect(w http.ResponseWriter, r *http.Request) {
    code := r.PathValue("code") // PathValue é nativo do Go 1.22+

    originalURL, err := h.svc.Resolve(r.Context(), code)
    if err != nil {
        http.Error(w, "URL não encontrada", http.StatusNotFound)
        return
    }

    http.Redirect(w, r, originalURL, http.StatusFound) // 302 Found
}

// Health trata GET /health.
// Usado pelo ALB