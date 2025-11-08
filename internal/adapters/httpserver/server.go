package httpserver

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/oauth2"

	"github.com/phenrril/tienda3d/internal/adapters/payments/mercadopago"
	"github.com/phenrril/tienda3d/internal/domain"
	"github.com/phenrril/tienda3d/internal/usecase"
	"github.com/xuri/excelize/v2"
)

type Server struct {
	mux       *http.ServeMux
	tmpl      *template.Template
	products  *usecase.ProductUC
	quotes    *usecase.QuoteUC
	orders    *usecase.OrderUC
	payments  *usecase.PaymentUC
	models    domain.UploadedModelRepo
	storage   domain.FileStorage
	customers domain.CustomerRepo
	oauthCfg  *oauth2.Config

	adminAllowed map[string]struct{}
	adminSecret  []byte

	// último reporte de importación masiva (en memoria)
	lastImport *ImportReport
}

var emailRe = regexp.MustCompile(`^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$`)

func New(t *template.Template, p *usecase.ProductUC, q *usecase.QuoteUC, o *usecase.OrderUC, pay *usecase.PaymentUC, m domain.UploadedModelRepo, fs domain.FileStorage, customers domain.CustomerRepo, oauthCfg *oauth2.Config) http.Handler {
	s := &Server{tmpl: t, products: p, quotes: q, orders: o, payments: pay, models: m, storage: fs, customers: customers, oauthCfg: oauthCfg, mux: http.NewServeMux()}

	allowed := map[string]struct{}{}
	if raw := os.Getenv("ADMIN_ALLOWED_EMAILS"); raw != "" {
		for _, e := range strings.Split(raw, ",") {
			e = strings.ToLower(strings.TrimSpace(e))
			if e != "" {
				allowed[e] = struct{}{}
			}
		}
	}
	s.adminAllowed = allowed
	sec := os.Getenv("JWT_ADMIN_SECRET")
	if sec == "" {
		sec = os.Getenv("SECRET_KEY")
	}
	if sec == "" {
		sec = "dev-admin-secret"
	}
	s.adminSecret = []byte(sec)

	s.routes()
	return Chain(s.mux,
		PublicRateLimit(map[string]int{
			"/api/quote":    15,
			"/api/checkout": 10,
			"/webhooks/mp":  30,
		}),
		RateLimit(60),
		SecurityAndStaticCache,
		Gzip,
		RequestID,
		Recovery,
		Logging,
	)
}

func (s *Server) routes() {

	s.mux.Handle("/public/", http.StripPrefix("/public/", http.FileServer(http.Dir("public"))))

	s.mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("uploads"))))

	// SEO endpoints
	s.mux.HandleFunc("/robots.txt", s.handleRobots)
	s.mux.HandleFunc("/sitemap.xml", s.handleSitemap)

	s.mux.HandleFunc("/", s.handleHome)
	s.mux.HandleFunc("/products", s.handleProducts)
	s.mux.HandleFunc("/product/", s.handleProduct)
	s.mux.HandleFunc("/quote/", s.handleQuoteView)
	s.mux.HandleFunc("/checkout", s.handleCheckout)
	s.mux.HandleFunc("/pay/", s.handlePaySimulated)

	s.mux.HandleFunc("/cart", s.handleCart)
	s.mux.HandleFunc("/cart/update", s.handleCartUpdate)
	s.mux.HandleFunc("/cart/remove", s.handleCartRemove)
	s.mux.HandleFunc("/cart/checkout", s.handleCartCheckout)

	s.mux.HandleFunc("/api/products", s.apiProducts)
	s.mux.HandleFunc("/api/products/", s.apiProductByID)
	s.mux.HandleFunc("/api/products/clear-images/", s.apiProductClearImages)

	// Variantes por producto
	// GET /api/products/{slug}/variants · POST /api/products/{slug}/variants · DELETE /api/products/{slug}/variants/{id}
	// Se manejan dentro de apiProductByID por simplicidad de routing

	s.mux.HandleFunc("/api/products/upload", s.apiProductUpload)

	// Admin utilidades
	s.mux.HandleFunc("/admin/scan", s.handleAdminScan)
	s.mux.HandleFunc("/admin/import/csv", s.handleAdminImportCSV)
	s.mux.HandleFunc("/admin/export/csv", s.handleAdminExportCSV)
	// Reporte última importación
	s.mux.HandleFunc("/admin/uncharged", s.handleAdminUncharged)
	s.mux.HandleFunc("/api/quote", s.apiQuote)
	s.mux.HandleFunc("/api/checkout", s.apiCheckout)
	s.mux.HandleFunc("/webhooks/mp", s.webhookMP)
	s.mux.HandleFunc("/api/products/delete", s.apiProductsBulkDelete)

	s.mux.HandleFunc("/auth/google/login", s.handleGoogleLogin)
	s.mux.HandleFunc("/auth/google/callback", s.handleGoogleCallback)
	s.mux.HandleFunc("/logout", s.handleLogout)

	s.mux.HandleFunc("/admin/login", s.handleAdminLogin)
	s.mux.HandleFunc("/admin/auth", s.handleAdminAuth)
	s.mux.HandleFunc("/admin/logout", s.handleAdminLogout)

	s.mux.HandleFunc("/admin/orders", s.handleAdminOrders)
	s.mux.HandleFunc("/admin/products", s.handleAdminProducts)

	s.mux.HandleFunc("/admin/sales", s.handleAdminSales)
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	list, _, err := s.products.List(r.Context(), domain.ProductFilter{Page: 1, PageSize: 8})
	if err != nil {
		http.Error(w, "err", 500)
		return
	}
	base := s.canonicalBase(r)
	data := map[string]any{"Products": list, "CanonicalURL": base + "/", "OGImage": base + "/public/assets/img/chroma-logo.png"}
	if u := readUserSession(w, r); u != nil {
		data["User"] = u
	}
	s.render(w, "home.html", data)
}

func (s *Server) handleProducts(w http.ResponseWriter, r *http.Request) {
	qv := r.URL.Query()
	page, _ := strconv.Atoi(qv.Get("page"))
	if page < 1 {
		page = 1
	}
	sort := qv.Get("sort")
	query := qv.Get("q")
	category := qv.Get("category")
	pageSize := 24
	list, total, _ := s.products.List(r.Context(), domain.ProductFilter{Page: page, PageSize: pageSize, Sort: sort, Query: query, Category: category})
	pages := (int(total) + (pageSize - 1)) / pageSize
	if pages == 0 {
		pages = 1
	}
	cats, _ := s.products.Categories(r.Context())
	base := s.canonicalBase(r)
	data := map[string]any{
		"Products":     list,
		"Total":        total,
		"Page":         page,
		"Pages":        pages,
		"Query":        query,
		"Sort":         sort,
		"Category":     category,
		"Categories":   cats,
		"CanonicalURL": base + "/products",
		"OGImage":      base + "/public/assets/img/chroma-logo.png",
	}
	if u := readUserSession(w, r); u != nil {
		data["User"] = u
	}
	s.render(w, "products.html", data)
}

func (s *Server) handleProduct(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/product/")
	if slug == "" {
		http.NotFound(w, r)
		return
	}
	p, err := s.products.GetBySlug(r.Context(), slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	seen := map[string]struct{}{}
	colors := []string{}
	for _, v := range p.Variants {
		c := strings.TrimSpace(v.Color)
		if c == "" {
			continue
		}
		// mostrar sólo colores con stock
		if v.Stock <= 0 {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		colors = append(colors, c)
		if len(colors) == 16 {
			break
		}
	}
	defaultColor := "#111827"
	if len(colors) > 0 {
		defaultColor = colors[0]
	}
	added := 0
	if r.URL.Query().Get("added") == "1" {
		added = 1
	}
	base := s.canonicalBase(r)
	og := base + "/public/assets/img/chroma-logo.png"
	if len(p.Images) > 0 && strings.TrimSpace(p.Images[0].URL) != "" {
		if strings.HasPrefix(p.Images[0].URL, "http://") || strings.HasPrefix(p.Images[0].URL, "https://") {
			og = p.Images[0].URL
		} else {
			if !strings.HasPrefix(p.Images[0].URL, "/") {
				og = base + "/" + p.Images[0].URL
			} else {
				og = base + p.Images[0].URL
			}
		}
	}
	data := map[string]any{"Product": p, "Colors": colors, "DefaultColor": defaultColor, "Added": added, "CanonicalURL": base + "/product/" + p.Slug, "OGImage": og}
	if u := readUserSession(w, r); u != nil {
		data["User"] = u
	}
	s.render(w, "product.html", data)
}

// canonicalBase arma el esquema y host para URLs absolutas
func (s *Server) canonicalBase(r *http.Request) string {
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	if host == "" {
		host = "www.celusfera.com.ar"
	}
	return scheme + "://" + host
}

func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	base := s.canonicalBase(r)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	// listar productos
	var all []domain.Product
	page := 1
	for {
		list, total, err := s.products.List(r.Context(), domain.ProductFilter{Page: page, PageSize: 200})
		if err != nil {
			break
		}
		all = append(all, list...)
		if len(all) >= int(total) || len(list) == 0 {
			break
		}
		page++
		if page > 10 {
			break
		}
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	now := time.Now().Format("2006-01-02")
	b.WriteString("\n  <url><loc>" + base + "/" + "</loc><lastmod>" + now + "</lastmod></url>")
	b.WriteString("\n  <url><loc>" + base + "/products" + "</loc><lastmod>" + now + "</lastmod></url>")
	b.WriteString("\n  <url><loc>" + base + "/cart" + "</loc><lastmod>" + now + "</lastmod></url>")
	for _, p := range all {
		lm := p.UpdatedAt
		if lm.IsZero() {
			lm = p.CreatedAt
		}
		last := now
		if !lm.IsZero() {
			last = lm.Format("2006-01-02")
		}
		b.WriteString("\n  <url><loc>" + base + "/product/" + template.URLQueryEscaper(p.Slug) + "</loc><lastmod>" + last + "</lastmod></url>")
	}
	b.WriteString("\n</urlset>")
	_, _ = w.Write([]byte(b.String()))
}

func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	data, err := os.ReadFile("robots.txt")
	if err == nil {
		_, _ = w.Write(data)
		return
	}
	_, _ = w.Write([]byte("User-agent: *\nDisallow:\nSitemap: https://www.celusfera.com.ar/sitemap.xml\n"))
}

func (s *Server) handleQuoteView(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/quote/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	q, err := s.quotes.Quotes.FindByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data := map[string]any{"Quote": q}
	if u := readUserSession(w, r); u != nil {
		data["User"] = u
	}
	s.render(w, "quote.html", data)
}

func (s *Server) handleCheckout(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{}
	if u := readUserSession(w, r); u != nil {
		data["User"] = u
	}
	s.render(w, "checkout.html", data)
}

func (s *Server) apiProducts(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method == http.MethodGet {
		list, total, _ := s.products.List(r.Context(), domain.ProductFilter{Page: 1, PageSize: 100})
		writeJSON(w, 200, map[string]any{"items": list, "total": total})
		return
	}
	if r.Method == http.MethodPost {
		var req struct {
			Name        string            `json:"name"`
			Category    string            `json:"category"`
			ShortDesc   string            `json:"short_desc"`
			BasePrice   float64           `json:"base_price"`
			GrossPrice  float64           `json:"gross_price"`
			MarginPct   float64           `json:"margin_pct"`
			ReadyToShip bool              `json:"ready_to_ship"`
			WidthMM     float64           `json:"width_mm"`
			HeightMM    float64           `json:"height_mm"`
			DepthMM     float64           `json:"depth_mm"`
			Brand       string            `json:"brand"`
			Model       string            `json:"model"`
			Attributes  map[string]string `json:"attributes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "json", 400)
			return
		}
		// calcular BasePrice desde GrossPrice y MarginPct si corresponde
		if req.GrossPrice > 0 && req.MarginPct != 0 {
			req.BasePrice = req.GrossPrice * (1.0 + req.MarginPct/100.0)
		}
		if req.BasePrice == 0 && req.GrossPrice > 0 && req.MarginPct == 0 {
			// si no hay margen, usar bruto como base
			req.BasePrice = req.GrossPrice
		}
		if req.Name == "" || req.BasePrice < 0 || req.WidthMM < 0 || req.HeightMM < 0 || req.DepthMM < 0 {
			http.Error(w, "datos", 400)
			return
		}
		p := &domain.Product{Name: req.Name, Category: req.Category, ShortDesc: req.ShortDesc, BasePrice: req.BasePrice, GrossPrice: req.GrossPrice, MarginPct: req.MarginPct, ReadyToShip: req.ReadyToShip, WidthMM: req.WidthMM, HeightMM: req.HeightMM, DepthMM: req.DepthMM, Brand: req.Brand, Model: req.Model, Attributes: req.Attributes}
		if err := s.products.Create(r.Context(), p); err != nil {
			http.Error(w, "crear", 500)
			return
		}
		writeJSON(w, 201, p)
		return
	}
	http.Error(w, "method", 405)
}

func (s *Server) apiProductByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	// Variantes nested: /api/products/{slug}/variants[/{id}]
	if strings.Contains(r.URL.Path, "/variants") {
		s.apiProductVariants(w, r)
		return
	}
	// Download image from URL: /api/products/{slug}/download-image
	if strings.HasSuffix(r.URL.Path, "/download-image") {
		s.apiProductDownloadImage(w, r)
		return
	}
	if r.Method == http.MethodGet {
		idStr := strings.TrimPrefix(r.URL.Path, "/api/products/")
		p, err := s.products.GetBySlug(r.Context(), idStr)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, p)
		return
	}
	if r.Method == http.MethodPut {
		idStr := strings.TrimPrefix(r.URL.Path, "/api/products/")
		if idStr == "" {
			http.Error(w, "slug", 400)
			return
		}
		p, err := s.products.GetBySlug(r.Context(), idStr)
		if err != nil || p == nil {
			http.Error(w, "not found", 404)
			return
		}
		var req struct {
			Name        *string           `json:"name"`
			Category    *string           `json:"category"`
			ShortDesc   *string           `json:"short_desc"`
			BasePrice   *float64          `json:"base_price"`
			GrossPrice  *float64          `json:"gross_price"`
			MarginPct   *float64          `json:"margin_pct"`
			ReadyToShip *bool             `json:"ready_to_ship"`
			WidthMM     *float64          `json:"width_mm"`
			HeightMM    *float64          `json:"height_mm"`
			DepthMM     *float64          `json:"depth_mm"`
			Brand       *string           `json:"brand"`
			Model       *string           `json:"model"`
			Attributes  map[string]string `json:"attributes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "json", 400)
			return
		}
		if req.Name != nil {
			p.Name = *req.Name
		}
		if req.Category != nil {
			p.Category = *req.Category
		}
		if req.ShortDesc != nil {
			p.ShortDesc = *req.ShortDesc
		}
		// Primero aplicar bruto/margen si vienen; sobreescriben base si corresponde
		if req.GrossPrice != nil {
			p.GrossPrice = *req.GrossPrice
		}
		if req.MarginPct != nil {
			p.MarginPct = *req.MarginPct
		}
		if p.GrossPrice > 0 && p.MarginPct != 0 {
			p.BasePrice = p.GrossPrice * (1.0 + p.MarginPct/100.0)
		}
		if req.BasePrice != nil && *req.BasePrice >= 0 {
			p.BasePrice = *req.BasePrice
		}
		if req.ReadyToShip != nil {
			p.ReadyToShip = *req.ReadyToShip
		}
		if req.WidthMM != nil && *req.WidthMM >= 0 {
			p.WidthMM = *req.WidthMM
		}
		if req.HeightMM != nil && *req.HeightMM >= 0 {
			p.HeightMM = *req.HeightMM
		}
		if req.DepthMM != nil && *req.DepthMM >= 0 {
			p.DepthMM = *req.DepthMM
		}
		if req.Brand != nil {
			p.Brand = *req.Brand
		}
		if req.Model != nil {
			p.Model = *req.Model
		}
		if req.Attributes != nil {
			p.Attributes = req.Attributes
		}
		if err := s.products.Create(r.Context(), p); err != nil {
			http.Error(w, "save", 500)
			return
		}
		writeJSON(w, 200, p)
		return
	}
	if r.Method == http.MethodDelete {
		idStr := strings.TrimPrefix(r.URL.Path, "/api/products/")
		if idStr == "" {
			http.Error(w, "slug", 400)
			return
		}

		imgPaths, err := s.products.DeleteFullBySlug(r.Context(), idStr)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				http.Error(w, "not found", 404)
				return
			}
			http.Error(w, "delete", 500)
			return
		}
		removedFiles := []string{}
		for _, pth := range imgPaths {
			sp := strings.TrimSpace(pth)
			if sp == "" {
				continue
			}

			sp = strings.TrimPrefix(sp, "/")

			if !strings.Contains(sp, "uploads") {
				continue
			}
			if _, err := os.Stat(sp); err == nil {
				if err2 := os.Remove(sp); err2 == nil {
					removedFiles = append(removedFiles, sp)
				}
			}
		}
		writeJSON(w, 200, map[string]any{"status": "ok", "slug": idStr, "removed_files": removedFiles})
		return
	}
	http.Error(w, "method", 405)
}

// /api/products/clear-images/{slug}
func (s *Server) apiProductClearImages(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	slug := strings.TrimPrefix(r.URL.Path, "/api/products/clear-images/")
	slug = strings.TrimSpace(slug)
	if slug == "" {
		http.Error(w, "slug", 400)
		return
	}
	p, err := s.products.GetBySlug(r.Context(), slug)
	if err != nil || p == nil {
		http.Error(w, "prod", 404)
		return
	}
	// borrar de DB
	var removed []string
	if repo, ok := s.products.Products.(interface {
		ClearImages(context.Context, uuid.UUID) ([]string, error)
	}); ok {
		paths, err := repo.ClearImages(r.Context(), p.ID)
		if err != nil {
			http.Error(w, "clear", 500)
			return
		}
		removed = paths
	} else {
		http.Error(w, "unsupported", 500)
		return
	}
	// borrar de FS
	deleted := []string{}
	for _, sp := range removed {
		sps := strings.TrimSpace(sp)
		if sps == "" {
			continue
		}
		sps = strings.TrimPrefix(sps, "/")
		if !strings.Contains(sps, "uploads") {
			continue
		}
		if _, err := os.Stat(sps); err == nil {
			_ = os.Remove(sps)
			deleted = append(deleted, sps)
		}
	}
	writeJSON(w, 200, map[string]any{"status": "ok", "deleted": deleted})
}

// /api/products/{slug}/download-image - Download image from URL and associate with product
func (s *Server) apiProductDownloadImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/products/")
	slugEnc := strings.TrimSuffix(rest, "/download-image")
	slugEnc = strings.TrimSuffix(slugEnc, "/")
	slug, _ := url.PathUnescape(slugEnc)
	p, err := s.products.GetBySlug(r.Context(), slug)
	if err != nil || p == nil {
		http.Error(w, "product not found", http.StatusNotFound)
		return
	}

	var payload struct {
		ImageURL string `json:"image_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, 400, map[string]any{"status": "error", "message": "invalid json"})
		return
	}

	if !strings.HasPrefix(payload.ImageURL, "http") {
		writeJSON(w, 400, map[string]any{"status": "error", "message": "invalid url"})
		return
	}

	// Download image
	req2, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, payload.ImageURL, nil)
	req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil || resp2 == nil || resp2.StatusCode != 200 {
		writeJSON(w, 502, map[string]any{"status": "error", "message": "download failed"})
		return
	}
	data, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()
	if len(data) == 0 {
		writeJSON(w, 502, map[string]any{"status": "error", "message": "empty image"})
		return
	}

	// Determine extension
	ext := ".jpg"
	ct := strings.ToLower(resp2.Header.Get("Content-Type"))
	if strings.Contains(ct, "png") {
		ext = ".png"
	} else if strings.Contains(ct, "webp") {
		ext = ".webp"
	} else if strings.Contains(ct, "jpeg") || strings.Contains(ct, "jpg") {
		ext = ".jpg"
	}

	filename := sanitizeFileName(p.Slug + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ext)
	storedPath, err := s.storage.SaveImage(r.Context(), filename, data)
	if err != nil {
		writeJSON(w, 500, map[string]any{"status": "error", "message": "storage failed: " + err.Error()})
		return
	}

	if !strings.HasPrefix(storedPath, "/") {
		storedPath = "/" + strings.ReplaceAll(storedPath, "\\", "/")
	}

	img := domain.Image{URL: storedPath, Alt: p.Name}
	if err := s.products.AddImages(r.Context(), p.ID, []domain.Image{img}); err != nil {
		writeJSON(w, 500, map[string]any{"status": "error", "message": "db error: " + err.Error()})
		return
	}

	writeJSON(w, 200, map[string]any{"status": "ok", "image_url": storedPath, "message": "Imagen agregada exitosamente"})
}

func sanitizeFileName(name string) string {
	if name == "" {
		return "image.jpg"
	}
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, "/", "-")
	mapped := strings.Map(func(r rune) rune {
		if r == '.' || r == '-' || r == '_' || unicode.IsDigit(r) || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return r
		}
		return '-'
	}, name)
	for strings.Contains(mapped, "--") {
		mapped = strings.ReplaceAll(mapped, "--", "-")
	}
	mapped = strings.Trim(mapped, "-.")
	if mapped == "" {
		return "image.jpg"
	}
	return mapped
}

func splitWords(s string) []string {
	if s == "" {
		return nil
	}
	f := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == '/' || r == '\\'
	})
	out := make([]string, 0, len(f))
	for _, w := range f {
		w = strings.TrimSpace(w)
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

func includesAny(hay string, keys []string) bool {
	for _, k := range keys {
		if strings.Contains(hay, k) {
			return true
		}
	}
	return false
}

func includesAll(hay string, keys []string) bool {
	for _, k := range keys {
		if strings.TrimSpace(k) == "" {
			continue
		}
		if !strings.Contains(hay, k) {
			return false
		}
	}
	return true
}

// /api/products/{slug}/variants
func (s *Server) apiProductVariants(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/products/"), "/")
	if len(parts) < 2 || parts[1] != "variants" {
		http.Error(w, "path", 404)
		return
	}
	slug := parts[0]
	p, err := s.products.GetBySlug(r.Context(), slug)
	if err != nil || p == nil {
		http.Error(w, "prod", 404)
		return
	}
	// DELETE /api/products/{slug}/variants/{id}
	if r.Method == http.MethodDelete && len(parts) == 3 {
		vid, err := uuid.Parse(parts[2])
		if err != nil {
			http.Error(w, "variant", 400)
			return
		}
		if err := s.products.DeleteVariant(r.Context(), vid); err != nil {
			http.Error(w, "delete", 500)
			return
		}
		writeJSON(w, 200, map[string]any{"status": "ok"})
		return
	}
	if r.Method == http.MethodGet {
		list, err := s.products.ListVariants(r.Context(), p.ID)
		if err != nil {
			http.Error(w, "list", 500)
			return
		}
		writeJSON(w, 200, map[string]any{"items": list})
		return
	}
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		var req struct {
			ID         string            `json:"id"`
			SKU        string            `json:"sku"`
			EAN        string            `json:"ean"`
			Attributes map[string]string `json:"attributes"`
			Price      float64           `json:"price"`
			Cost       float64           `json:"cost"`
			Stock      int               `json:"stock"`
			ImageURL   string            `json:"image_url"`
			Color      string            `json:"color"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "json", 400)
			return
		}
		var v domain.Variant
		if req.ID != "" {
			if uid, err := uuid.Parse(req.ID); err == nil {
				v.ID = uid
			}
		}
		v.ProductID = p.ID
		v.SKU = strings.TrimSpace(req.SKU)
		v.EAN = strings.TrimSpace(req.EAN)
		v.Attributes = req.Attributes
		v.Price = req.Price
		v.Cost = req.Cost
		v.Stock = req.Stock
		v.ImageURL = strings.TrimSpace(req.ImageURL)
		v.Color = strings.TrimSpace(req.Color)
		if v.Price < 0 || v.Cost < 0 || v.Stock < 0 {
			http.Error(w, "datos", 400)
			return
		}
		if v.ID == uuid.Nil {
			if err := s.products.CreateVariant(r.Context(), &v); err != nil {
				http.Error(w, "create", 500)
				return
			}
		} else {
			if err := s.products.UpdateVariant(r.Context(), &v); err != nil {
				http.Error(w, "update", 500)
				return
			}
		}
		writeJSON(w, 200, v)
		return
	}
	http.Error(w, "method", 405)
}

func (s *Server) apiProductsBulkDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	var req struct {
		Slugs []string `json:"slugs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Slugs) == 0 {
		http.Error(w, "json", 400)
		return
	}
	deleted := []string{}
	errorsMap := map[string]string{}
	for _, sl := range req.Slugs {
		if sl == "" {
			continue
		}
		if err := s.products.DeleteBySlug(r.Context(), sl); err != nil {
			errorsMap[sl] = err.Error()
		} else {
			deleted = append(deleted, sl)
		}
	}
	writeJSON(w, 200, map[string]any{"deleted": deleted, "errors": errorsMap})
}

func (s *Server) apiQuote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}

	dec := json.NewDecoder(io.LimitReader(r.Body, 2048))
	var req struct {
		UploadedModelID string  `json:"uploaded_model_id"`
		Material        string  `json:"material"`
		Layer           float64 `json:"layer_height_mm"`
		Infill          int     `json:"infill_pct"`
		Quality         string  `json:"quality"`
	}
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "json", 400)
		return
	}

	mat := strings.ToUpper(strings.TrimSpace(req.Material))
	allowedMat := map[string]struct{}{string(domain.MaterialPLA): {}, string(domain.MaterialPETG): {}, string(domain.MaterialTPU): {}}
	if _, ok := allowedMat[mat]; !ok {
		http.Error(w, "datos", 400)
		return
	}
	qual := strings.ToLower(strings.TrimSpace(req.Quality))
	allowedQual := map[string]struct{}{string(domain.QualityDraft): {}, string(domain.QualityStandard): {}, string(domain.QualityHigh): {}}
	if _, ok := allowedQual[qual]; !ok {
		http.Error(w, "datos", 400)
		return
	}
	if req.Layer <= 0 || req.Layer > 1.0 {
		http.Error(w, "datos", 400)
		return
	}
	if req.Infill < 0 || req.Infill > 100 {
		http.Error(w, "datos", 400)
		return
	}
	id, err := uuid.Parse(req.UploadedModelID)
	if err != nil {
		http.Error(w, "model", 400)
		return
	}
	model, err := s.models.FindByID(r.Context(), id)
	if err != nil {
		http.Error(w, "model", 404)
		return
	}
	q, err := s.quotes.CreateFromModel(r.Context(), model, domain.QuoteConfig{Material: domain.Material(mat), LayerHeightMM: req.Layer, InfillPct: req.Infill, Quality: domain.PrintQuality(qual)})
	if err != nil {
		http.Error(w, "quote", 500)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, q)
}

func (s *Server) apiCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 2048))
	var req struct {
		QuoteID string `json:"quote_id"`
		Email   string `json:"email"`
	}
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "json", 400)
		return
	}
	if !emailRe.MatchString(strings.TrimSpace(req.Email)) {
		http.Error(w, "email", 400)
		return
	}
	qid, err := uuid.Parse(req.QuoteID)
	if err != nil {
		http.Error(w, "quote", 400)
		return
	}
	q, err := s.quotes.Quotes.FindByID(r.Context(), qid)
	if err != nil {
		http.Error(w, "quote", 404)
		return
	}
	if time.Now().After(q.ExpireAt) {
		http.Error(w, "expired", 400)
		return
	}
	order, err := s.orders.CreateFromQuote(r.Context(), q, strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil {
		http.Error(w, "order", 500)
		return
	}
	payURL, err := s.payments.CreatePreference(r.Context(), order)
	if err != nil {
		http.Error(w, "payment", 500)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"init_point": payURL, "order_id": order.ID})
}

func (s *Server) webhookMP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 65536))
	var evt struct {
		Type   string `json:"type"`
		Action string `json:"action"`
		Data   struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &evt)
	payID := evt.Data.ID
	if payID == "" {
		payID = r.URL.Query().Get("id")
	}
	if payID == "" {
		log.Warn().Msg("webhook sin payment id")
		w.WriteHeader(200)
		return
	}
	status, extRef, err := s.payments.Gateway.PaymentInfo(r.Context(), payID)
	if err != nil {
		log.Error().Err(err).Str("payment_id", payID).Msg("payment info")
		w.WriteHeader(200)
		return
	}
	orderID, ok := mercadopago.VerifyExternalRef(extRef)
	if !ok {
		log.Warn().Str("ext", extRef).Msg("external ref inválido")
		w.WriteHeader(200)
		return
	}
	uid, err := uuid.Parse(orderID)
	if err != nil {
		w.WriteHeader(200)
		return
	}
	o, err := s.orders.Orders.FindByID(r.Context(), uid)
	if err != nil || o == nil {
		log.Error().Err(err).Str("order_id", orderID).Msg("orden no encontrada para webhook")
		w.WriteHeader(200)
		return
	}
	approved := false
	switch status {
	case "approved":
		approved = true
		o.MPStatus = "approved"
		o.Status = domain.OrderStatusFinished
	case "pending", "in_process", "in_mediation":
		o.MPStatus = status
		if o.Status != domain.OrderStatusFinished {
			o.Status = domain.OrderStatusAwaitingPay
		}
	default:
		o.MPStatus = status
		if status == "rejected" {
			o.Status = domain.OrderStatusCancelled
		}
	}
	notify := false
	if approved && !o.Notified {
		o.Notified = true
		notify = true
	}
	if err := s.orders.Orders.Save(r.Context(), o); err != nil {
		log.Error().Err(err).Msg("guardar orden webhook")
	}
	if notify {
		go sendOrderNotify(o, true)
	}
	w.WriteHeader(200)
}

type cartItem struct {
	Slug  string  `json:"slug"`
	Color string  `json:"color"`
	Qty   int     `json:"qty"`
	Price float64 `json:"price"`
}

type cartPayload struct {
	Items []cartItem `json:"items"`
}

type cartLine struct {
	Slug      string
	Color     string
	Qty       int
	UnitPrice float64
	Subtotal  float64
	Name      string
	Image     string
}

func aggregateCart(cp cartPayload, lookup func(slug string) (*domain.Product, error)) []cartLine {
	m := map[string]*cartLine{}
	for _, it := range cp.Items {
		if it.Qty <= 0 {
			continue
		}
		key := it.Slug + "|" + it.Color
		line, ok := m[key]
		if !ok {
			line = &cartLine{Slug: it.Slug, Color: it.Color, Qty: 0, UnitPrice: it.Price}
			m[key] = line
		}
		line.Qty += it.Qty
	}
	res := []cartLine{}
	for _, l := range m {
		p, err := lookup(l.Slug)
		if err == nil && p != nil {
			l.Name = p.Name
			if len(p.Images) > 0 {
				l.Image = p.Images[0].URL
			}

			if p.BasePrice != 0 {
				l.UnitPrice = p.BasePrice
			}
		}
		l.Subtotal = l.UnitPrice * float64(l.Qty)
		res = append(res, *l)
	}
	return res
}

var provinceCosts = map[string]float64{
	"Santa Fe":            9000,
	"Buenos Aires":        9000,
	"CABA":                9000,
	"Cordoba":             9000,
	"Entre Rios":          9000,
	"Corrientes":          9000,
	"Chaco":               9000,
	"Misiones":            9000,
	"Formosa":             9000,
	"Santiago del Estero": 9000,
	"Tucuman":             9000,
	"Salta":               9000,
	"Jujuy":               9000,
	"Catamarca":           9000,
	"La Rioja":            9000,
	"San Juan":            9000,
	"San Luis":            9000,
	"Mendoza":             9000,
	"La Pampa":            9000,
	"Neuquen":             9000,
	"Rio Negro":           9000,
	"Chubut":              9000,
	"Santa Cruz":          9000,
	"Tierra del Fuego":    9000,
}

func shippingCostFor(province string) float64 {
	if v, ok := provinceCosts[province]; ok {
		return v
	}
	if province == "" {
		return 0
	}
	return 9000
}

func (s *Server) handleCart(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		cp := readCart(r)
		lines := aggregateCart(cp, func(slug string) (*domain.Product, error) { return s.products.GetBySlug(r.Context(), slug) })
		total := 0.0
		for _, l := range lines {
			total += l.Subtotal
		}
		provs := []string{}
		for p := range provinceCosts {
			provs = append(provs, p)
		}
		data := map[string]any{"Lines": lines, "Total": total, "Provinces": provs, "ProvinceCosts": provinceCosts}
		if u := readUserSession(w, r); u != nil {
			data["User"] = u
		}
		s.render(w, "cart.html", data)
		return
	}
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "form", 400)
			return
		}
		slug := r.FormValue("slug")
		color := r.FormValue("color")
		// Intento fallback si slug vacío y multipart presente
		if slug == "" && r.MultipartForm != nil {
			if v, ok := r.MultipartForm.Value["slug"]; ok && len(v) > 0 {
				slug = v[0]
			}
			if color == "" {
				if v, ok := r.MultipartForm.Value["color"]; ok && len(v) > 0 {
					color = v[0]
				}
			}
		}
		if slug == "" {
			http.Error(w, "slug", 400)
			return
		}
		p, err := s.products.GetBySlug(r.Context(), slug)
		if err != nil {
			http.Error(w, "prod", 404)
			return
		}
		// Fallback de color: si llega vacío, usar el primer color disponible o #111827
		if strings.TrimSpace(color) == "" {
			// intentar deducir de variantes del producto
			seen := map[string]struct{}{}
			for _, v := range p.Variants {
				c := strings.TrimSpace(v.Color)
				if c == "" {
					continue
				}
				if _, ok := seen[c]; ok {
					continue
				}
				color = c
				break
			}
			if strings.TrimSpace(color) == "" {
				color = "#111827"
			}
		}
		// Convertir SIEMPRE a nombre genérico cuando sea hex conocido
		cart := readCart(r)
		cart.Items = append(cart.Items, cartItem{Slug: slug, Color: normalizeColorName(color), Qty: 1, Price: p.BasePrice})
		writeCart(w, cart)
		accept := r.Header.Get("Accept")
		if strings.Contains(accept, "application/json") || r.Header.Get("X-Requested-With") == "fetch" {
			count := 0
			for _, it := range cart.Items {
				count += it.Qty
			}
			writeJSON(w, 200, map[string]any{"status": "ok", "slug": slug, "items": count})
			return
		}
		http.Redirect(w, r, "/product/"+slug+"?added=1", 302)
		return
	}
	http.Error(w, "method", 405)
}

func (s *Server) handleCartUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form", 400)
		return
	}
	slug := r.FormValue("slug")
	color := r.FormValue("color")
	op := r.FormValue("op")
	qtyStr := r.FormValue("qty")
	cart := readCart(r)

	agg := map[string]int{}
	for _, it := range cart.Items {
		if it.Qty > 0 {
			agg[it.Slug+"|"+it.Color] += it.Qty
		}
	}
	key := slug + "|" + color
	cur := agg[key]
	switch op {
	case "inc":
		cur++
	case "dec":
		if cur > 1 {
			cur--
		} else {
			cur = 0
		}
	case "set":
		if q, err := strconv.Atoi(qtyStr); err == nil {
			cur = q
		}
	}
	if cur < 0 {
		cur = 0
	}
	agg[key] = cur

	newCart := cartPayload{}
	for k, q := range agg {
		if q <= 0 {
			continue
		}
		parts := strings.SplitN(k, "|", 2)
		newCart.Items = append(newCart.Items, cartItem{Slug: parts[0], Color: normalizeColorName(parts[1]), Qty: q})
	}

	for i := range newCart.Items {
		p, _ := s.products.GetBySlug(r.Context(), newCart.Items[i].Slug)
		if p != nil {
			newCart.Items[i].Price = p.BasePrice
		}
	}
	writeCart(w, newCart)
	http.Redirect(w, r, "/cart", 302)
}

func (s *Server) handleCartRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form", 400)
		return
	}
	slug := r.FormValue("slug")
	color := r.FormValue("color")
	cart := readCart(r)
	newItems := []cartItem{}
	for _, it := range cart.Items {
		if !(it.Slug == slug && it.Color == color) {
			newItems = append(newItems, it)
		}
	}
	cart.Items = newItems
	writeCart(w, cart)
	http.Redirect(w, r, "/cart", 302)
}

// normalizeColorName transforma códigos hex válidos en nombres simples si hay match
// y limpia espacios. Para hex que no matchean, deja el valor original.
func normalizeColorName(c string) string {
	s := strings.TrimSpace(c)
	if s == "" {
		return s
	}
	lower := strings.ToLower(s)
	// Map comunes
	m := map[string]string{
		"#111827": "Negro",
		"#000000": "Negro",
		"#ffffff": "Blanco",
		"#ff0000": "Rojo",
		"#dc2626": "Rojo",
		"#10b981": "Verde",
		"#3b82f6": "Azul",
		"#6366f1": "Violeta",
		"#f59e0b": "Amarillo",
		"#ef4444": "Rojo",
		"#8b5cf6": "Violeta",
		"#ec4899": "Rosa",
		"#14b8a6": "Turquesa",
		"#f472b6": "Rosa",
		"#fcd34d": "Amarillo",
		"#a3e635": "Lima",
		"#334155": "Gris oscuro",
		"#64748b": "Gris",
	}
	if name, ok := m[lower]; ok {
		return name
	}
	return s
}

func formatColorES(c string) string {
	s := strings.TrimSpace(c)
	if s == "" {
		return ""
	}
	// Si es hex, devolver nombre si lo conocemos
	if strings.HasPrefix(s, "#") && (len(s) == 7 || len(s) == 4) {
		return normalizeColorName(s)
	}
	return s
}

func (s *Server) handleCartCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form", 400)
		return
	}
	email := r.FormValue("email")
	name := r.FormValue("name")
	phone := r.FormValue("phone")
	dni := r.FormValue("dni")
	postal := r.FormValue("postal_code")
	if email == "" || name == "" {
		http.Redirect(w, r, "/cart?err=datos", 302)
		return
	}
	shippingMethod := r.FormValue("shipping")
	if shippingMethod == "" {
		shippingMethod = "retiro"
	}

	addrEnvio := r.FormValue("address_envio")
	addrCadete := r.FormValue("address_cadete")
	legacyAddr := r.FormValue("address")
	province := r.FormValue("province")
	address := ""
	switch shippingMethod {
	case "envio":
		address = addrEnvio
	case "cadete":
		address = addrCadete
	default:
		address = legacyAddr
	}

	if shippingMethod == "envio" {
		if province == "" || address == "" || postal == "" || dni == "" || phone == "" {
			http.Redirect(w, r, "/cart?err=envio", 302)
			return
		}
		dniRe := regexp.MustCompile(`^\d{7,8}$`)
		pcRe := regexp.MustCompile(`^\d{4,5}$`)
		if !dniRe.MatchString(dni) || !pcRe.MatchString(postal) {
			http.Redirect(w, r, "/cart?err=formato", 302)
			return
		}
	} else if shippingMethod == "cadete" {
		if address == "" || phone == "" {
			http.Redirect(w, r, "/cart?err=cadete", 302)
			return
		}
		if province == "" {
			province = "Santa Fe"
		}
	}
	cp := readCart(r)
	if len(cp.Items) == 0 {
		http.Redirect(w, r, "/cart?err=vacio", 302)
		return
	}
	lines := aggregateCart(cp, func(slug string) (*domain.Product, error) { return s.products.GetBySlug(r.Context(), slug) })
	if len(lines) == 0 {
		http.Redirect(w, r, "/cart?err=vacio", 302)
		return
	}
	o := &domain.Order{ID: uuid.New(), Status: domain.OrderStatusAwaitingPay, Email: email, Name: name, Phone: phone, DNI: dni, PostalCode: postal, ShippingMethod: shippingMethod}
	itemsTotal := 0.0
	for _, l := range lines {
		p, _ := s.products.GetBySlug(r.Context(), l.Slug)
		var pid *uuid.UUID
		var title string
		if p != nil {
			pid = &p.ID
			title = p.Name
		} else {
			title = "Producto"
		}
		o.Items = append(o.Items, domain.OrderItem{ID: uuid.New(), ProductID: pid, Qty: l.Qty, UnitPrice: l.UnitPrice, Title: title, Color: normalizeColorName(l.Color)})
		itemsTotal += l.UnitPrice * float64(l.Qty)
	}
	shippingCost := 0.0
	if shippingMethod == "envio" {
		shippingCost = shippingCostFor(province)
		if address == "" {
			address = "(sin dirección)"
		}
		o.Address = address
		o.Province = province
	} else if shippingMethod == "cadete" {
		shippingCost = 5000
		if address == "" {
			address = "(sin dirección)"
		}
		o.Address = address
		if province == "" {
			province = "Santa Fe"
		}
		o.Province = province
	}
	o.ShippingCost = shippingCost
	o.Total = itemsTotal + shippingCost
	if err := s.orders.Orders.Save(r.Context(), o); err != nil {
		http.Redirect(w, r, "/cart?err=orden", 302)
		return
	}
	redirURL, err := s.payments.CreatePreference(r.Context(), o)
	if err != nil {
		redirURL = "/pay/" + o.ID.String()
	} else {
		_ = s.orders.Orders.Save(r.Context(), o)
	}
	writeCart(w, cartPayload{})
	http.Redirect(w, r, redirURL, 302)
}

func (s *Server) handlePaySimulated(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/pay/")
	uid, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	o, err := s.orders.Orders.FindByID(r.Context(), uid)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	status := strings.ToLower(q.Get("status"))
	if status == "" {
		status = strings.ToLower(q.Get("collection_status"))
	}
	success := false
	if status == "approved" {
		success = true
	}
	if status != "" {
		if success {
			o.MPStatus = "approved"
			o.Status = domain.OrderStatusFinished
			if !o.Notified {
				o.Notified = true
				_ = s.orders.Orders.Save(r.Context(), o)
				go sendOrderNotify(o, true)
			} else {
				_ = s.orders.Orders.Save(r.Context(), o)
			}
		} else {
			o.MPStatus = status
			_ = s.orders.Orders.Save(r.Context(), o)
		}
	}
	msg := "Pago pendiente / simulado"
	if status == "rejected" {
		msg = "El pago fue rechazado."
	}
	if success {
		msg = "Pago aprobado. Gracias por tu compra."
	}
	data := map[string]any{"Order": o, "StatusMsg": msg, "Success": success}
	if u := readUserSession(w, r); u != nil {
		data["User"] = u
	}
	s.render(w, "pay.html", data)
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	if m, ok := data.(map[string]any); ok {
		if _, exists := m["Year"]; !exists {
			m["Year"] = time.Now().Year()
		}
		if _, exists := m["User"]; !exists {
			if u := readUserSession(w, nil); u != nil {
				m["User"] = u
			}
		}
		data = m
	} else {
		m2 := map[string]any{"Year": time.Now().Year()}
		if u := readUserSession(w, nil); u != nil {
			m2["User"] = u
		}
		data = m2
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Error().Err(err).Str("tpl", name).Msg("render")
		http.Error(w, "tpl", 500)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func secretKey() []byte {
	k := os.Getenv("SESSION_KEY")
	if k == "" {
		k = "dev-insecure"
	}
	return []byte(k)
}

func readCart(r *http.Request) cartPayload {
	c, err := r.Cookie("cart")
	if err != nil {
		return cartPayload{}
	}
	parts := strings.SplitN(c.Value, ".", 2)
	if len(parts) != 2 {
		return cartPayload{}
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[0])
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	h := hmac.New(sha256.New, secretKey())
	h.Write(payload)
	if !hmac.Equal(sig, h.Sum(nil)) {
		return cartPayload{}
	}
	var cp cartPayload
	_ = json.Unmarshal(payload, &cp)
	return cp
}

func writeCart(w http.ResponseWriter, cp cartPayload) {
	b, _ := json.Marshal(cp)
	h := hmac.New(sha256.New, secretKey())
	h.Write(b)
	sig := base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	val := sig + "." + base64.RawURLEncoding.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{Name: "cart", Value: val, Path: "/", MaxAge: 60 * 60 * 24 * 7, HttpOnly: true})
}

func (s *Server) apiProductUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}

	if err := r.ParseMultipartForm(25 << 20); err != nil {
		http.Error(w, "multipart", 400)
		return
	}
	existingSlug := strings.TrimSpace(r.FormValue("existing_slug"))
	var p *domain.Product
	if existingSlug != "" {
		if prod, err := s.products.GetBySlug(r.Context(), existingSlug); err == nil {
			p = prod
		} else {
			http.Error(w, "prod", 404)
			return
		}
	}
	if p == nil {
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			http.Error(w, "name", 400)
			return
		}
		bp, _ := strconv.ParseFloat(r.FormValue("base_price"), 64)
		gp, _ := strconv.ParseFloat(r.FormValue("gross_price"), 64)
		mp, _ := strconv.ParseFloat(r.FormValue("margin_pct"), 64)
		if gp > 0 && mp != 0 {
			bp = gp * (1.0 + mp/100.0)
		}
		if bp < 0 {
			http.Error(w, "price", 400)
			return
		}
		cat := r.FormValue("category")
		sdesc := r.FormValue("short_desc")
		brand := r.FormValue("brand")
		model := r.FormValue("model")
		attrsRaw := strings.TrimSpace(r.FormValue("attributes"))
		ready := r.FormValue("ready_to_ship") == "true" || r.FormValue("ready_to_ship") == "1"
		wm, _ := strconv.ParseFloat(r.FormValue("width_mm"), 64)
		hm, _ := strconv.ParseFloat(r.FormValue("height_mm"), 64)
		dm, _ := strconv.ParseFloat(r.FormValue("depth_mm"), 64)
		if wm < 0 {
			wm = 0
		}
		if hm < 0 {
			hm = 0
		}
		if dm < 0 {
			dm = 0
		}
		p = &domain.Product{Name: name, Category: cat, ShortDesc: sdesc, BasePrice: bp, GrossPrice: gp, MarginPct: mp, ReadyToShip: ready, WidthMM: wm, HeightMM: hm, DepthMM: dm, Brand: brand, Model: model}
		if attrsRaw != "" {
			var m map[string]string
			if json.Unmarshal([]byte(attrsRaw), &m) == nil {
				p.Attributes = m
			}
		}
		if err := s.products.Create(r.Context(), p); err != nil {
			log.Error().Err(err).Msg("crear producto")
			http.Error(w, "crear", 500)
			return
		}
	}

	// Máximo 6 imágenes por producto: calcular remanente
	currentCount := 0
	if rp, err := s.products.GetBySlug(r.Context(), p.Slug); err == nil && rp != nil {
		currentCount = len(rp.Images)
	}
	maxRemaining := 6 - currentCount
	if maxRemaining < 0 {
		maxRemaining = 0
	}

	files := []*multipart.FileHeader{}
	if r.MultipartForm != nil {
		if fhArr, ok := r.MultipartForm.File["image"]; ok {
			files = append(files, fhArr...)
		}
		if fhArr, ok := r.MultipartForm.File["images"]; ok {
			files = append(files, fhArr...)
		}
	}
	if maxRemaining == 0 {
		writeJSON(w, 201, map[string]any{"product": p, "added_images": 0, "limit_reached": true})
		return
	}
	if len(files) > maxRemaining {
		files = files[:maxRemaining]
	}
	imgs := []domain.Image{}
	for _, fh := range files {
		if fh.Size == 0 {
			continue
		}
		f, err := fh.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil || len(data) == 0 {
			continue
		}
		storedPath, err := s.storage.SaveImage(r.Context(), fh.Filename, data)
		if err != nil {
			log.Warn().Err(err).Str("file", fh.Filename).Msg("no se pudo guardar imagen")
			continue
		}
		if !strings.HasPrefix(storedPath, "/") {
			storedPath = "/" + strings.ReplaceAll(storedPath, "\\", "/")
		}
		imgs = append(imgs, domain.Image{URL: storedPath, Alt: p.Name})
	}
	if len(imgs) > 0 {
		if err := s.products.AddImages(r.Context(), p.ID, imgs); err != nil {
			log.Error().Err(err).Msg("add images")
		}
		if rp, err := s.products.GetBySlug(r.Context(), p.Slug); err == nil {
			p = rp
		}
	}
	writeJSON(w, 201, map[string]any{"product": p, "added_images": len(imgs)})
}

func (s *Server) handleAdminProducts(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminSession(r) {
		http.Redirect(w, r, "/admin/auth", 302)
		return
	}
	list, total, _ := s.products.List(r.Context(), domain.ProductFilter{Page: 1, PageSize: 200})

	tok := s.readAdminToken(r)
	data := map[string]any{"Products": list, "Total": total, "AdminToken": tok}
	s.render(w, "admin_products.html", data)
}

// admin/uncharged: muestra el listado de items sin precio/detectados durante la última importación
func (s *Server) handleAdminUncharged(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminSession(r) {
		http.Redirect(w, r, "/admin/auth", 302)
		return
	}
	rep := s.lastImport
	if rep == nil {
		rep = &ImportReport{}
	}
	tok := s.readAdminToken(r)
	data := map[string]any{"Report": rep, "AdminToken": tok}
	s.render(w, "admin_uncharged.html", data)
}

func (s *Server) handleAdminOrders(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminSession(r) {
		http.Redirect(w, r, "/admin/auth", 302)
		return
	}
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	var mpStatus *string
	filterApproved := false
	if r.URL.Query().Get("approved") == "1" {
		st := "approved"
		mpStatus = &st
		filterApproved = true
	}
	list, total, err := s.orders.Orders.List(r.Context(), nil, mpStatus, page, 20)
	if err != nil {
		http.Error(w, "err", 500)
		return
	}
	pages := (int(total) + 19) / 20
	data := map[string]any{"Orders": list, "Page": page, "Pages": pages, "AdminToken": s.readAdminToken(r), "FilterApproved": filterApproved}
	s.render(w, "admin_orders.html", data)
}

func (s *Server) handleAdminSales(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminSession(r) {
		http.Redirect(w, r, "/admin/auth", 302)
		return
	}
	q := r.URL.Query()
	layout := "admin_sales.html"

	const layoutIn = "2006-01-02"
	var (
		to   time.Time
		from time.Time
		err  error
	)
	if ds := q.Get("to"); ds != "" {
		to, err = time.Parse(layoutIn, ds)
		if err != nil {
			to = time.Now()
		}
	} else {
		to = time.Now()
	}
	if ds := q.Get("from"); ds != "" {
		from, err = time.Parse(layoutIn, ds)
		if err != nil {
			from = to.AddDate(0, 0, -29)
		}
	} else {
		from = to.AddDate(0, 0, -29)
	}
	if from.After(to) {
		from, to = to, from
	}

	ordersAll, err := s.orders.Orders.ListInRange(r.Context(), from, to)
	if err != nil {
		http.Error(w, "err", 500)
		return
	}

	orders := make([]domain.Order, 0, len(ordersAll))
	for _, o := range ordersAll {
		if strings.EqualFold(o.MPStatus, "approved") {
			orders = append(orders, o)
		}
	}

	var totalRevenue, shippingRevenue float64
	statusCounts := map[string]int{}
	mpStatusCounts := map[string]int{}
	shippingMethodCounts := map[string]int{}
	provinceCounts := map[string]int{}
	itemsRevenue := 0.0
	productAgg := map[string]struct {
		Title   string
		Qty     int
		Revenue float64
	}{}
	dayRevenue := map[string]struct {
		Revenue float64
		Orders  int
	}{}

	for _, o := range orders {
		totalRevenue += o.Total
		shippingRevenue += o.ShippingCost
		statusCounts[string(o.Status)]++
		if o.MPStatus != "" {
			mpStatusCounts[o.MPStatus]++
		}
		if o.ShippingMethod != "" {
			shippingMethodCounts[o.ShippingMethod]++
		}
		if o.Province != "" {
			provinceCounts[o.Province]++
		}
		dayKey := o.CreatedAt.Format("2006-01-02")
		dr := dayRevenue[dayKey]
		dr.Revenue += o.Total
		dr.Orders++
		dayRevenue[dayKey] = dr
		lineItems := 0.0
		for _, it := range o.Items {
			lineRev := it.UnitPrice * float64(it.Qty)
			lineItems += lineRev
			key := it.Title
			cur := productAgg[key]
			cur.Title = it.Title
			cur.Qty += it.Qty
			cur.Revenue += lineRev
			productAgg[key] = cur
		}
		itemsRevenue += lineItems
	}
	avgOrderValue := 0.0
	if len(orders) > 0 {
		avgOrderValue = totalRevenue / float64(len(orders))
	}

	prodList := make([]struct {
		Title   string
		Qty     int
		Revenue float64
	}, 0, len(productAgg))
	for _, v := range productAgg {
		prodList = append(prodList, v)
	}
	sort.Slice(prodList, func(i, j int) bool {
		if prodList[i].Qty == prodList[j].Qty {
			return prodList[i].Revenue > prodList[j].Revenue
		}
		return prodList[i].Qty > prodList[j].Qty
	})
	if len(prodList) > 25 {
		prodList = prodList[:25]
	}

	dayKeys := make([]string, 0, len(dayRevenue))
	for k := range dayRevenue {
		dayKeys = append(dayKeys, k)
	}
	sort.Strings(dayKeys)
	daySeries := []struct {
		Day     string
		Revenue float64
		Orders  int
	}{}
	for _, k := range dayKeys {
		v := dayRevenue[k]
		daySeries = append(daySeries, struct {
			Day     string
			Revenue float64
			Orders  int
		}{Day: k, Revenue: v.Revenue, Orders: v.Orders})
	}

	if strings.ToLower(q.Get("format")) == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=ventas_%s_%s.csv", from.Format(layoutIn), to.Format(layoutIn)))
		fmt.Fprintln(w, "order_id,created_at,status,mp_status,total,shipping_method,shipping_cost,province")
		for _, o := range orders {
			fmt.Fprintf(w, "%s,%s,%s,%s,%.2f,%s,%.2f,%s\n", o.ID, o.CreatedAt.Format(time.RFC3339), o.Status, o.MPStatus, o.Total, o.ShippingMethod, o.ShippingCost, strings.ReplaceAll(o.Province, ",", " "))
		}
		return
	}

	data := map[string]any{
		"From":                 from.Format(layoutIn),
		"To":                   to.Format(layoutIn),
		"OrdersCount":          len(orders),
		"TotalRevenue":         totalRevenue,
		"ItemsRevenue":         itemsRevenue,
		"ShippingRevenue":      shippingRevenue,
		"AvgOrderValue":        avgOrderValue,
		"StatusCounts":         statusCounts,
		"MPStatusCounts":       mpStatusCounts,
		"ShippingMethodCounts": shippingMethodCounts,
		"ProvinceCounts":       provinceCounts,
		"TopProducts":          prodList,
		"DailySeries":          daySeries,
		"AdminToken":           s.readAdminToken(r),
	}

	s.render(w, layout, data)
}

func (s *Server) handleAdminAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if s.isAdminSession(r) {
			http.Redirect(w, r, "/admin/products", 302)
			return
		}
		data := map[string]any{}
		s.render(w, "admin_auth.html", data)
		return
	}
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "form", 400)
			return
		}
		user := strings.TrimSpace(r.FormValue("user"))
		pass := strings.TrimSpace(r.FormValue("pass"))
		cfgUser := os.Getenv("ADMIN_USER")
		cfgPass := os.Getenv("ADMIN_PASS")
		if cfgUser == "" {
			cfgUser = "admin"
		}
		if cfgPass == "" {
			cfgPass = "admin123"
		}
		if user != cfgUser || pass != cfgPass {
			http.Error(w, "credenciales", 401)
			return
		}
		email := user + "@local"
		if _, ok := s.adminAllowed[email]; !ok {
			if len(s.adminAllowed) == 0 {
				s.adminAllowed[email] = struct{}{}
			} else {
				for k := range s.adminAllowed {
					email = k
					break
				}
			}
		}
		tok, _, err := s.issueAdminToken(email, 6*time.Hour)
		if err != nil {
			http.Error(w, "token", 500)
			return
		}
		secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
		http.SetCookie(w, &http.Cookie{Name: "admin_token", Value: tok, Path: "/", MaxAge: 60 * 60 * 6, HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode})
		http.Redirect(w, r, "/admin/products", 302)
		return
	}
	http.Error(w, "method", 405)
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{Name: "admin_token", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/admin/auth", 302)
}

func (s *Server) isAdminSession(r *http.Request) bool {
	if tok := s.readAdminToken(r); tok != "" {
		if _, err := s.verifyAdminToken(tok); err == nil {
			return true
		}
	}
	return false
}

func (s *Server) readAdminToken(r *http.Request) string {
	c, err := r.Cookie("admin_token")
	if err != nil || c.Value == "" {
		return ""
	}
	return c.Value
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		tok := strings.TrimSpace(auth[7:])
		if _, err := s.verifyAdminToken(tok); err == nil {
			return true
		}
	}

	if tok := s.readAdminToken(r); tok != "" {
		if _, err := s.verifyAdminToken(tok); err == nil {
			return true
		}
	}
	http.Error(w, "unauthorized", 401)
	return false
}

func sendOrderEmail(o *domain.Order, success bool) error {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASS")
	to := os.Getenv("ORDER_NOTIFY_EMAIL")
	if to == "" {
		to = "ventas@celusfera.com.ar"
	}
	if host == "" || port == "" || user == "" || pass == "" {
		log.Warn().Msg("SMTP no configurado, se omite envío de email")
		return nil
	}
	addr := host + ":" + port
	statusTxt := "PAGO FALLIDO"
	if success {
		statusTxt = "PAGO APROBADO"
	}
	var buf bytes.Buffer
	_, _ = fmt.Fprintf(&buf, "Subject: Nueva orden %s #%s\r\n", statusTxt, o.ID.String())
	_, _ = fmt.Fprintf(&buf, "From: %s\r\n", user)
	_, _ = fmt.Fprintf(&buf, "To: %s\r\n", to)
	buf.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n")
	_, _ = fmt.Fprintf(&buf, "Estado: %s\n", statusTxt)
	_, _ = fmt.Fprintf(&buf, "Orden: %s\n", o.ID)
	_, _ = fmt.Fprintf(&buf, "Nombre: %s\nEmail: %s\nTel: %s\nDNI: %s\n", o.Name, o.Email, o.Phone, o.DNI)
	if o.ShippingMethod == "envio" || o.ShippingMethod == "cadete" {
		_, _ = fmt.Fprintf(&buf, "Envío (%s) a: %s (%s) CP:%s\n", o.ShippingMethod, o.Address, o.Province, o.PostalCode)
	} else {
		buf.WriteString("Retiro en local\n")
	}
	buf.WriteString("Items:\n")
	for _, it := range o.Items {
		col := formatColorES(it.Color)
		if col != "" {
			_, _ = fmt.Fprintf(&buf, "- %s x%d $%.2f Color: %s\n", it.Title, it.Qty, it.UnitPrice, col)
		} else {
			_, _ = fmt.Fprintf(&buf, "- %s x%d $%.2f\n", it.Title, it.Qty, it.UnitPrice)
		}
	}
	_, _ = fmt.Fprintf(&buf, "Total: $%.2f (Envío: $%.2f)\n", o.Total, o.ShippingCost)
	auth := smtp.PlainAuth("", user, pass, host)
	if err := smtp.SendMail(addr, auth, user, []string{to}, buf.Bytes()); err != nil {
		log.Error().Err(err).Msg("email send")
		return err
	}
	return nil
}

func sendOrderTelegram(o *domain.Order, success bool) error {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	// Soportar múltiples IDs: TELEGRAM_CHAT_IDS (coma-separado) o fallback TELEGRAM_CHAT_ID
	rawIDs := os.Getenv("TELEGRAM_CHAT_IDS")
	if strings.TrimSpace(rawIDs) == "" {
		rawIDs = os.Getenv("TELEGRAM_CHAT_ID")
	}
	if token == "" || strings.TrimSpace(rawIDs) == "" {
		return fmt.Errorf("telegram vars faltantes")
	}
	statusTxt := "PAGO FALLIDO"
	if success {
		statusTxt = "PAGO APROBADO"
	}
	var b strings.Builder
	b.WriteString("Orden ")
	b.WriteString(o.ID.String())
	b.WriteString(" - ")
	b.WriteString(statusTxt)
	b.WriteString("\n")
	fmt.Fprintf(&b, "Nombre: %s\nEmail: %s\nTel: %s\nDNI: %s\n", o.Name, o.Email, o.Phone, o.DNI)
	if o.ShippingMethod == "envio" || o.ShippingMethod == "cadete" {
		fmt.Fprintf(&b, "Envío (%s) a: %s (%s %s) CP:%s\n", o.ShippingMethod, o.Address, o.Province, o.ShippingMethod, o.PostalCode)
	} else {
		b.WriteString("Retiro en local\n")
	}
	b.WriteString("Items:\n")
	for _, it := range o.Items {
		col := formatColorES(it.Color)
		if col != "" {
			fmt.Fprintf(&b, "- %s x%d — $%.2f  Color: %s\n", it.Title, it.Qty, it.UnitPrice, col)
		} else {
			fmt.Fprintf(&b, "- %s x%d — $%.2f\n", it.Title, it.Qty, it.UnitPrice)
		}
	}
	fmt.Fprintf(&b, "Total: $%.2f (Envio: $%.2f)\n", o.Total, o.ShippingCost)
	apiURL := "https://api.telegram.org/bot" + token + "/sendMessage"
	// Separar por coma y enviar a cada destino
	ids := []string{}
	for _, part := range strings.Split(rawIDs, ",") {
		id := strings.TrimSpace(part)
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return fmt.Errorf("telegram chat ids vacios")
	}
	var lastErr error
	for _, id := range ids {
		form := url.Values{}
		form.Set("chat_id", id)
		form.Set("text", b.String())
		form.Set("disable_web_page_preview", "1")
		resp, err := http.PostForm(apiURL, form)
		if err != nil {
			lastErr = err
			continue
		}
		func() {
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				lastErr = fmt.Errorf("telegram status %d: %s", resp.StatusCode, string(body))
			}
		}()
	}
	return lastErr
}

func sendOrderNotify(o *domain.Order, success bool) {
	if err := sendOrderTelegram(o, success); err != nil {
		log.Warn().Err(err).Msg("telegram notif fallo")
		if os.Getenv("SMTP_HOST") != "" {
			_ = sendOrderEmail(o, success)
		}
	}
}

type sessionUser struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func writeUserSession(w http.ResponseWriter, u *sessionUser) {
	if u == nil {
		http.SetCookie(w, &http.Cookie{Name: "sess", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
		return
	}
	b, _ := json.Marshal(u)
	h := hmac.New(sha256.New, secretKey())
	h.Write(b)
	sig := base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	val := sig + "." + base64.RawURLEncoding.EncodeToString(b)

	http.SetCookie(w, &http.Cookie{Name: "sess", Value: val, Path: "/", MaxAge: 60 * 60 * 24 * 7, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
}

func readUserSession(w http.ResponseWriter, r *http.Request) *sessionUser {
	if r == nil {
		return nil
	}
	c, err := r.Cookie("sess")
	if err != nil || c.Value == "" {
		return nil
	}
	parts := strings.SplitN(c.Value, ".", 2)
	if len(parts) != 2 {
		return nil
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[0])
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	h := hmac.New(sha256.New, secretKey())
	h.Write(payload)
	if !hmac.Equal(sig, h.Sum(nil)) {
		return nil
	}
	var u sessionUser
	if err := json.Unmarshal(payload, &u); err != nil {
		return nil
	}
	if u.Email == "" {
		return nil
	}
	return &u
}

func (s *Server) handleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	if s.oauthCfg == nil {
		http.Error(w, "oauth no configurado", 500)
		return
	}
	state := uuid.New().String()
	http.SetCookie(w, &http.Cookie{Name: "oauth_state", Value: state, Path: "/", MaxAge: 300, HttpOnly: true, Secure: false})
	loginURL := s.oauthCfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
	http.Redirect(w, r, loginURL, 302)
}

func (s *Server) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if s.oauthCfg == nil {
		http.Error(w, "oauth no configurado", 500)
		return
	}
	q := r.URL.Query()
	state := q.Get("state")
	code := q.Get("code")
	c, _ := r.Cookie("oauth_state")
	if c == nil || c.Value == "" || c.Value != state {
		http.Error(w, "state", 400)
		return
	}
	tok, err := s.oauthCfg.Exchange(r.Context(), code)
	if err != nil {
		log.Error().Err(err).Msg("exchange oauth")
		http.Error(w, "oauth", 400)
		return
	}
	client := s.oauthCfg.Client(r.Context(), tok)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil || resp.StatusCode != 200 {
		log.Error().Err(err).Msg("userinfo")
		http.Error(w, "userinfo", 400)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var info struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	_ = json.Unmarshal(body, &info)
	if info.Email == "" {
		http.Error(w, "email", 400)
		return
	}
	if s.customers != nil {
		if cust, err := s.customers.FindByEmail(r.Context(), info.Email); err != nil && err == domain.ErrNotFound {
			_ = s.customers.Save(r.Context(), &domain.Customer{ID: uuid.New(), Email: info.Email, Name: info.Name})
		} else if cust == nil && err == nil {
			_ = s.customers.Save(r.Context(), &domain.Customer{ID: uuid.New(), Email: info.Email, Name: info.Name})
		}
	}
	writeUserSession(w, &sessionUser{Email: info.Email, Name: info.Name})
	http.Redirect(w, r, "/", 302)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	writeUserSession(w, nil)
	http.Redirect(w, r, "/", 302)
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	cfgKey := os.Getenv("ADMIN_API_KEY")
	if cfgKey == "" {
		log.Error().Msg("ADMIN_API_KEY faltante")
		http.Error(w, "config", 500)
		return
	}
	apiKey := r.Header.Get("X-Admin-Key")
	if apiKey == "" || !secureCompare(apiKey, cfgKey) {
		http.Error(w, "unauthorized", 401)
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" && len(s.adminAllowed) == 1 {
		for k := range s.adminAllowed {
			email = k
		}
	}
	if _, ok := s.adminAllowed[email]; !ok {
		http.Error(w, "forbidden", 403)
		return
	}
	tok, exp, err := s.issueAdminToken(email, 30*time.Minute)
	if err != nil {
		http.Error(w, "token", 500)
		return
	}
	writeJSON(w, 200, map[string]any{"token": tok, "exp": exp.Unix(), "email": email})
}

func (s *Server) issueAdminToken(email string, dur time.Duration) (string, time.Time, error) {
	head := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	exp := time.Now().Add(dur)
	claims := map[string]any{"sub": email, "email": email, "role": "admin", "exp": exp.Unix(), "iat": time.Now().Unix(), "iss": "tienda3d"}
	b, _ := json.Marshal(claims)
	pay := base64.RawURLEncoding.EncodeToString(b)
	unsigned := head + "." + pay
	h := hmac.New(sha256.New, s.adminSecret)
	h.Write([]byte(unsigned))
	sig := base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	return unsigned + "." + sig, exp, nil
}

func (s *Server) verifyAdminToken(tok string) (string, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("formato")
	}
	unsigned := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("sig")
	}
	h := hmac.New(sha256.New, s.adminSecret)
	h.Write([]byte(unsigned))
	if !hmac.Equal(sig, h.Sum(nil)) {
		return "", fmt.Errorf("firma")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("payload")
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return "", fmt.Errorf("json")
	}
	role, _ := m["role"].(string)
	email, _ := m["email"].(string)
	expF, _ := m["exp"].(float64)
	if role != "admin" || email == "" {
		return "", fmt.Errorf("claims")
	}
	if time.Now().Unix() > int64(expF) {
		return "", fmt.Errorf("exp")
	}
	if _, ok := s.adminAllowed[strings.ToLower(email)]; !ok {
		return "", fmt.Errorf("not allowed")
	}
	return email, nil
}

func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func (s *Server) handleAdminScan(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminSession(r) {
		http.Error(w, "unauthorized", 401)
		return
	}
	q := r.URL.Query()
	ean := strings.TrimSpace(q.Get("ean"))
	sku := strings.TrimSpace(q.Get("sku"))
	if ean == "" && sku == "" {
		http.Error(w, "param", 400)
		return
	}
	if ean != "" {
		p, v, err := s.products.SearchByEAN(r.Context(), ean)
		if err != nil || v == nil || p == nil {
			http.Error(w, "not found", 404)
			return
		}
		writeJSON(w, 200, map[string]any{"product": p, "variant": v})
		return
	}
	p, v, err := s.products.SearchBySKU(r.Context(), sku)
	if err != nil || v == nil || p == nil {
		http.Error(w, "not found", 404)
		return
	}
	writeJSON(w, 200, map[string]any{"product": p, "variant": v})
}

func (s *Server) handleAdminImportCSV(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "multipart", 400)
		return
	}
	fh := r.MultipartForm.File["file"]
	if len(fh) == 0 {
		http.Error(w, "file", 400)
		return
	}
	f, err := fh[0].Open()
	if err != nil {
		http.Error(w, "file", 400)
		return
	}
	defer f.Close()

	pricesText := strings.TrimSpace(r.FormValue("prices_text"))
	fxRate, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("fx_rate")), 64)
	defaultMargin, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("default_margin_pct")), 64)
	useOpenAI := strings.TrimSpace(r.FormValue("use_openai")) == "true"

	if fxRate <= 0 {
		http.Error(w, "fx", 400)
		return
	}

	data, _ := io.ReadAll(io.LimitReader(f, 48<<20))
	if len(data) == 0 {
		http.Error(w, "empty", 400)
		return
	}

	var createdP, updatedP, createdV, updatedV, unmatched int

	if useOpenAI {
		// Usar OpenAI para normalizar (con timeout de 5 minutos para procesar lotes)
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		normalized, err := s.normalizeWithOpenAI(ctx, data, pricesText)
		if err != nil {
			log.Error().Err(err).Msg("error usando OpenAI")
			writeJSON(w, 500, map[string]any{
				"error":   "openai_error",
				"message": err.Error(),
			})
			return
		}
		createdP, updatedP, createdV, updatedV, unmatched = s.importFromNormalizedData(r, normalized, fxRate, defaultMargin)
	} else {
		// Método tradicional
		priceMap := parseUSDPrices(pricesText)
		createdP, updatedP, createdV, updatedV, unmatched = s.importFromXLSXCombined(r, data, priceMap, pricesText, fxRate, defaultMargin)
	}

	// devolver también resumen del reporte
	resp := map[string]any{"created_products": createdP, "updated_products": updatedP, "created_variants": createdV, "updated_variants": updatedV, "unmatched": unmatched}
	if s.lastImport != nil {
		resp["report"] = map[string]any{
			"timestamp":       s.lastImport.Timestamp.Format(time.RFC3339),
			"unmatched_items": s.lastImport.UnmatchedItems,
			"errors":          s.lastImport.Errors,
		}
	}
	writeJSON(w, 200, resp)
}

// parseUSDPrices convierte el texto en un mapa nombre base -> precio USD
func parseUSDPrices(text string) map[string]float64 {
	res := map[string]float64{}
	if strings.TrimSpace(text) == "" {
		return res
	}
	lines := strings.Split(text, "\n")
	priceRe := regexp.MustCompile(`\$\s*([0-9][0-9.,]*)`)
	for _, ln := range lines {
		l := strings.TrimSpace(ln)
		if l == "" {
			continue
		}
		// Ignorar líneas que indiquen sin stock/precio
		if strings.Contains(strings.ToLower(l), "sin stock") || strings.Contains(strings.ToLower(l), "sin precio") {
			continue
		}
		m := priceRe.FindStringSubmatch(l)
		if len(m) == 2 {
			usdStr := strings.ReplaceAll(m[1], ",", "")
			usd, _ := strconv.ParseFloat(usdStr, 64)
			name := strings.TrimSpace(priceRe.ReplaceAllString(l, ""))
			name = strings.Trim(name, "-:\t ")
			key := normalizeBaseKey(name)
			if key != "" && usd > 0 {
				res[key] = usd
			}
		}
	}
	return res
}

// importFromXLSXCombined procesa el XLSX de colores y combina con el mapa de precios
type ImportReport struct {
	CreatedProducts     int
	UpdatedProducts     int
	CreatedVariants     int
	UpdatedVariants     int
	UnmatchedPrices     int
	UnmatchedItems      map[string]int    // baseKey -> cantidad de veces sin precio (agrupado)
	UnmatchedReasons    map[string]string // baseKey -> razón (sin_stock, no_encontrado, etc)
	Errors              []string
	Timestamp           time.Time
	CreatedProductSlugs []string
	UpdatedProductSlugs []string
	CreatedVariantKeys  []string // slug:color
	UpdatedVariantKeys  []string
}

func (s *Server) importFromXLSXCombined(r *http.Request, data []byte, priceUSD map[string]float64, pricesText string, fxRate float64, defaultMargin float64) (int, int, int, int, int) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return 0, 0, 0, 0, 0
	}
	defer f.Close()

	createdP, updatedP := 0, 0
	createdV, updatedV := 0, 0
	unmatched := 0
	rep := &ImportReport{
		Timestamp:        time.Now(),
		UnmatchedItems:   make(map[string]int),    // mapa para agrupar duplicados
		UnmatchedReasons: make(map[string]string), // razón de cada uno
	}

	sheets := f.GetSheetList()
	for _, sh := range sheets {
		rows, err := f.GetRows(sh)
		if err != nil || len(rows) == 0 {
			continue
		}
		category := ""
		for _, row := range rows {
			if len(row) == 0 {
				continue
			}
			// Columnas esperadas según muestra: B = nombre, C = color (a veces), D/E = stock
			name := ""
			color := ""
			stockStr := ""
			if len(row) > 1 {
				name = strings.TrimSpace(row[1])
			}
			if len(row) > 2 {
				color = strings.TrimSpace(row[2])
			}
			if len(row) > 3 {
				stockStr = strings.TrimSpace(row[3])
			}

			if isSectionTitle(name) {
				category = strings.ToLower(strings.TrimSpace(name))
				continue
			}
			if name == "" {
				continue
			}
			// Ignorar filas de encabezado tipo "Color"
			if strings.EqualFold(strings.TrimSpace(row[2]), "Color") {
				continue
			}

			baseKey := normalizeBaseKey(removeColorFromName(name))
			if color == "" {
				color = inferColorFromName(name)
			}
			stock := mapStock(stockStr)

			usd := priceUSD[baseKey]
			matchMethod := "exacto"
			if usd <= 0 {
				if alt := matchUSDPrice(priceUSD, baseKey); alt > 0 {
					usd = alt
					matchMethod = "fuzzy"
				}
			}
			if usd <= 0 {
				unmatched++
				rep.UnmatchedItems[baseKey]++ // incrementar contador de este producto

				// Determinar razón específica
				reason := detectUnmatchReason(baseKey, pricesText)
				if _, exists := rep.UnmatchedReasons[baseKey]; !exists {
					rep.UnmatchedReasons[baseKey] = reason
				}

				log.Debug().Str("producto", baseKey).Str("razon", reason).Str("nombre_original", name).Msg("sin precio")
				continue
			}

			// Log exitoso con método de match
			if matchMethod == "fuzzy" {
				log.Debug().Str("producto", baseKey).Str("metodo", matchMethod).Float64("usd", usd).Msg("match encontrado")
			}
			gross := usd * fxRate
			margin := defaultMargin
			price := gross * (1.0 + margin/100.0)

			brand, model := inferBrandModel(baseKey)
			p, _ := s.products.GetBySlug(r.Context(), slugify(baseKey))
			if p == nil {
				p = &domain.Product{Name: baseKey, Category: category, Brand: brand, Model: model, GrossPrice: gross, MarginPct: margin, BasePrice: price}
				_ = s.products.Create(r.Context(), p)
				createdP++
				if p.Slug != "" {
					rep.CreatedProductSlugs = append(rep.CreatedProductSlugs, p.Slug)
				}
			} else {
				p.GrossPrice = gross
				p.MarginPct = margin
				p.BasePrice = price
				_ = s.products.Create(r.Context(), p)
				updatedP++
				if p.Slug != "" {
					rep.UpdatedProductSlugs = append(rep.UpdatedProductSlugs, p.Slug)
				}
			}

			// Variante/color
			// buscar variante existente por color
			var existing *domain.Variant
			if p != nil {
				vs, _ := s.products.ListVariants(r.Context(), p.ID)
				for i := range vs {
					if strings.EqualFold(strings.TrimSpace(vs[i].Color), strings.TrimSpace(color)) {
						existing = &vs[i]
						break
					}
				}
			}
			if existing == nil {
				v := &domain.Variant{ProductID: p.ID, Color: color, Stock: stock}
				_ = s.products.CreateVariant(r.Context(), v)
				createdV++
				if p.Slug != "" {
					rep.CreatedVariantKeys = append(rep.CreatedVariantKeys, p.Slug+":"+strings.TrimSpace(color))
				}
			} else {
				// Preservar stock existente si XLSX no trae dato (stockStr vacío) o si el valor sería negativo
				if strings.TrimSpace(stockStr) != "" && stock >= 0 {
					existing.Stock = stock
				}
				_ = s.products.UpdateVariant(r.Context(), existing)
				updatedV++
				if p.Slug != "" {
					rep.UpdatedVariantKeys = append(rep.UpdatedVariantKeys, p.Slug+":"+strings.TrimSpace(color))
				}
			}
		}
	}
	rep.CreatedProducts = createdP
	rep.UpdatedProducts = updatedP
	rep.CreatedVariants = createdV
	rep.UpdatedVariants = updatedV
	rep.UnmatchedPrices = unmatched
	s.lastImport = rep

	// Log resumen
	total := createdP + updatedP
	if total > 0 {
		matchRate := float64(total) / float64(total+unmatched) * 100
		log.Info().
			Int("creados", createdP).
			Int("actualizados", updatedP).
			Int("sin_precio", unmatched).
			Float64("tasa_match", matchRate).
			Msg("importación tradicional completada")
	}

	return createdP, updatedP, createdV, updatedV, unmatched
}

func isSectionTitle(s string) bool {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return false
	}
	// heurística: títulos suelen estar en mayúsculas y sin números
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return false
		}
	}
	return true
}

func removeColorFromName(s string) string {
	// Primero quitar colores agrupados entre paréntesis
	s = regexp.MustCompile(`\s*\([^)]*\)\s*`).ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)

	// Luego quitar el color al final si es reconocido
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return s
	}

	// Lista expandida de colores comunes
	colors := []string{
		"negro", "black", "blanco", "white", "azul", "blue", "rosa", "pink",
		"amarillo", "yellow", "verde", "green", "silver", "starlight", "midnight",
		"purple", "púrpura", "morado", "space", "gray", "grey", "gris", "oro", "gold",
		"red", "rojo", "orange", "naranja", "coral", "arena", "sand", "cosmic",
		"deep", "pearl", "perlado", "oscuro", "dark", "light", "claro",
	}

	// Verificar última palabra
	last := strings.ToLower(parts[len(parts)-1])
	for _, c := range colors {
		if last == c || strings.Contains(last, c) {
			return strings.TrimSpace(strings.Join(parts[:len(parts)-1], " "))
		}
	}

	// Verificar últimas 2 palabras (ej: "Azul Oscuro", "Deep Blue")
	if len(parts) >= 2 {
		lastTwo := strings.ToLower(parts[len(parts)-2] + " " + parts[len(parts)-1])
		for _, c := range colors {
			if strings.Contains(lastTwo, c) {
				return strings.TrimSpace(strings.Join(parts[:len(parts)-2], " "))
			}
		}
	}

	return s
}

func inferColorFromName(s string) string {
	colors := []string{"Negro", "Black", "Blanco", "White", "Azul", "Blue", "Rosa", "Pink", "Amarillo", "Yellow", "Verde", "Green", "Silver", "Starlight", "Midnight", "Purple", "Space Gray", "Space Black", "Natural", "Sage Green", "Mist Blue", "Lavender"}
	ls := strings.ToLower(s)
	for _, c := range colors {
		if strings.Contains(ls, strings.ToLower(c)) {
			return c
		}
	}
	return ""
}

func normalizeBaseKey(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\t", " ")

	// Quitar colores agrupados entre paréntesis (ej: "(Negro,Blanco,Azul...)")
	s = regexp.MustCompile(`\s*\([^)]*\)\s*`).ReplaceAllString(s, " ")

	// Limpiar espacios múltiples
	s = strings.Join(strings.Fields(s), " ")

	// normalizar capacidades individuales sin espacio (ej: 256GB → 256 GB)
	re := regexp.MustCompile(`(\d+)(GB|TB|MHz|mm|")`)
	s = re.ReplaceAllString(s, "$1 $2")

	// normalizar separadores "x/y" en RAM/Storage (ej: 12/512 GB → 12/512 GB)
	re2 := regexp.MustCompile(`(\d+)/(\d+)\s*(GB|TB)`)
	s = re2.ReplaceAllString(s, "$1/$2 $3")

	// Normalizar variaciones comunes
	s = strings.ReplaceAll(s, "  ", " ")

	// limpiar espacios múltiples otra vez
	s = strings.Join(strings.Fields(s), " ")

	return s
}

func slugify(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "-"))
}

func inferBrandModel(s string) (string, string) {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return "", ""
	}
	brand := parts[0]
	model := strings.TrimSpace(strings.Join(parts[1:], " "))
	return brand, model
}

// matchUSDPrice intenta encontrar un precio en el mapa con matching fuzzy mejorado
func matchUSDPrice(m map[string]float64, baseKey string) float64 {
	// Intento 1: Match exacto
	if v, ok := m[baseKey]; ok {
		return v
	}

	// Función de normalización agresiva
	normalize := func(s string) string {
		s = strings.ToLower(s)
		// Quitar paréntesis y contenido
		s = regexp.MustCompile(`\s*\([^)]*\)`).ReplaceAllString(s, "")
		// Quitar sufijos comunes (orden importa: más específicos primero)
		suffixes := []string{" 5g ds", " 4g ds", " 5g", " 4g", " ds", " wifi", " wi-fi", " lte"}
		for _, suf := range suffixes {
			s = strings.TrimSuffix(s, suf)
		}
		// Limpiar caracteres especiales
		s = strings.ReplaceAll(s, "\u00a0", " ")
		s = strings.ReplaceAll(s, "\"", "")
		s = strings.ReplaceAll(s, "+", " ")
		// Normalizar espacios
		s = strings.Join(strings.Fields(s), " ")
		return strings.TrimSpace(s)
	}

	baseNorm := normalize(baseKey)

	// Intento 2: Match normalizado
	for k, v := range m {
		if normalize(k) == baseNorm {
			return v
		}
	}

	// Intento 3: Match parcial (contiene)
	// Útil para "Samsung S25+" que puede venir como "Samsung S25+ 12/256 GB"
	for k, v := range m {
		kNorm := normalize(k)
		// Si la clave normalizada está contenida en baseNorm o viceversa
		if len(kNorm) > 10 && len(baseNorm) > 10 { // Solo para nombres razonablemente largos
			if strings.Contains(baseNorm, kNorm) || strings.Contains(kNorm, baseNorm) {
				return v
			}
		}
	}

	return 0
}

func (s *Server) handleAdminExportCSV(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=products.csv")
	fmt.Fprintln(w, "slug,name,category,brand,model,short_desc,variant_sku,variant_ean,attr_color,attr_capacidad,price_net,stock,image_url")
	page := 1
	for {
		list, total, err := s.products.List(r.Context(), domain.ProductFilter{Page: page, PageSize: 200})
		if err != nil || len(list) == 0 {
			break
		}
		for _, p := range list {
			vars, _ := s.products.ListVariants(r.Context(), p.ID)
			if len(vars) == 0 {
				fmt.Fprintf(w, "%s,%s,%s,%s,%s,%q,,,,,,\n", p.Slug, p.Name, p.Category, p.Brand, p.Model, p.ShortDesc)
			}
			for _, v := range vars {
				color := strings.TrimSpace(v.Color)
				if color == "" && v.Attributes != nil {
					color = v.Attributes["color"]
				}
				cap := ""
				if v.Attributes != nil {
					cap = v.Attributes["capacidad"]
				}
				fmt.Fprintf(w, "%s,%s,%s,%s,%s,%q,%s,%s,%s,%s,%.2f,%d,%s\n",
					p.Slug, p.Name, p.Category, p.Brand, p.Model, p.ShortDesc,
					v.SKU, v.EAN, color, cap, v.Price, v.Stock, v.ImageURL)
			}
		}
		if page*200 >= int(total) {
			break
		}
		page++
	}
}

func mapStock(s string) int {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0
	}
	if strings.Contains(s, "sin") {
		return 0
	}
	if strings.Contains(s, "bajo") {
		return 2
	}
	return 10
}

// detectUnmatchReason determina por qué un producto no tiene precio asignado
func detectUnmatchReason(baseKey string, pricesText string) string {
	baseKeyLower := strings.ToLower(baseKey)
	pricesTextLower := strings.ToLower(pricesText)

	// Buscar si el producto aparece en el texto de precios
	if !strings.Contains(pricesTextLower, baseKeyLower) {
		// Buscar partes del nombre (ej: "iPhone 17" sin la capacidad)
		parts := strings.Fields(baseKey)
		if len(parts) >= 2 {
			partialName := strings.ToLower(parts[0] + " " + parts[1])
			if strings.Contains(pricesTextLower, partialName) {
				return "formato_diferente"
			}
		}
		return "no_encontrado"
	}

	// El producto aparece en el texto, verificar si dice "Sin Stock"
	idx := strings.Index(pricesTextLower, baseKeyLower)
	if idx >= 0 {
		// Buscar "sin stock" en los siguientes 150 caracteres (misma línea aproximadamente)
		snippet := pricesTextLower[idx:]
		if len(snippet) > 150 {
			snippet = snippet[:150]
		}
		if strings.Contains(snippet, "sin stock") {
			return "sin_stock"
		}
	}

	// Aparece pero no tiene precio válido
	return "precio_invalido"
}

// normalizeWithOpenAI usa la API de OpenAI para normalizar y matchear productos en lotes
func (s *Server) normalizeWithOpenAI(ctx context.Context, xlsxData []byte, pricesText string) (map[string]NormalizedProduct, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, errors.New("OPENAI_API_KEY no configurada")
	}

	// Extraer productos del XLSX (agrupados por nombre base para reducir datos)
	f, err := excelize.OpenReader(bytes.NewReader(xlsxData))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Agrupar productos por nombre base con sus colores
	productGroups := make(map[string][]string) // nombre -> []colores
	sheets := f.GetSheetList()
	for _, sh := range sheets {
		rows, err := f.GetRows(sh)
		if err != nil || len(rows) == 0 {
			continue
		}
		for _, row := range rows {
			if len(row) > 1 {
				name := strings.TrimSpace(row[1])
				color := ""
				if len(row) > 2 {
					color = strings.TrimSpace(row[2])
				}
				if name != "" && !isSectionTitle(name) && !strings.EqualFold(color, "Color") {
					if _, exists := productGroups[name]; !exists {
						productGroups[name] = []string{}
					}
					if color != "" && color != name {
						productGroups[name] = append(productGroups[name], color)
					}
				}
			}
		}
	}

	// Construir lista compacta de productos
	xlsxProducts := []string{}
	for name, colors := range productGroups {
		if len(colors) > 0 {
			// Limitar a primeros 3 colores para reducir tamaño
			uniqueColors := make(map[string]bool)
			compactColors := []string{}
			for _, c := range colors {
				if !uniqueColors[c] && len(compactColors) < 3 {
					uniqueColors[c] = true
					compactColors = append(compactColors, c)
				}
			}
			if len(colors) > 3 {
				xlsxProducts = append(xlsxProducts, name+" ("+strings.Join(compactColors, ",")+"...)")
			} else {
				xlsxProducts = append(xlsxProducts, name+" ("+strings.Join(compactColors, ",")+")")
			}
		} else {
			xlsxProducts = append(xlsxProducts, name)
		}
	}

	// Dividir en lotes de 50 productos para máxima velocidad
	const batchSize = 50
	totalBatches := (len(xlsxProducts) + batchSize - 1) / batchSize

	log.Info().Int("total_productos", len(xlsxProducts)).Int("lotes", totalBatches).Msg("procesando con OpenAI en lotes")

	allProducts := make(map[string]NormalizedProduct)
	client := openai.NewClient(apiKey)

	for batchNum := 0; batchNum < totalBatches; batchNum++ {
		start := batchNum * batchSize
		end := start + batchSize
		if end > len(xlsxProducts) {
			end = len(xlsxProducts)
		}

		batchProducts := xlsxProducts[start:end]
		log.Info().Int("lote", batchNum+1).Int("total", totalBatches).Int("productos", len(batchProducts)).Msg("procesando lote")

		// Mostrar primeros 3 productos del lote para debug
		if len(batchProducts) > 0 {
			sampleSize := 3
			if len(batchProducts) < sampleSize {
				sampleSize = len(batchProducts)
			}
			log.Info().Strs("muestra", batchProducts[:sampleSize]).Msg("productos enviados (muestra)")
		}

		// Construir prompt optimizado pero claro
		prompt := fmt.Sprintf(`Matchea estos productos con sus precios USD.

PRECIOS:
%s

PRODUCTOS A MATCHEAR:
%s

Devuelve JSON con TODOS los productos matcheados:
{"productos":[{"nombre_base":"nombre del producto","precio_usd":precio_numero,"variantes":[{"color":"nombre_color","stock":"disponible"}]}]}

Importante:
- Si un producto dice "Sin Stock" en precios → precio_usd: 0
- Ignora diferencias menores: "256GB" = "256 GB", "5G DS" = "5G"
- Si NO hay precio → precio_usd: 0
- Incluye TODOS los productos en la respuesta
`, pricesText, strings.Join(batchProducts, "\n"))

		// Timeout de 60 segundos por lote (margen para listas largas de precios)
		batchCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		resp, err := client.CreateChatCompletion(batchCtx, openai.ChatCompletionRequest{
			Model: "gpt-4o-mini",
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: "Eres un experto en matchear productos. Devuelve SIEMPRE JSON válido con TODOS los productos que te envían.",
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Temperature: 0,
			MaxTokens:   8000, // Aumentar para permitir más productos en la respuesta
		})
		cancel()

		if err != nil {
			return nil, fmt.Errorf("error en lote %d/%d: %w", batchNum+1, totalBatches, err)
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("respuesta vacía de OpenAI en lote %d/%d", batchNum+1, totalBatches)
		}

		// Parsear respuesta JSON del lote
		content := strings.TrimSpace(resp.Choices[0].Message.Content)
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)

		var result struct {
			Productos []NormalizedProduct `json:"productos"`
		}
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			log.Error().Str("content", content).Err(err).Int("lote", batchNum+1).Msg("error parseando respuesta de OpenAI")
			return nil, fmt.Errorf("error parseando JSON de OpenAI en lote %d/%d: %w", batchNum+1, totalBatches, err)
		}

		// Agregar productos del lote al mapa total
		for _, p := range result.Productos {
			allProducts[p.NombreBase] = p
		}

		// Warning si se procesaron menos productos de los esperados
		if len(result.Productos) < len(batchProducts) {
			log.Warn().
				Int("lote", batchNum+1).
				Int("enviados", len(batchProducts)).
				Int("procesados", len(result.Productos)).
				Int("faltantes", len(batchProducts)-len(result.Productos)).
				Msg("algunos productos no fueron matcheados por OpenAI")
		} else {
			log.Info().Int("lote", batchNum+1).Int("productos_procesados", len(result.Productos)).Msg("lote completado")
		}
	}

	log.Info().Int("total_productos_normalizados", len(allProducts)).Msg("normalización completada")
	return allProducts, nil
}

type NormalizedProduct struct {
	NombreBase string              `json:"nombre_base"`
	PrecioUSD  float64             `json:"precio_usd"`
	Variantes  []NormalizedVariant `json:"variantes"`
}

type NormalizedVariant struct {
	Color     string `json:"color"`
	Capacidad string `json:"capacidad"`
	Stock     string `json:"stock"`
}

// importFromNormalizedData procesa los datos normalizados por OpenAI
func (s *Server) importFromNormalizedData(r *http.Request, normalized map[string]NormalizedProduct, fxRate float64, defaultMargin float64) (int, int, int, int, int) {
	createdP, updatedP := 0, 0
	createdV, updatedV := 0, 0
	unmatched := 0
	rep := &ImportReport{
		Timestamp:        time.Now(),
		UnmatchedItems:   make(map[string]int),    // mapa para agrupar duplicados
		UnmatchedReasons: make(map[string]string), // razón de cada uno
	}

	for baseKey, normProd := range normalized {
		// Si no tiene precio, marcar como sin matchear
		if normProd.PrecioUSD <= 0 {
			unmatched++
			rep.UnmatchedItems[baseKey]++ // incrementar contador
			if _, exists := rep.UnmatchedReasons[baseKey]; !exists {
				rep.UnmatchedReasons[baseKey] = "openai_sin_precio"
			}
			continue
		}

		gross := normProd.PrecioUSD * fxRate
		margin := defaultMargin
		price := gross * (1.0 + margin/100.0)

		// Inferir brand y model del nombre base
		brand, model := inferBrandModel(baseKey)

		// Buscar o crear producto
		p, _ := s.products.GetBySlug(r.Context(), slugify(baseKey))
		if p == nil {
			p = &domain.Product{
				Name:       baseKey,
				Category:   "", // OpenAI podría incluir categoría si lo pedimos
				Brand:      brand,
				Model:      model,
				GrossPrice: gross,
				MarginPct:  margin,
				BasePrice:  price,
			}
			_ = s.products.Create(r.Context(), p)
			createdP++
			if p.Slug != "" {
				rep.CreatedProductSlugs = append(rep.CreatedProductSlugs, p.Slug)
			}
		} else {
			// Actualizar precios
			p.GrossPrice = gross
			p.MarginPct = margin
			p.BasePrice = price
			_ = s.products.Create(r.Context(), p)
			updatedP++
			if p.Slug != "" {
				rep.UpdatedProductSlugs = append(rep.UpdatedProductSlugs, p.Slug)
			}
		}

		// Procesar variantes
		for _, normVar := range normProd.Variantes {
			color := strings.TrimSpace(normVar.Color)
			if color == "" {
				color = "Default"
			}

			// Determinar stock
			stock := 10
			if strings.Contains(strings.ToLower(normVar.Stock), "sin") {
				stock = 0
			}

			// Buscar variante existente
			var existing *domain.Variant
			if p != nil {
				vs, _ := s.products.ListVariants(r.Context(), p.ID)
				for i := range vs {
					if strings.EqualFold(strings.TrimSpace(vs[i].Color), color) {
						existing = &vs[i]
						break
					}
				}
			}

			if existing == nil {
				// Crear nueva variante
				v := &domain.Variant{
					ProductID: p.ID,
					Color:     color,
					Stock:     stock,
				}
				_ = s.products.CreateVariant(r.Context(), v)
				createdV++
				if p.Slug != "" {
					rep.CreatedVariantKeys = append(rep.CreatedVariantKeys, p.Slug+":"+color)
				}
			} else {
				// Actualizar stock solo si viene dato válido
				if stock >= 0 {
					existing.Stock = stock
				}
				_ = s.products.UpdateVariant(r.Context(), existing)
				updatedV++
				if p.Slug != "" {
					rep.UpdatedVariantKeys = append(rep.UpdatedVariantKeys, p.Slug+":"+color)
				}
			}
		}
	}

	rep.CreatedProducts = createdP
	rep.UpdatedProducts = updatedP
	rep.CreatedVariants = createdV
	rep.UpdatedVariants = updatedV
	rep.UnmatchedPrices = unmatched
	s.lastImport = rep

	return createdP, updatedP, createdV, updatedV, unmatched
}
