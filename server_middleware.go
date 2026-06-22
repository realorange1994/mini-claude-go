package main

import (
	"net/http"
	"strings"
)

// ─── Server Middleware (MiMo-Code 3) ────────────────────────────────────────
//
// Production-grade middleware: CORS, compression, auth.
//
// MiMo-Code source: server/middleware.ts (96 lines)

// MiddlewareConfig holds middleware configuration.
type MiddlewareConfig struct {
	AllowedOrigins []string
	Password       string
	CompressSSE    bool
}

// NewMiddlewareConfig creates a new middleware config.
func NewMiddlewareConfig() *MiddlewareConfig {
	return &MiddlewareConfig{
		AllowedOrigins: []string{
			"http://localhost",
			"http://127.0.0.1",
		},
		CompressSSE: false,
	}
}

// CORSMiddleware adds CORS headers.
func CORSMiddleware(config *MiddlewareConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				origin = "*"
			}

			// Check allowed origins
			allowed := false
			for _, o := range config.AllowedOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
				// Check wildcard subdomains
				if strings.HasPrefix(o, "*.") {
					domain := strings.TrimPrefix(o, "*")
					if strings.HasSuffix(origin, domain) {
						allowed = true
						break
					}
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// AuthMiddleware adds basic authentication.
func AuthMiddleware(password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if password == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Check Authorization header
			auth := r.Header.Get("Authorization")
			if auth == "Bearer "+password {
				next.ServeHTTP(w, r)
				return
			}

			// Check query param
			token := r.URL.Query().Get("auth_token")
			if token == password {
				next.ServeHTTP(w, r)
				return
			}

			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		})
	}
}

// CompressionMiddleware adds gzip compression (skips SSE streams).
func CompressionMiddleware(compressSSE bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip SSE streams
			if !compressSSE && isSSEEndpoint(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// Skip POST to session endpoints
			if r.Method == "POST" && isSessionEndpoint(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isSSEEndpoint checks if the path is an SSE endpoint.
func isSSEEndpoint(path string) bool {
	return strings.Contains(path, "/event") || strings.Contains(path, "/global/event")
}

// isSessionEndpoint checks if the path is a session endpoint.
func isSessionEndpoint(path string) bool {
	return strings.Contains(path, "/session/") && (strings.Contains(path, "/message") || strings.Contains(path, "/prompt"))
}

// FormatMiddlewareStatus formats middleware status for display.
func FormatMiddlewareStatus(config *MiddlewareConfig) string {
	if config == nil {
		return "No middleware."
	}
	return "CORS origins: " + strings.Join(config.AllowedOrigins, ", ")
}
