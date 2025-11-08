# Tienda3D

Proyecto de tienda online para venta de modelos 3D impresos, desarrollado en Go (arquitectura hexagonal + SSR). 

## Objetivo
Permitir la carga, visualización y compra de productos impresos en 3D, con integración a MercadoPago, manejo de imágenes y flujo de checkout completo.

## Novedades recientes
- Carrusel en detalle de producto (autoplay, flechas, miniaturas, swipe, teclado) + layout responsive.
- Selector de color (swatches) con fallback de colores si el producto no define variantes.
- Formulario de checkout rediseñado (gradientes, focus visible, layout responsive).
- Costos de envío unificados (actualmente todos 9000 en `provinceCosts` dentro de `server.go`).
- Endpoint multipart `POST /api/products/upload` para crear producto + múltiples imágenes en una sola request.
- Eliminación completa de producto (DELETE `/api/products/{slug}`) borra registros + archivos físicos en `uploads/`.
- Borrado masivo simple (`POST /api/products/delete`).
- Autenticación Admin con JWT (login por API key + lista de emails permitidos) protegiendo endpoints de gestión.
- Endpoint admin paginado de órdenes `GET /admin/orders`.
- Al subir imágenes se guardan en `uploads/images/...` (storage local) y se sirven vía `/uploads/...`.
- Integración opcional: OAuth Google (si se configuran las variables), notificaciones a Telegram y/o email (SMTP).
- Docker Compose parametrizado por `.env` (sin secretos en el YAML).

## Arquitectura y Estructura
- **Go nativo**: servidor HTTP + middlewares propios.
- **GORM + Postgres**: persistencia.
- **html/template (SSR)**: vistas accesibles y rápidas.
- **MercadoPago**: generación de preferencias (sandbox o prod según token y `APP_ENV`).
- **Storage local**: archivos en carpeta configurable (`STORAGE_DIR`, por defecto `uploads`).
- **Notificaciones**: Telegram y opcional email (SMTP) tras pago aprobado.
- **OAuth Google**: login rápido (opcional).
- **Admin JWT**: gestión segura de productos y órdenes.

```
/cmd/tienda3d          # main real
/internal/app          # wiring, inicialización, migraciones
/internal/domain       # entidades + puertos
/internal/usecase      # casos de uso
/internal/adapters     # http, repos, payments, storage
/internal/views        # templates SSR
/public/assets         # estáticos (css, imágenes públicas)
/uploads               # imágenes subidas (dinámicas)
```

## Autenticación Admin (JWT)
Requiere definir:
- `ADMIN_API_KEY`: clave secreta que se envía en el login.
- `ADMIN_ALLOWED_EMAILS`: lista separada por coma de emails autorizados (si se envía email en login se valida pertenezca).
- `JWT_ADMIN_SECRET` (opcional; si falta se usa `SECRET_KEY`).

Flujo:
1. `POST /admin/login` con header `X-Admin-Key: <ADMIN_API_KEY>` y opcional body `{ "email": "permitido@example.com" }`.
2. Respuesta contiene `{ token, exp, email }`.
3. Usar `Authorization: Bearer <token>` en llamadas a endpoints protegidos.
4. Expiración típica: 30 minutos (renovar con nuevo login).

Endpoints protegidos actuales: creación/listado/detalle/borrado de productos, upload multipart, borrado masivo y listado de órdenes `/admin/orders`.

## Variables de entorno (completas)
Obligatorias mínimas para entorno local:
- `DB_DSN` cadena Postgres (si usás docker compose se arma con variables POSTGRES_*)
- `SESSION_KEY` clave aleatoria segura (firmar cookies)
- `MP_ACCESS_TOKEN` token MercadoPago (TEST-... o prod)
- `ADMIN_API_KEY` (para login admin)
- `ADMIN_ALLOWED_EMAILS` (al menos un email, p.ej. tu correo)

Recomendadas / adicionales:
- `PORT` (default 8080)
- `APP_ENV` (`dev` | `production`) controla selección de token MP y logs
- `PROD_ACCESS_TOKEN` token MP producción (si querés separar del TEST)
- `PUBLIC_BASE_URL` URL pública (para back_urls y webhooks de MP)
- `BASE_URL` (usado también para OAuth Google)
- `SECRET_KEY` firma del external_reference MP (fallback "dev")
- `JWT_ADMIN_SECRET` secreto dedicado para firmar JWT admin (si no, usa `SECRET_KEY`)
- `STORAGE_DIR` carpeta para archivos subidos (default `uploads`)
- `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `ORDER_NOTIFY_EMAIL` (notificación email)
- `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID` o `TELEGRAM_CHAT_IDS` (notificación Telegram). `TELEGRAM_CHAT_IDS` permite múltiples destinos separados por coma, p. ej.: `-1001234567890,@SoyCanalla`.
- `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` (OAuth Google)

Docker / DB:
- `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `DB_PORT`, `APP_PORT`

MercadoPago alterno:
- `PROD_ACCESS_TOKEN` (solo si querés tener sandbox + prod separados)

### Ejemplo `.env.example`
```
POSTGRES_USER=postgres
POSTGRES_PASSWORD=change_me
POSTGRES_DB=tienda3d
DB_PORT=5432
APP_PORT=8080
DB_DSN=postgres://postgres:change_me@db:5432/tienda3d?sslmode=disable
SESSION_KEY=generate_secure_key
MP_ACCESS_TOKEN=TEST-xxxxxxxxxxxxxxxxxxxx
APP_ENV=dev
PUBLIC_BASE_URL=http://localhost:8080
BASE_URL=http://localhost:8080
STORAGE_DIR=uploads
SECRET_KEY=sign_ref_key
ADMIN_API_KEY=super_admin_key
ADMIN_ALLOWED_EMAILS=tuemail@example.com
JWT_ADMIN_SECRET=jwt_admin_secret
SMTP_HOST=
SMTP_PORT=587
SMTP_USER=
SMTP_PASS=
ORDER_NOTIFY_EMAIL=
TELEGRAM_BOT_TOKEN=
TELEGRAM_CHAT_ID=
GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=
```

## Docker Compose
Ahora parametrizado (ver `docker-compose.yml`):
```
docker compose up -d --build
```
Asegurate de crear `.env` a partir de `.env.example` antes de levantar.

## Flujos principales
### 1. Carga de productos
Opción recomendada: `POST /api/products/upload` (multipart) con uno o más campos `image` / `images`. El backend guarda cada archivo en `uploads/images/<timestamp>-<filename>` y registra las rutas. Requiere Bearer token admin.

### 2. Visualización
- `/products` listado con filtros.
- `/product/{slug}`: carrusel + swatches de color (si hay variantes se deducen los colores; de lo contrario se usa un set por defecto).

### 3. Carrito y Checkout
- Agregar desde el detalle (envía `slug` + `color`).
- Carrito `/cart`: editar cantidades, elegir envío o retiro. Todos los costos provinciales actualmente son 9000 (configurar en código `provinceCosts`).
- Checkout: botón MercadoPago genera preferencia (sandbox si token `TEST-` y no estás en producción).

### 4. Pagos y Webhooks
- Webhook MP: `/webhooks/mp` (configurar en MercadoPago a `PUBLIC_BASE_URL/webhooks/mp`).
- Página de estado `/pay/{orderID}` se usa como success/pending/failure.

### 5. Eliminación de productos
- `DELETE /api/products/{slug}` elimina DB + archivos (Bearer admin).
- `POST /api/products/delete` borrado masivo simple (Bearer admin) (no borra archivos físicos).

### 6. Órdenes (Admin)
- `GET /admin/orders` listado paginado de órdenes (Bearer admin). Útil para ver estado después de webhooks.

## Endpoints principales
Web (SSR): `/`, `/products`, `/product/{slug}`, `/cart`, `/checkout`, `/pay/{id}`

Admin / protegidos (Bearer):
- `POST /admin/login` (obtención token)
- `GET /admin/orders`
- `POST /api/products/upload`
- `POST /api/products`
- `GET /api/products`
- `GET /api/products/{slug}`
- `DELETE /api/products/{slug}`
- `POST /api/products/delete`

Públicos:
- `POST /api/quote`
- `POST /api/checkout`
- `POST /webhooks/mp`

## Desarrollo
```
go run ./cmd/tienda3d
# o
make dev
```
Docker:
```
docker compose up -d --build
```

## Pruebas manuales rápidas
1. Login admin (`POST /admin/login`).
2. Subir un producto (ver `carga.md`).
3. Ver en `/products`.
4. Agregar al carrito, cambiar cantidades.
5. Simular pago (token TEST) -> MP sandbox -> volver a `/pay/{id}`.
6. Ver `/admin/orders` para confirmar estado y logs para webhook.

## Notas
- No se crean productos demo automáticamente.
- Las imágenes dinámicas viven fuera de `/public` en `uploads/`.
- El slug se recalcula si faltaba (backfill en migración).
- Color picker: si no hay variantes, se muestran colores por defecto.
- Para producción: configurar `APP_ENV=production`, `PROD_ACCESS_TOKEN`, dominio HTTPS en `PUBLIC_BASE_URL`.
- JWT admin expira (≈30 min); re-logear para nuevo token.

---
Ver `carga.md` para detalles de carga y ejemplos curl.
