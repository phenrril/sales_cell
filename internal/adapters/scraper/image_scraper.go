package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/rs/zerolog/log"
)

type ImageScraper struct {
	client *http.Client
}

func NewImageScraper() *ImageScraper {
	return &ImageScraper{
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

// SearchImages busca imágenes de un producto usando DuckDuckGo Images (más confiable que Google)
func (s *ImageScraper) SearchImages(ctx context.Context, productName, brand, model string, maxResults int) ([]string, error) {
	if maxResults <= 0 {
		maxResults = 6
	}
	if maxResults > 20 {
		maxResults = 20
	}

	// Construir query de búsqueda: nombre + marca + modelo + "smartphone"
	query := s.buildImageQuery(productName, brand, model)

	// Intentar DuckDuckGo Images primero (más confiable)
	images, err := s.searchDuckDuckGo(ctx, query, maxResults)
	if err == nil && len(images) > 0 {
		log.Info().Str("query", query).Int("found", len(images)).Msg("Imágenes encontradas en DuckDuckGo")
		return images, nil
	}

	log.Warn().Err(err).Msg("Error en DuckDuckGo, intentando Google Images")

	// Fallback a Google Images
	images, err = s.searchGoogleImages(ctx, query, maxResults)
	if err == nil && len(images) > 0 {
		log.Info().Str("query", query).Int("found", len(images)).Msg("Imágenes encontradas en Google")
		return images, nil
	}

	return nil, fmt.Errorf("no se encontraron imágenes: %w", err)
}

func (s *ImageScraper) buildImageQuery(productName, brand, model string) string {
	parts := []string{}

	// Normalizar marca
	if brand != "" {
		brand = strings.TrimSpace(brand)
		if strings.ToLower(brand) == "moto" {
			brand = "motorola"
		}
		parts = append(parts, brand)
	}

	// Agregar modelo
	if model != "" {
		parts = append(parts, strings.TrimSpace(model))
	}

	// Si no hay marca/modelo, usar el nombre del producto
	if len(parts) == 0 {
		parts = append(parts, productName)
	}

	// Agregar término de búsqueda para mejorar resultados
	parts = append(parts, "smartphone")

	return strings.Join(parts, " ")
}

// searchDuckDuckGo busca imágenes usando DuckDuckGo Images API (no oficial pero funciona)
func (s *ImageScraper) searchDuckDuckGo(ctx context.Context, query string, maxResults int) ([]string, error) {
	// DuckDuckGo Images usa una API no oficial pero estable
	searchURL := fmt.Sprintf("https://duckduckgo.com/?q=%s&iax=images&ia=images", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "es-ES,es;q=0.9,en;q=0.8")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// DuckDuckGo carga las imágenes dinámicamente, necesitamos extraer el token vqd
	vqdPattern := regexp.MustCompile(`vqd="([^"]+)"`)
	matches := vqdPattern.FindStringSubmatch(string(body))
	if len(matches) < 2 {
		return nil, fmt.Errorf("no se encontró token vqd")
	}
	vqd := matches[1]

	// Ahora hacer la búsqueda real de imágenes
	imageSearchURL := fmt.Sprintf("https://duckduckgo.com/i.js?q=%s&vqd=%s&o=json&p=1&s=0", url.QueryEscape(query), url.QueryEscape(vqd))

	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, imageSearchURL, nil)
	if err != nil {
		return nil, err
	}
	req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req2.Header.Set("Referer", searchURL)

	resp2, err := s.client.Do(req2)
	if err != nil {
		return nil, err
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", resp2.StatusCode)
	}

	var result struct {
		Results []struct {
			Image     string `json:"image"`
			Thumbnail string `json:"thumbnail"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp2.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decodificando JSON: %w", err)
	}

	images := []string{}
	minSize := 300 // Tamaño mínimo para que se vea bien

	for _, img := range result.Results {
		// Filtrar por tamaño mínimo
		if img.Width >= minSize && img.Height >= minSize {
			imageURL := img.Image
			if imageURL == "" {
				imageURL = img.Thumbnail
			}
			if imageURL != "" && strings.HasPrefix(imageURL, "http") {
				images = append(images, imageURL)
				if len(images) >= maxResults {
					break
				}
			}
		}
	}

	return images, nil
}

// searchGoogleImages busca imágenes usando Google Images (fallback)
func (s *ImageScraper) searchGoogleImages(ctx context.Context, query string, maxResults int) ([]string, error) {
	searchURL := fmt.Sprintf("https://www.google.com/search?tbm=isch&q=%s&safe=active", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "es-ES,es;q=0.9,en;q=0.8")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	images := []string{}

	// Google Images estructura: buscar en los divs con imágenes
	doc.Find("img[data-src], img[src]").Each(func(i int, sel *goquery.Selection) {
		if len(images) >= maxResults {
			return
		}

		imageURL := ""
		if src, exists := sel.Attr("data-src"); exists && strings.HasPrefix(src, "http") {
			imageURL = src
		} else if src, exists := sel.Attr("src"); exists && strings.HasPrefix(src, "http") {
			imageURL = src
		}

		// Filtrar URLs de thumbnails pequeños y logos
		if imageURL != "" {
			// Excluir URLs de Google que son thumbnails
			if strings.Contains(imageURL, "googleusercontent.com") && !strings.Contains(imageURL, "=s") {
				// Intentar obtener la imagen en tamaño completo
				if strings.Contains(imageURL, "=w") {
					// Reemplazar parámetro de ancho para obtener imagen más grande
					imageURL = regexp.MustCompile(`=w\d+-h\d+`).ReplaceAllString(imageURL, "=w800-h600")
				}
			}

			// Verificar que no sea un logo o icono pequeño
			if !strings.Contains(strings.ToLower(imageURL), "logo") &&
				!strings.Contains(strings.ToLower(imageURL), "icon") &&
				!strings.Contains(imageURL, "gstatic.com") {
				images = append(images, imageURL)
			}
		}
	})

	// También buscar en los datos JSON embebidos en la página
	doc.Find("script").Each(func(i int, sel *goquery.Selection) {
		if len(images) >= maxResults {
			return
		}
		scriptText := sel.Text()
		// Buscar URLs de imágenes en el JSON embebido
		imgPattern := regexp.MustCompile(`"(https?://[^"]+\.(?:jpg|jpeg|png|webp)[^"]*)"`)
		matches := imgPattern.FindAllStringSubmatch(scriptText, -1)
		for _, match := range matches {
			if len(images) >= maxResults {
				break
			}
			if len(match) > 1 && strings.HasPrefix(match[1], "http") {
				imgURL := match[1]
				// Filtrar thumbnails y logos
				if !strings.Contains(strings.ToLower(imgURL), "logo") &&
					!strings.Contains(strings.ToLower(imgURL), "icon") &&
					!strings.Contains(imgURL, "gstatic.com") {
					images = append(images, imgURL)
				}
			}
		}
	})

	// Eliminar duplicados
	seen := make(map[string]bool)
	uniqueImages := []string{}
	for _, img := range images {
		if !seen[img] {
			seen[img] = true
			uniqueImages = append(uniqueImages, img)
			if len(uniqueImages) >= maxResults {
				break
			}
		}
	}

	return uniqueImages, nil
}

