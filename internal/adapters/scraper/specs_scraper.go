package scraper

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type SpecsScraper struct {
	client *http.Client
}

func NewSpecsScraper() *SpecsScraper {
	return &SpecsScraper{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// SearchSpecs busca especificaciones técnicas de un teléfono en múltiples sitios
func (s *SpecsScraper) SearchSpecs(ctx context.Context, productName, brand, model string) (map[string]string, error) {
	// Construir query de búsqueda
	query := s.buildSearchQuery(productName, brand, model)

	// Intentar buscar en diferentes sitios
	specs := make(map[string]string)

	// 1. GSMArena
	if gsmSpecs, err := s.searchGSMArena(ctx, query); err == nil && len(gsmSpecs) > 0 {
		specs = mergeSpecs(specs, gsmSpecs)
	}

	// 2. PhoneArena
	if phoneSpecs, err := s.searchPhoneArena(ctx, query); err == nil && len(phoneSpecs) > 0 {
		specs = mergeSpecs(specs, phoneSpecs)
	}

	// 3. Búsqueda genérica con Google
	if len(specs) == 0 {
		if googleSpecs, err := s.searchGoogle(ctx, query); err == nil && len(googleSpecs) > 0 {
			specs = mergeSpecs(specs, googleSpecs)
		}
	}

	return specs, nil
}

func (s *SpecsScraper) buildSearchQuery(productName, brand, model string) string {
	parts := []string{}
	if brand != "" {
		parts = append(parts, brand)
	}
	if model != "" {
		parts = append(parts, model)
	}
	if len(parts) == 0 {
		return productName
	}
	return strings.Join(parts, " ")
}

func (s *SpecsScraper) searchGSMArena(ctx context.Context, query string) (map[string]string, error) {
	// Buscar en GSMArena
	searchURL := fmt.Sprintf("https://www.gsmarena.com/results.php3?sQuickSearch=yes&sName=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

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

	// Buscar primer resultado
	var deviceURL string
	doc.Find("div.makers a").First().Each(func(i int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists {
			deviceURL = "https://www.gsmarena.com/" + href
		}
	})

	if deviceURL == "" {
		return nil, fmt.Errorf("no se encontró dispositivo")
	}

	// Obtener especificaciones del dispositivo
	return s.getGSMArenaSpecs(ctx, deviceURL)
}

func (s *SpecsScraper) getGSMArenaSpecs(ctx context.Context, deviceURL string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, deviceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

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

	specs := make(map[string]string)

	// Extraer especificaciones de las tablas
	doc.Find("table").Each(func(i int, table *goquery.Selection) {
		table.Find("tr").Each(func(j int, tr *goquery.Selection) {
			tds := tr.Find("td")
			if tds.Length() >= 2 {
				label := strings.TrimSpace(tds.First().Text())
				value := strings.TrimSpace(tds.Eq(1).Text())

				// Limpiar el valor (remover saltos de línea y espacios extra)
				value = strings.ReplaceAll(value, "\n", " ")
				value = strings.ReplaceAll(value, "\r", " ")
				value = regexp.MustCompile(`\s+`).ReplaceAllString(value, " ")
				value = strings.TrimSpace(value)

				// Buscar sensores específicamente (puede estar en una fila con múltiples valores)
				if strings.Contains(strings.ToLower(label), "sensor") {
					if s.isValidSensors(value) {
						// Si ya hay sensores, agregar a la lista
						if existing, exists := specs["Sensores"]; exists {
							specs["Sensores"] = existing + ", " + value
						} else {
							specs["Sensores"] = value
						}
					}
				}

				// Normalizar y mapear especificaciones con validación
				if spec := s.normalizeSpec(label, value); spec != "" {
					// Normalizar el valor según el tipo de especificación
					normalizedValue := s.normalizeValue(spec, value)
					// Solo agregar si no existe o si el nuevo valor es mejor (más largo, más específico)
					if existing, exists := specs[spec]; !exists || len(normalizedValue) > len(existing) {
						specs[spec] = normalizedValue
					}
				}
			}
		})
	})

	return specs, nil
}

func (s *SpecsScraper) searchPhoneArena(ctx context.Context, query string) (map[string]string, error) {
	// Buscar en PhoneArena
	searchURL := fmt.Sprintf("https://www.phonearena.com/phones/search?query=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

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

	// Buscar primer resultado
	var deviceURL string
	doc.Find("a.phone").First().Each(func(i int, sel *goquery.Selection) {
		if href, exists := sel.Attr("href"); exists {
			if strings.HasPrefix(href, "http") {
				deviceURL = href
			} else {
				deviceURL = "https://www.phonearena.com" + href
			}
		}
	})

	if deviceURL == "" {
		return nil, fmt.Errorf("no se encontró dispositivo")
	}

	// Obtener especificaciones
	return s.getPhoneArenaSpecs(ctx, deviceURL)
}

func (s *SpecsScraper) getPhoneArenaSpecs(ctx context.Context, deviceURL string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, deviceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

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

	specs := make(map[string]string)

	// Buscar especificaciones en diferentes secciones
	doc.Find(".specs-table tr, .specs-list li").Each(func(i int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())
		if text == "" {
			return
		}

		// Intentar extraer especificaciones comunes
		for _, pattern := range s.getSpecPatterns() {
			if matches := pattern.regex.FindStringSubmatch(text); len(matches) > 1 {
				specs[pattern.key] = strings.TrimSpace(matches[1])
				break
			}
		}
	})

	return specs, nil
}

func (s *SpecsScraper) searchGoogle(ctx context.Context, query string) (map[string]string, error) {
	// Búsqueda en Google con "especificaciones técnicas"
	searchQuery := query + " especificaciones técnicas"
	searchURL := fmt.Sprintf("https://www.google.com/search?q=%s", url.QueryEscape(searchQuery))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

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

	specs := make(map[string]string)

	// Buscar en los resultados destacados de Google
	doc.Find("div[data-attrid], div.g").Each(func(i int, sel *goquery.Selection) {
		text := sel.Text()
		for _, pattern := range s.getSpecPatterns() {
			if matches := pattern.regex.FindStringSubmatch(text); len(matches) > 1 {
				specs[pattern.key] = strings.TrimSpace(matches[1])
			}
		}
	})

	return specs, nil
}

type specPattern struct {
	key   string
	regex *regexp.Regexp
}

func (s *SpecsScraper) getSpecPatterns() []specPattern {
	return []specPattern{
		{key: "RAM", regex: regexp.MustCompile(`(?i)(?:RAM|Memoria RAM|Memoria)[:\s]+(\d+\s*(?:GB|MB|gb|mb))`)},
		{key: "Almacenamiento", regex: regexp.MustCompile(`(?i)(?:Almacenamiento|Storage|Capacidad|Memoria interna)[:\s]+(\d+\s*(?:GB|TB|gb|tb))`)},
		{key: "Pantalla", regex: regexp.MustCompile(`(?i)(?:Pantalla|Display|Screen|Tamaño de pantalla)[:\s]+([\d.]+[\s"]*(?:pulgadas|inches|"|pulg|inch))`)},
		{key: "Cámara", regex: regexp.MustCompile(`(?i)(?:Cámara principal|Cámara trasera|Camera|Main Camera|Rear Camera)[:\s]+(\d+\s*(?:MP|Mpx|megapixels?))`)},
		{key: "Batería", regex: regexp.MustCompile(`(?i)(?:Batería|Battery|Capacidad de batería)[:\s]+(\d+\s*(?:mAh|mah|mAh))`)},
		{key: "Procesador", regex: regexp.MustCompile(`(?i)(?:Procesador|Processor|Chipset|SoC)[:\s]+([A-Za-z0-9\s\-]+(?:Snapdragon|MediaTek|Exynos|Apple|A\d+|Helio|Dimensity|Tensor))`)},
		{key: "Sistema Operativo", regex: regexp.MustCompile(`(?i)(?:Sistema operativo|OS|Operating System)[:\s]+(Android\s*[\d.]+|iOS\s*[\d.]+|Android|iOS)`)},
	}
}

func (s *SpecsScraper) normalizeSpec(label, value string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	value = strings.TrimSpace(value)

	if value == "" {
		return ""
	}

	// Validar y mapear especificaciones con validación de contenido
	// RAM: debe contener números seguidos de GB o MB (preferir GB)
	if strings.Contains(label, "ram") || strings.Contains(label, "memory") {
		if s.isValidRAM(value) {
			return "RAM"
		}
		return ""
	}

	// Almacenamiento: debe contener números seguidos de GB o TB
	if strings.Contains(label, "internal") || strings.Contains(label, "storage") || strings.Contains(label, "capacity") {
		if s.isValidStorage(value) {
			return "Almacenamiento"
		}
		return ""
	}

	// Pantalla: debe contener pulgadas o números con "
	if strings.Contains(label, "display") || strings.Contains(label, "screen") {
		if s.isValidScreen(value) {
			return "Pantalla"
		}
		return ""
	}

	// Cámara: debe contener MP o megapixels
	if strings.Contains(label, "camera") || strings.Contains(label, "main camera") {
		if s.isValidCamera(value) {
			return "Cámara"
		}
		return ""
	}

	// Batería: debe contener mAh, W, o Wh
	if strings.Contains(label, "battery") || strings.Contains(label, "batería") {
		if s.isValidBattery(value) {
			return "Batería"
		}
		return ""
	}

	// Procesador: debe contener nombres conocidos de procesadores
	if strings.Contains(label, "chipset") || strings.Contains(label, "processor") || strings.Contains(label, "soc") {
		if s.isValidProcessor(value) {
			return "Procesador"
		}
		return ""
	}

	// Sistema Operativo: debe ser Android o iOS, NO sensores ni GPS
	if strings.Contains(label, "os") || strings.Contains(label, "operating system") || strings.Contains(label, "platform") {
		if s.isValidOS(value) {
			return "Sistema Operativo"
		}
		return ""
	}

	// Sensores: debe contener nombres de sensores comunes
	if strings.Contains(label, "sensor") || strings.Contains(label, "sensors") {
		if s.isValidSensors(value) {
			return "Sensores"
		}
		return ""
	}

	return ""
}

// isValidRAM valida que el valor sea una cantidad de RAM válida
func (s *SpecsScraper) isValidRAM(value string) bool {
	valueLower := strings.ToLower(value)
	// No debe contener símbolos de moneda ni ser un precio
	if strings.Contains(valueLower, "₹") || strings.Contains(valueLower, "$") || strings.Contains(valueLower, "€") || strings.Contains(valueLower, "£") {
		return false
	}
	// Debe contener números seguidos de GB o MB
	ramPattern := regexp.MustCompile(`\d+\s*(?:GB|MB|gb|mb)`)
	return ramPattern.MatchString(value)
}

// normalizeValue normaliza el valor según el tipo de especificación
func (s *SpecsScraper) normalizeValue(specType, value string) string {
	if specType == "RAM" {
		// Convertir MB a GB si es necesario (solo si es >= 1024 MB)
		mbPattern := regexp.MustCompile(`(\d+)\s*MB`)
		if matches := mbPattern.FindStringSubmatch(value); len(matches) > 1 {
			if mb, err := strconv.Atoi(matches[1]); err == nil && mb >= 1024 {
				gb := float64(mb) / 1024.0
				return fmt.Sprintf("%.1f GB", gb)
			}
		}
		// Si ya está en GB, mantenerlo
		gbPattern := regexp.MustCompile(`(\d+)\s*GB`)
		if matches := gbPattern.FindStringSubmatch(value); len(matches) > 1 {
			return matches[1] + " GB"
		}
	}
	return value
}

// isValidStorage valida que el valor sea almacenamiento válido
func (s *SpecsScraper) isValidStorage(value string) bool {
	valueLower := strings.ToLower(value)
	// No debe contener símbolos de moneda
	if strings.Contains(valueLower, "₹") || strings.Contains(valueLower, "$") || strings.Contains(valueLower, "€") || strings.Contains(valueLower, "£") {
		return false
	}
	// Debe contener números seguidos de GB o TB
	storagePattern := regexp.MustCompile(`\d+\s*(?:GB|TB|gb|tb)`)
	return storagePattern.MatchString(value)
}

// isValidScreen valida que el valor sea una pantalla válida
func (s *SpecsScraper) isValidScreen(value string) bool {
	valueLower := strings.ToLower(value)
	// Debe contener pulgadas, inches, o números con "
	screenPattern := regexp.MustCompile(`[\d.]+\s*(?:pulgadas|inches|"|pulg|inch)`)
	return screenPattern.MatchString(valueLower)
}

// isValidCamera valida que el valor sea una cámara válida
func (s *SpecsScraper) isValidCamera(value string) bool {
	valueLower := strings.ToLower(value)
	// Debe contener MP o megapixels
	cameraPattern := regexp.MustCompile(`\d+\s*(?:MP|Mpx|megapixels?)`)
	return cameraPattern.MatchString(valueLower)
}

// isValidBattery valida que el valor sea una batería válida (mAh, W, o Wh)
func (s *SpecsScraper) isValidBattery(value string) bool {
	valueLower := strings.ToLower(value)
	// No debe contener "active use score" u otros valores de tiempo de uso
	if strings.Contains(valueLower, "active use") || strings.Contains(valueLower, "score") || strings.Contains(valueLower, "hours") {
		return false
	}
	// Debe contener mAh, W, o Wh
	batteryPattern := regexp.MustCompile(`\d+\s*(?:mAh|mah|W|Wh|wh|w)`)
	return batteryPattern.MatchString(valueLower)
}

// isValidProcessor valida que el valor sea un procesador válido
func (s *SpecsScraper) isValidProcessor(value string) bool {
	valueLower := strings.ToLower(value)
	// No debe contener símbolos de moneda
	if strings.Contains(valueLower, "₹") || strings.Contains(valueLower, "$") || strings.Contains(valueLower, "€") || strings.Contains(valueLower, "£") {
		return false
	}
	// Debe contener nombres conocidos de procesadores
	processorKeywords := []string{"snapdragon", "mediatek", "exynos", "apple", "tensor", "helio", "dimensity", "a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10", "a11", "a12", "a13", "a14", "a15", "a16", "a17", "a18"}
	for _, keyword := range processorKeywords {
		if strings.Contains(valueLower, keyword) {
			return true
		}
	}
	return false
}

// isValidOS valida que el valor sea un sistema operativo válido
func (s *SpecsScraper) isValidOS(value string) bool {
	valueLower := strings.ToLower(value)
	// NO debe contener sensores comunes (GPS, GLONASS, etc.)
	sensorKeywords := []string{"gps", "glonass", "galileo", "beidou", "sensor", "accelerometer", "gyroscope", "magnetometer", "proximity", "ambient", "light"}
	for _, keyword := range sensorKeywords {
		if strings.Contains(valueLower, keyword) {
			return false
		}
	}
	// Debe contener Android o iOS
	osPattern := regexp.MustCompile(`(?:android|ios)`)
	return osPattern.MatchString(valueLower)
}

// isValidSensors valida que el valor sea una lista de sensores válida
func (s *SpecsScraper) isValidSensors(value string) bool {
	valueLower := strings.ToLower(value)
	// No debe contener símbolos de moneda ni ser un precio
	if strings.Contains(valueLower, "₹") || strings.Contains(valueLower, "$") || strings.Contains(valueLower, "€") || strings.Contains(valueLower, "£") {
		return false
	}
	// Debe contener al menos un sensor común
	sensorKeywords := []string{"gps", "glonass", "galileo", "beidou", "accelerometer", "gyroscope", "magnetometer", "proximity", "ambient", "light", "compass", "barometer", "fingerprint", "face", "iris"}
	for _, keyword := range sensorKeywords {
		if strings.Contains(valueLower, keyword) {
			return true
		}
	}
	return false
}

func mergeSpecs(existing, new map[string]string) map[string]string {
	result := make(map[string]string)

	// Copiar existentes
	for k, v := range existing {
		result[k] = v
	}

	// Agregar nuevas (solo si no existen)
	for k, v := range new {
		if _, exists := result[k]; !exists && v != "" {
			result[k] = v
		}
	}

	return result
}
