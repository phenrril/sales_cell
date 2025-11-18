package httpserver

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type key int

const requestIDKey key = 1

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := make([]byte, 8)
		_, _ = rand.Read(id)
		ctx := context.WithValue(r.Context(), requestIDKey, fmtHex(id))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetReqID(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = time.Now() // start - kept for potential future logging
		rw := &respWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
	})
}

type respWriter struct {
	http.ResponseWriter
	status int
}

func (rw *respWriter) WriteHeader(code int) { rw.status = code; rw.ResponseWriter.WriteHeader(code) }

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				http.Error(w, "internal error", 500)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		gz := gzip.NewWriter(w)
		defer func() { _ = gz.Close() }()
		w.Header().Set("Content-Encoding", "gzip")
		gw := &gzipResponseWriter{ResponseWriter: w, Writer: gz}
		next.ServeHTTP(gw, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	io.Writer
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) { return w.Writer.Write(b) }

// SecurityAndStaticCache agrega headers de seguridad y caché fuerte para /public/
func SecurityAndStaticCache(next http.Handler) http.Handler {
	const (
		hsts   = "max-age=31536000; includeSubDomains; preload"
		xcto   = "nosniff"
		xfo    = "SAMEORIGIN"
		refpol = "strict-origin-when-cross-origin"
		coop   = "same-origin"
	)
	// Permite Google Fonts y habilita inline scripts para las vistas (admin usa <script> inline)
	const csp = "default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; script-src 'self' 'unsafe-inline'; font-src 'self' https://fonts.gstatic.com"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Headers de seguridad globales
		w.Header().Set("Strict-Transport-Security", hsts)
		w.Header().Set("X-Content-Type-Options", xcto)
		w.Header().Set("X-Frame-Options", xfo)
		w.Header().Set("Referrer-Policy", refpol)
		w.Header().Set("Cross-Origin-Opener-Policy", coop)
		w.Header().Set("Content-Security-Policy", csp)

		// Caché estático para /public/
		if strings.HasPrefix(r.URL.Path, "/public/") && r.Method == http.MethodGet {
			// En desarrollo, deshabilitar completamente el caché para ver cambios inmediatos
			appEnv := os.Getenv("APP_ENV")
			isDev := appEnv == "" || appEnv == "development" || appEnv == "dev"

			// Si tiene parámetro de versión en query string (ej: ?v=123), no cachear agresivamente
			hasVersion := r.URL.Query().Get("v") != ""

			if isDev || hasVersion {
				// En desarrollo: deshabilitar completamente el caché
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate, max-age=0")
				w.Header().Set("Pragma", "no-cache")
				w.Header().Set("Expires", "0")
			} else {
				// En producción: caché fuerte
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

				// ETag/Last-Modified basado en FS local (solo en producción)
				rel := strings.TrimPrefix(r.URL.Path, "/")
				rel = filepath.Clean(rel)
				if strings.HasPrefix(rel, "public"+string(filepath.Separator)) || rel == "public" {
					if fi, err := os.Stat(rel); err == nil && fi.Mode().IsRegular() {
						w.Header().Set("Last-Modified", fi.ModTime().UTC().Format(http.TimeFormat))
						etag := "W=\"" + strconv.FormatInt(fi.Size(), 10) + "-" + strconv.FormatInt(fi.ModTime().Unix(), 16) + "\""
						w.Header().Set("ETag", etag)
						if inm := r.Header.Get("If-None-Match"); inm != "" && strings.Contains(inm, etag) {
							w.WriteHeader(http.StatusNotModified)
							return
						}
					}
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}

func RateLimit(maxPerMin int) func(http.Handler) http.Handler {
	var mu sync.Mutex
	buckets := map[string]*bucket{}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if ip == "" {
				ip = r.RemoteAddr
			}
			mu.Lock()
			b, ok := buckets[ip]
			if !ok || time.Since(b.ts) > time.Minute {
				b = &bucket{count: 0, ts: time.Now()}
				buckets[ip] = b
			}
			if b.count >= maxPerMin {
				mu.Unlock()
				http.Error(w, "rate limit", http.StatusTooManyRequests)
				return
			}
			b.count++
			mu.Unlock()
			next.ServeHTTP(w, r)
		})
	}
}

func PublicRateLimit(perPathLimits map[string]int) func(http.Handler) http.Handler {

	var mu sync.Mutex
	buckets := map[string]*bucket{}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit, ok := perPathLimits[r.URL.Path]
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if ip == "" {
				ip = r.RemoteAddr
			}
			key := r.URL.Path + "|" + ip
			mu.Lock()
			b, ok := buckets[key]
			if !ok || time.Since(b.ts) > time.Minute {
				b = &bucket{count: 0, ts: time.Now()}
				buckets[key] = b
			}
			if b.count >= limit {
				mu.Unlock()
				w.Header().Set("Retry-After", "60")
				http.Error(w, "rate limit", http.StatusTooManyRequests)
				return
			}
			b.count++
			remaining := limit - b.count
			mu.Unlock()
			w.Header().Set("X-RateLimit-Limit", strconvItoa(limit))
			w.Header().Set("X-RateLimit-Remaining", strconvItoa(remaining))
			next.ServeHTTP(w, r)
		})
	}
}

func strconvItoa(i int) string { return fmtInt(i) }

func fmtInt(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	buf := [20]byte{}
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

type bucket struct {
	count int
	ts    time.Time
}

func Chain(h http.Handler, m ...func(http.Handler) http.Handler) http.Handler {
	for i := len(m) - 1; i >= 0; i-- {
		h = m[i](h)
	}
	return h
}

func fmtHex(b []byte) string {
	const hexd = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexd[v>>4]
		out[i*2+1] = hexd[v&0x0f]
	}
	return string(out)
}
