package postgres

import (
	"context"
	"errors"
	"time"

	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/phenrril/tienda3d/internal/domain"
)

type ProductRepo struct{ db *gorm.DB }

func NewProductRepo(db *gorm.DB) *ProductRepo { return &ProductRepo{db: db} }

// RawDB expone la conexión de base de datos (útil para operaciones avanzadas)
func (r *ProductRepo) RawDB() *gorm.DB { return r.db }

func (r *ProductRepo) Save(ctx context.Context, p *domain.Product) error {
	return r.db.WithContext(ctx).Save(p).Error
}

func (r *ProductRepo) AddImages(ctx context.Context, productID uuid.UUID, imgs []domain.Image) error {
	if len(imgs) == 0 {
		return nil
	}
	for i := range imgs {
		if imgs[i].ID == uuid.Nil {
			imgs[i].ID = uuid.New()
		}
		imgs[i].ProductID = productID
		if imgs[i].CreatedAt.IsZero() {
			imgs[i].CreatedAt = time.Now()
		}
	}
	return r.db.WithContext(ctx).Create(&imgs).Error
}

func (r *ProductRepo) FindBySlug(ctx context.Context, slug string) (*domain.Product, error) {
	var p domain.Product
	if err := r.db.WithContext(ctx).Preload("Images").Preload("Variants").First(&p, "slug = ?", slug).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *ProductRepo) List(ctx context.Context, f domain.ProductFilter) ([]domain.Product, int64, error) {
	var list []domain.Product
	q := r.db.WithContext(ctx).Model(&domain.Product{})

	// Por defecto, solo mostrar productos activos (a menos que se especifique lo contrario)
	if f.IncludeInactive == nil || !*f.IncludeInactive {
		q = q.Where("active = ?", true)
	}

	if f.Category != "" {
		// Si category=celulares, buscar productos de marcas de celulares
		if f.Category == "celulares" {
			q = q.Where("LOWER(category) IN ('iphone', 'samsung', 'xiaomi', 'moto', 'poco')")
		} else {
			q = q.Where("category = ?", f.Category)
		}
	}
	if f.ReadyToShip != nil {
		q = q.Where("ready_to_ship = ?", *f.ReadyToShip)
	}
	if f.Query != "" {
		query := strings.TrimSpace(f.Query)

		// Caso especial: "novedades" -> Todo lo que NO sea celulares ni smartwatches (consolas, auriculares, etc.)
		if strings.EqualFold(query, "novedades") {
			// Excluir categorías de celulares y smartwatches
			q = q.Where("LOWER(category) NOT IN ('iphone', 'samsung', 'xiaomi', 'moto', 'poco', 'pencil para ipad usb-c') AND LOWER(brand) != 'watch' AND LOWER(name) NOT LIKE '%watch%'")
		} else if strings.EqualFold(query, "ofertas") {
			// Ofertas -> Smartwatches (Apple Watch están en categoría "pencil para ipad usb-c" con brand "Watch")
			q = q.Where("LOWER(brand) = 'watch' OR LOWER(category) = 'pencil para ipad usb-c' OR LOWER(name) LIKE '%watch%'")
		} else if strings.EqualFold(query, "auriculares") {
			// Auriculares -> Buscar por categoría audio-auris o productos con "auri", "airpod" en el nombre
			q = q.Where("LOWER(category) = 'audio-auris' OR LOWER(name) LIKE '%auri%' OR LOWER(name) LIKE '%auricular%' OR LOWER(name) LIKE '%airpod%'")
		} else if strings.EqualFold(query, "notebooks") {
			// Notebooks -> Buscar por categoría notebooks o productos con "notebook", "macbook", "nb " en el nombre
			q = q.Where("LOWER(category) = 'notebooks' OR LOWER(name) LIKE '%notebook%' OR LOWER(name) LIKE '%macbook%' OR LOWER(name) LIKE 'nb %'")
		} else if strings.EqualFold(query, "samsung") {
			// Samsung -> Buscar por category (más preciso)
			if f.Category == "celulares" {
				// Ya está filtrado por celulares, buscar Samsung
				q = q.Where("LOWER(category) = 'samsung'")
			} else {
				// Buscar todos los Samsung
				q = q.Where("LOWER(category) = 'samsung' OR LOWER(brand) = 'samsung'")
			}
		} else if strings.EqualFold(query, "apple") || strings.EqualFold(query, "iphone") {
			// Apple -> Solo celulares iPhone cuando category=celulares
			if f.Category == "celulares" {
				// Ya está filtrado por celulares, buscar solo iPhone (excluir Watch)
				q = q.Where("LOWER(category) = 'iphone' AND LOWER(brand) != 'watch'")
			} else {
				// Ecosistema Apple completo: iPhone + Watch
				q = q.Where("LOWER(category) = 'iphone' OR (LOWER(category) = 'pencil para ipad usb-c' AND LOWER(brand) = 'watch')")
			}
		} else if strings.EqualFold(query, "moto") || strings.EqualFold(query, "motorola") {
			// Motorola -> Buscar por category
			if f.Category == "celulares" {
				// Ya está filtrado por celulares, buscar Motorola
				q = q.Where("LOWER(category) = 'moto'")
			} else {
				q = q.Where("LOWER(category) = 'moto' OR LOWER(brand) = 'moto'")
			}
		} else if strings.EqualFold(query, "xiaomi") {
			// Xiaomi -> Buscar por category (incluye Xiaomi y Poco)
			if f.Category == "celulares" {
				// Ya está filtrado por celulares, buscar Xiaomi y Poco
				q = q.Where("LOWER(category) IN ('xiaomi', 'poco')")
			} else {
				q = q.Where("LOWER(category) IN ('xiaomi', 'poco') OR LOWER(brand) IN ('xiaomi', 'poco')")
			}
		} else if strings.EqualFold(query, "tcl") {
			// TCL -> Buscar por brand y name (no hay categoría TCL en la BD actual)
			if f.Category == "celulares" {
				// Ya está filtrado por celulares, buscar TCL
				q = q.Where("LOWER(brand) = 'tcl' OR LOWER(name) LIKE 'tcl%'")
			} else {
				q = q.Where("LOWER(brand) = 'tcl' OR LOWER(name) LIKE 'tcl%'")
			}
		} else {
			// Búsqueda genérica
			like := "%" + query + "%"
			q = q.Where("LOWER(name) LIKE LOWER(?) OR LOWER(category) LIKE LOWER(?) OR LOWER(brand) LIKE LOWER(?) OR LOWER(model) LIKE LOWER(?)", like, like, like, like)
		}
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	switch f.Sort {
	case "price_desc":
		q = q.Order("base_price desc")
	case "price_asc":
		q = q.Order("base_price asc")
	case "newest":
		q = q.Order("created_at desc")
	default:
		q = q.Order("name asc")
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	offset := (f.Page - 1) * f.PageSize
	if err := q.Offset(offset).Limit(f.PageSize).Preload("Images", func(db *gorm.DB) *gorm.DB { return db.Order("created_at asc") }).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *ProductRepo) DeleteBySlug(ctx context.Context, slug string) error {
	return r.db.WithContext(ctx).Where("slug = ?", slug).Delete(&domain.Product{}).Error
}

func (r *ProductRepo) DeleteFullBySlug(ctx context.Context, slug string) ([]string, error) {
	if slug == "" {
		return nil, errors.New("slug vacío")
	}
	var p domain.Product
	if err := r.db.WithContext(ctx).Preload("Images").Preload("Variants").First(&p, "slug = ?", slug).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	imgPaths := []string{}
	for _, im := range p.Images {
		imgPaths = append(imgPaths, im.URL)
	}
	return imgPaths, r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("product_id = ?", p.ID).Delete(&domain.Image{}).Error; err != nil {
			return err
		}
		if err := tx.Where("product_id = ?", p.ID).Delete(&domain.Variant{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&domain.Product{}, "id = ?", p.ID).Error; err != nil {
			return err
		}
		return nil
	})
}

func (r *ProductRepo) DistinctCategories(ctx context.Context) ([]string, error) {
	cats := []string{}
	if err := r.db.WithContext(ctx).Model(&domain.Product{}).
		Distinct("category").Where("category <> ''").Order("category asc").Pluck("category", &cats).Error; err != nil {
		return nil, err
	}
	return cats, nil
}

// --- Variantes ---

func (r *ProductRepo) SaveVariant(ctx context.Context, v *domain.Variant) error {
	if v.ID == uuid.Nil {
		v.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Save(v).Error
}

func (r *ProductRepo) ListVariants(ctx context.Context, productID uuid.UUID) ([]domain.Variant, error) {
	var list []domain.Variant
	if err := r.db.WithContext(ctx).Where("product_id = ?", productID).Order("created_at asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *ProductRepo) FindVariantByEAN(ctx context.Context, ean string) (*domain.Product, *domain.Variant, error) {
	var v domain.Variant
	if err := r.db.WithContext(ctx).First(&v, "ean = ?", ean).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, domain.ErrNotFound
		}
		return nil, nil, err
	}
	var p domain.Product
	if err := r.db.WithContext(ctx).First(&p, "id = ?", v.ProductID).Error; err != nil {
		return nil, nil, err
	}
	return &p, &v, nil
}

func (r *ProductRepo) FindVariantBySKU(ctx context.Context, sku string) (*domain.Product, *domain.Variant, error) {
	var v domain.Variant
	if err := r.db.WithContext(ctx).First(&v, "sku = ?", sku).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, domain.ErrNotFound
		}
		return nil, nil, err
	}
	var p domain.Product
	if err := r.db.WithContext(ctx).First(&p, "id = ?", v.ProductID).Error; err != nil {
		return nil, nil, err
	}
	return &p, &v, nil
}

func (r *ProductRepo) UpdateVariantStock(ctx context.Context, variantID uuid.UUID, delta int) error {
	return r.db.WithContext(ctx).Model(&domain.Variant{}).Where("id = ?", variantID).UpdateColumn("stock", gorm.Expr("COALESCE(stock,0) + ?", delta)).Error
}

func (r *ProductRepo) DeleteVariant(ctx context.Context, variantID uuid.UUID) error {
	if variantID == uuid.Nil {
		return errors.New("variant id vacío")
	}
	return r.db.WithContext(ctx).Where("id = ?", variantID).Delete(&domain.Variant{}).Error
}

func (r *ProductRepo) ClearImages(ctx context.Context, productID uuid.UUID) ([]string, error) {
	var list []domain.Image
	if err := r.db.WithContext(ctx).Where("product_id = ?", productID).Find(&list).Error; err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(list))
	for _, im := range list {
		paths = append(paths, im.URL)
	}
	if err := r.db.WithContext(ctx).Where("product_id = ?", productID).Delete(&domain.Image{}).Error; err != nil {
		return nil, err
	}
	return paths, nil
}

// MarkAllInactive marca todos los productos como inactivos (para proceso de importación)
func (r *ProductRepo) MarkAllInactive(ctx context.Context) error {
	return r.db.WithContext(ctx).Model(&domain.Product{}).Where("1 = 1").Update("active", false).Error
}

// GetInactiveSlugs obtiene los slugs de todos los productos inactivos
func (r *ProductRepo) GetInactiveSlugs(ctx context.Context) ([]string, error) {
	var slugs []string
	err := r.db.WithContext(ctx).Model(&domain.Product{}).
		Where("active = ?", false).
		Pluck("slug", &slugs).Error
	return slugs, err
}
