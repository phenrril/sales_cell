# Carga de productos y mantenimiento

Este documento resume cómo crear, listar y eliminar productos con las últimas features.

## 0. Autenticación Admin (nuevo)
Los endpoints de gestión (crear / listar / obtener / borrar / upload) ahora requieren un JWT de admin.

Flujo:
1. Configurar en `.env`:
   - `ADMIN_API_KEY` (clave larga secreta)
   - `ADMIN_ALLOWED_EMAILS` (ej: `mati.orset@gmail.com` o varias separadas por coma)
   - `JWT_ADMIN_SECRET` (opcional; si se omite usa `SECRET_KEY`)
2. Obtener token: `POST /admin/login` con header `X-Admin-Key` y (opcional) body JSON con el email permitido.
3. Usar el token devuelto en `Authorization: Bearer <token>` para todos los endpoints protegidos:
   - `POST /api/products`
   - `POST /api/products/upload`
   - `GET /api/products`
   - `GET /api/products/{slug}`
   - `DELETE /api/products/{slug}`
   - `POST /api/products/delete`
   - `GET /admin/orders` (listado paginado de órdenes)

Expira aprox. a los 30 minutos: repetir login si recibes 401 por token vencido.

Ejemplo login (curl):
```
curl -X POST http://localhost:8080/admin/login \
  -H "X-Admin-Key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"email":"mati.orset@gmail.com"}'
```
Respuesta:
```
{ "token": "<JWT>", "exp": 1712345678, "email": "mati.orset@gmail.com" }
```
Luego:
```
export ADMIN_JWT=<JWT>
```

Ejemplo petición protegida:
```
curl -H "Authorization: Bearer $ADMIN_JWT" http://localhost:8080/api/products
```
Si el token expira (30 min por defecto) repetir login.

---

## 1. Preparar entorno
Arrancar servidor:
```
go run ./cmd/tienda3d
# o
docker compose up -d --build
```
Asegurate de tener `.env` basado en `.env.example`.

Variables mínimas: `DB_DSN` (o POSTGRES_*) , `MP_ACCESS_TOKEN` (TEST-...), `SESSION_KEY`.
Opcionales: `PUBLIC_BASE_URL`, `APP_ENV`, `PROD_ACCESS_TOKEN`, `STORAGE_DIR`, `GOOGLE_CLIENT_ID/SECRET`, `TELEGRAM_BOT_TOKEN/CHAT_ID` o `TELEGRAM_CHAT_IDS` (coma-separado para múltiples destinos), SMTP_*, `ADMIN_API_KEY`, `ADMIN_ALLOWED_EMAILS`, `JWT_ADMIN_SECRET`.

## 2. Carga de imágenes / productos
(Requiere header `Authorization: Bearer <token>` obtenido en sección 0.)

### 2.1 Endpoint multipart (crear producto + imágenes)
`POST /api/products/upload`
Headers:
- `Authorization: Bearer <token>`
- `Content-Type: multipart/form-data`

Campos:
- `name` (req)
- `base_price` (req, número)
- `category` (opt)
- `short_desc` (opt)
- `ready_to_ship` (`true|false|1|0`)
- `width_mm` (opt, número >=0, ancho en milímetros)
- `height_mm` (opt, número >=0, alto en milímetros)
- `depth_mm` (opt, número >=0, profundidad/fondo en milímetros)
- `image` (1 archivo) y/o `images` (múltiples). Puedes incluir varias filas `images`.
- `existing_slug` (opt) si solo quieres agregar imágenes a un producto existente (ignora dimensiones si ya existe).

Notas dimensiones:
- Si omites las medidas se guardan como 0.
- Valores negativos se normalizan a 0 internamente.
- Útiles para mostrar specs o cálculos futuros de volumen.

Ejemplo Postman (form-data) agregar también Auth Bearer.

Curl:
```
curl -X POST http://localhost:8080/api/products/upload \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -F name="Maceta Hexa" \
  -F base_price=3500 \
  -F category=jardin \
  -F short_desc="Maceta decorativa" \
  -F ready_to_ship=true \
  -F width_mm=120 \
  -F height_mm=90 \
  -F depth_mm=110 \
  -F images=@img1.webp \
  -F images=@img2.webp
```
Respuesta 201 incluye `Images` con URLs `/uploads/images/...` y las medidas guardadas.

### 2.2 JSON simple (sin subir archivos)
`POST /api/products`
Headers: `Authorization: Bearer <token>`
Body JSON:
```
{
  "name": "Maceta Hexa",
  "category": "jardin",
  "short_desc": "Maceta decorativa impresa en 3D",
  "base_price": 3500,
  "ready_to_ship": true,
  "width_mm": 120,
  "height_mm": 90,
  "depth_mm": 110
}
```
Observaciones:
- Las medidas son opcionales; si no se envían quedan 0.
- Validación: solo se rechaza si alguna medida viene negativa.
- Este endpoint no crea imágenes.

## 3. Listar productos
`GET /api/products` (requiere token admin)
Devuelve `items` y `total`.

## 4. Obtener detalle por slug
`GET /api/products/{slug}` (requiere token admin)

## 5. Eliminación
(Endpoints protegidos por token)
### 5.1 Borrado completo (con archivos)
`DELETE /api/products/{slug}` elimina:
- Producto, variantes, imágenes (DB)
- Archivos físicos `uploads/...` asociados (si existen)
Respuesta:
```
{
  "status": "ok",
  "slug": "maceta-hexa",
  "removed_files": ["uploads/images/1696...-maceta-hexa.webp"]
}
```
Errores: 404 inexistente, 500 error interno.

### 5.2 Borrado masivo simple
`POST /api/products/delete`
Body:
```
{ "slugs": ["maceta-hexa", "otro-prod"] }
```
Devuelve arrays `deleted` y `errors`. (No borra archivos físicos).

## 6. Carrusel y miniaturas (frontend)
- Si el producto tiene >1 imagen se activa carrusel (autoplay 5s, flechas, thumbnails, swipe, teclado).
- Primera imagen se usa de portada en listados.

## 7. Colores
- Los colores se deducen de las variantes (`Product.Variants`).
- Si no hay variantes se muestra set por defecto (`#111827`, `#6366f1`, `#16a34a`, `#f59e0b`, `#ff3d00`)

## 8. Carrito y envío
- Agrega productos desde `/product/{slug}` (envía slug + color).
- Todos los costos provinciales actuales están fijos en 9000 en `server.go (provinceCosts)`.
- Retiro: costo 0.

## 9. Pagos
- `POST /api/checkout` genera preferencia MP (público, no requiere token admin).
- Carrito normal: checkout crea orden y llama a MercadoPago; redirección a `/pay/{orderID}`.
- Webhook: `/webhooks/mp` (configurar en MP apuntando a `PUBLIC_BASE_URL/webhooks/mp`).

## 10. Notificaciones
- **Telegram**: requiere `TELEGRAM_BOT_TOKEN` y uno de:
  - `TELEGRAM_CHAT_ID` (un único destino), o
  - `TELEGRAM_CHAT_IDS` con varios destinos separados por coma (ej.: `-1001234567890,@SoyCanalla`).
- **Email (SMTP)**: requiere SMTP_* variables y `ORDER_NOTIFY_EMAIL`.
- Se envían cuando el pago se marca aprobado (MP webhook o callback simulado).

## 11. OAuth Google (opcional)
Configurar `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `BASE_URL` para habilitar login. Se generan rutas `/auth/google/login` y callback.

## 12. Rutas claves
Web: `/`, `/products`, `/product/{slug}`, `/cart`, `/checkout`, `/pay/{id}`
API protegida: `/api/products`, `/api/products/upload`, `/api/products/{slug}`, `/api/products/delete`, `/admin/orders`
API pública: `/api/quote`, `/api/checkout`, `/webhooks/mp`

## 13. Errores comunes
| Código | Causa | Solución |
|--------|-------|----------|
| 400 | Campos faltantes / multipart | Revisar nombres y tipos en form-data |
| 401 | Falta token o inválido | Rehacer login `/admin/login` y usar Bearer |
| 403 | Email no permitido | Agregar email a `ADMIN_ALLOWED_EMAILS` |
| 404 | Producto no encontrado | Verificar slug |
| 405 | Método incorrecto | Usar método soportado |
| 500 | Error interno DB / storage | Revisar logs |

## 14. Buenas prácticas
- No subir `.env` (usar `.env.example`).
- Regenerar `SESSION_KEY` / `JWT_ADMIN_SECRET` si se filtran.
- Rotar `ADMIN_API_KEY` periódicamente.
- Usar token TEST de MP en desarrollo (empieza con `TEST-`).
- Ajustar costos de envío modificando `provinceCosts` y recompilando.

## 15. Ejemplos rápidos
Login admin:
```
curl -X POST http://localhost:8080/admin/login \
  -H "X-Admin-Key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"email":"mati.orset@gmail.com"}'
```
Listar productos:
```
curl -H "Authorization: Bearer $ADMIN_JWT" http://localhost:8080/api/products
```
Listar órdenes:
```
curl -H "Authorization: Bearer $ADMIN_JWT" http://localhost:8080/admin/orders
```
Carga multipart:
```
curl -X POST http://localhost:8080/api/products/upload \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -F name="Clip Bolsa" \
  -F base_price=600 \
  -F ready_to_ship=true \
  -F width_mm=30 -F height_mm=15 -F depth_mm=8 \
  -F image=@clip.jpg
```
Delete completo:
```
curl -X DELETE http://localhost:8080/api/products/clip-bolsa \
  -H "Authorization: Bearer $ADMIN_JWT"
```
Borrado masivo:
```
curl -X POST http://localhost:8080/api/products/delete \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{"slugs":["clip-bolsa","otro-prod"]}'
```

---
Fin.
