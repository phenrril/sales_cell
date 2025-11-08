## MVP Mayorista (Opción C: Variantes) — Plan de Implementación

### Objetivo
Transformar Chroma3D en un MVP para una tienda mayorista de celulares y accesorios, incorporando:
- Variantes por producto (color, capacidad, etc.) con `SKU`, `EAN`, precio y stock por variante.
- Operatoria mayorista: listas de precios B2B, IVA/condición fiscal, ventas por pack, escalas por cantidad y búsqueda por EAN.

### Alcance de esta fase
- Backoffice Admin: ABM de productos con grilla de variantes, búsqueda por EAN, importación/exportación CSV.
- Dominio/DB: nuevas entidades/campos para variantes, stock y atributos; clientes con condición fiscal; orden con desglose impositivo.
- Casos de uso: pricing B2B, impuestos según cliente, manejo de stock por variante.
- Front tienda pública: sin cambios profundos (mostrar variante seleccionada si aplica).

---

## Modelo de datos (Dominio)

Nota: existe `domain.Variant` actual enfocado en parámetros de impresión 3D. Para el MVP mayorista lo extenderemos (sin romper compatibilidad) y deprecaremos los campos de impresión.

### Producto (`domain.Product`)
- Mantener: `ID`, `Slug`, `Name`, `BasePrice`, `Category`, `ShortDesc`, `Images`, `CreatedAt/UpdatedAt`.
- Nuevo recomendado:
  - `Brand` (marca) `string`
  - `Model` (modelo) `string`
  - `Attributes` `map[string]string` (persistido como JSONB) para metadatos generales del producto.

### Variante (`domain.Variant`)
- Mantener: `ID`, `ProductID`, `CreatedAt`, `UpdatedAt`.
- Deprecados (no se usan para celulares): `Material`, `LayerHeightMM`, `InfillPct`.
- Nuevos campos:
  - `SKU` `string` (índice único)
  - `EAN` `string` (índice único, validar EAN-13/GTIN)
  - `Attributes` `map[string]string` (JSONB), ej.: `{ "color": "Negro", "capacidad": "128GB" }`
  - `Price` `decimal(12,2)` precio neto (sin IVA) por variante
  - `Cost` `decimal(12,2)` costo
  - `Stock` `int`
  - `ImageURL` `string` (opcional: imagen principal por variante)

### Imagen (`domain.Image`)
- Mantener como imágenes a nivel producto.
- Futuro: tabla opcional `variant_images` si se requieren múltiples imágenes por variante.

### Cliente (`domain.Customer`)
- Mantener: `ID`, `Email`, `Name`, `Phone`.
- Nuevos campos:
  - `TaxID` (CUIT) `string`
  - `TaxCondition` `string` enum: `RI` (Responsable Inscripto), `MT` (Monotributo), `EX` (Exento), `CF` (Consumidor Final)
  - `PriceList` `string` (p. ej., `MAYORISTA_A`, `MAYORISTA_B`) para listas diferenciadas

### Orden (`domain.Order` y `domain.OrderItem`)
- Nuevos/ajustes:
  - `CustomerID` en `Order`
  - Campos impositivos:
    - `SubtotalNet` `decimal(12,2)`
    - `VATAmount` `decimal(12,2)` (IVA total)
    - `Total` ya existe (Total bruto c/ IVA)
  - `OrderItem`:
    - `VariantID` (además de `ProductID`), `SKU`, `EAN` (denormalizados para auditoría)
    - `UnitPriceNet` (neto), `VATRate` (porcentaje), `VATAmount`, `UnitPriceGross`

---

## Base de Datos y Migraciones

Se utilizará `AutoMigrate` de GORM para los nuevos campos/tablas y se agregarán índices manuales.

### Cambios SQL propuestos
```sql
-- Productos
ALTER TABLE products ADD COLUMN IF NOT EXISTS brand text;
ALTER TABLE products ADD COLUMN IF NOT EXISTS model text;
ALTER TABLE products ADD COLUMN IF NOT EXISTS attributes jsonb DEFAULT '{}'::jsonb;

-- Variantes
ALTER TABLE variants ADD COLUMN IF NOT EXISTS sku text;
ALTER TABLE variants ADD COLUMN IF NOT EXISTS ean text;
ALTER TABLE variants ADD COLUMN IF NOT EXISTS attributes jsonb DEFAULT '{}'::jsonb;
ALTER TABLE variants ADD COLUMN IF NOT EXISTS price numeric(12,2) DEFAULT 0;
ALTER TABLE variants ADD COLUMN IF NOT EXISTS cost numeric(12,2) DEFAULT 0;
ALTER TABLE variants ADD COLUMN IF NOT EXISTS stock integer DEFAULT 0;
ALTER TABLE variants ADD COLUMN IF NOT EXISTS image_url text;

CREATE UNIQUE INDEX IF NOT EXISTS idx_variants_sku_unique ON variants (sku) WHERE sku IS NOT NULL AND sku <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_variants_ean_unique ON variants (ean) WHERE ean IS NOT NULL AND ean <> '';
CREATE INDEX IF NOT EXISTS idx_variants_product_id ON variants (product_id);
CREATE INDEX IF NOT EXISTS idx_variants_attributes_gin ON variants USING gin (attributes);

-- Clientes
ALTER TABLE customers ADD COLUMN IF NOT EXISTS tax_id text;
ALTER TABLE customers ADD COLUMN IF NOT EXISTS tax_condition text;
ALTER TABLE customers ADD COLUMN IF NOT EXISTS price_list text;

-- Órdenes
ALTER TABLE orders ADD COLUMN IF NOT EXISTS customer_id uuid;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS subtotal_net numeric(12,2) DEFAULT 0;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS vat_amount numeric(12,2) DEFAULT 0;

-- Ítems
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS variant_id uuid;
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS sku text;
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS ean text;
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS unit_price_net numeric(12,2) DEFAULT 0;
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS vat_rate numeric(5,2) DEFAULT 21.00;
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS vat_amount numeric(12,2) DEFAULT 0;
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS unit_price_gross numeric(12,2) DEFAULT 0;
```

Notas:
- Se reservan índices únicos para `SKU` y `EAN` a nivel variante.
- Se modela precio neto (sin IVA) a nivel variante para facilidad B2B.

---

## Repositorios y Puertos

### `domain.ProductRepo` (extensiones)
- Nuevos métodos sugeridos:
  - `FindVariantByEAN(ctx, ean string) (*domain.Product, *domain.Variant, error)`
  - `FindVariantBySKU(ctx, sku string) (*domain.Product, *domain.Variant, error)`
  - `SaveVariant(ctx, v *domain.Variant) error`
  - `UpdateVariantStock(ctx, variantID uuid.UUID, delta int) error`
  - `ListVariants(ctx, productID uuid.UUID) ([]domain.Variant, error)`

### `domain.CustomerRepo`
- Extender con consultas por `TaxID` y `PriceList` si fuera necesario.

---

## Casos de uso

### `usecase.ProductUC`
- Agregar:
  - `CreateVariant`, `UpdateVariant`, `DeleteVariant`
  - `GenerateCombinations` (a partir de sets de atributos, ej. colores x capacidades)
  - `SearchByEAN` y `SearchBySKU`

### Pricing B2B e IVA
- Políticas:
  - Precios guardados como netos (sin IVA) en `Variant.Price`.
  - IVA aplicado según `Customer.TaxCondition`:
    - `RI`, `MT`, `CF`: calcular IVA y mostrar total bruto; discriminar en factura si corresponde.
    - `EX`: IVA 0; total = neto.
  - Listas de precios por `Customer.PriceList` con ajustes multiplicativos o descuentos por variante.
  - Escalas por cantidad: tabla en memoria o JSON en producto (Fase 2), ej. 1-9, 10-49, 50+.

### Stock
- Descontar stock por `VariantID` al confirmar pedido.
- Validar disponibilidad al agregar al carrito.

---

## HTTP/Handlers y Vistas Admin

### Admin Productos (`internal/views/admin/products.html`)
- Añadir campos de producto: Marca, Modelo, Atributos.
- Sección Variantes:
  - Grilla con columnas: `Color`, `Capacidad` (u otros atributos), `SKU`, `EAN`, `Precio (neto)`, `Stock`, `Imagen`.
  - Botón “Generar combinaciones” desde sets predefinidos (p. ej., colores x capacidades).
  - Validación EAN (dígito verificador) y aviso de duplicados.
  - Búsqueda por EAN/SKU.

### Admin Clientes
- Altas con `TaxID`, `TaxCondition`, `PriceList`.

### Admin Ventas/Pedidos
- Ver detalle con desglose `Subtotal Net`, `IVA`, `Total`.

---

## Endpoints (mínimos sugeridos)

- `GET /admin/products` listado + búsqueda por `q`, `ean`, `sku`.
- `POST /admin/products` crear/editar producto.
- `POST /admin/products/:slug/variants` crear/editar variantes en lote.
- `DELETE /admin/products/:slug/variants/:id` eliminar variante.
- `GET /admin/scan?ean=...` buscar por EAN (alta rápida).
- `POST /admin/import/csv` importar productos/variantes.
- `GET /admin/export/csv` exportar.

Front:
- `GET /products/:slug` mostrar variantes y permitir elegir atributos.

---

## Validación de EAN

- Aceptar EAN-13/GTIN-13 (y opcional UPC/EAN-8 si hay casos).
- Validar dígito verificador en UI y backend.
- Índice único condicional para evitar duplicados.

---

## Importación/Exportación CSV

Plantilla CSV (encabezados):
```csv
slug,name,category,brand,model,short_desc,variant_sku,variant_ean,attr_color,attr_capacidad,price_net,stock,image_url
soporte-celular,Soporte Celular,accesorios,Generic,Soporte X,"Soporte universal",SC-NEG-128,7790000000010,Negro,128GB,1500,50,/uploads/images/sc-negro.jpg
soporte-celular,Soporte Celular,accesorios,Generic,Soporte X,"Soporte universal",SC-AZU-128,7790000000011,Azul,128GB,1500,20,/uploads/images/sc-azul.jpg
```

Reglas:
- Si `slug` existe: actualizar producto y upsert variantes por `variant_sku` o `variant_ean`.
- Si `slug` no existe: crear producto y variantes.

---

## Cálculo de Precios e IVA (ejemplo)

Para cada ítem:
- `neto_unit = Variant.Price` (ajustado por lista de precios/escala)
- `iva_pct = 21%` (por defecto; configurable por categoría)
- Según `Customer.TaxCondition`:
  - `EX`: `iva_unit = 0`, `bruto_unit = neto_unit`
  - caso general: `iva_unit = neto_unit * iva_pct`, `bruto_unit = neto_unit + iva_unit`
- Totales: `SubtotalNet = Σ(neto_unit * qty)`, `VATAmount = Σ(iva_unit * qty)`, `Total = SubtotalNet + VATAmount`

---

## Roadmap por fases

### Fase 1 (esta entrega)
- Extender dominio/DB (Variant con SKU/EAN/Price/Stock/Attributes; Customer con fiscal; Order con impuestos).
- Repos/UC: CRUD variantes, búsqueda por EAN/SKU, cálculo neto→bruto por condición fiscal.
- Admin: grilla de variantes, validación EAN, import/export CSV, búsqueda por EAN.

### Fase 2
- Listas de precios avanzadas y escalas por cantidad.
- Imágenes múltiples por variante.
- Packs/cajas con EAN propio y `pack_size`.

### Fase 3
- Facturación electrónica/AFIP (A/B) e integración contable.
- Catálogo de atributos tipados y filtros en tienda.

---

## Riesgos y mitigaciones
- Ruptura de compatibilidad con variantes anteriores: se mantienen campos y se agregan nuevos; migración segura.
- Duplicados de EAN/SKU: índices únicos condicionales.
- Rendimiento de filtros por atributo: índice GIN en `variants.attributes`.

---

## Tareas concretas (checklist)

- Dominio: extender `Variant` con `SKU`, `EAN`, `Attributes`, `Price`, `Cost`, `Stock`, `ImageURL`.
- Dominio: extender `Customer` con `TaxID`, `TaxCondition`, `PriceList`.
- Dominio/Orden: agregar `CustomerID`, `SubtotalNet`, `VATAmount` y campos en `OrderItem`.
- Repo Postgres: métodos para variantes (save/find by EAN/SKU, stock) e índices.
- Usecase: CRUD variantes, pricing con IVA, búsqueda por EAN.
- Admin UI: grilla de variantes, validación EAN, import/export.
- Endpoint de búsqueda por EAN y flujo de escaneo.


