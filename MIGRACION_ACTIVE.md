# Migración del Campo `active` - Guía de Actualización

## 🔍 Problema Identificado

El **Watch Serie 10 42 mm** aparecía en la página web aunque estaba marcado como "Sin Stock" en `texto.txt`. 

**Causa raíz:** 
- Los productos existentes en la base de datos no tenían el campo `active` (o estaba en `NULL`)
- El filtro `WHERE active = true` no funcionaba correctamente con valores NULL
- Los productos deprecados seguían apareciendo en la web

## ✅ Solución Implementada

Se agregó una migración automática en `internal/app/app.go` que:

1. **Crea la columna `active`** si no existe (con valor por defecto `true`)
2. **Actualiza todos los productos existentes** a `active = true` 
3. **Crea un índice** en la columna para mejorar el rendimiento

## 🚀 Pasos para Aplicar la Actualización

### Opción 1: Docker Compose (Recomendado)

```bash
# 1. Detener los contenedores
docker-compose down

# 2. Rebuild y reiniciar (la migración se ejecuta automáticamente al iniciar)
docker-compose up --build -d

# 3. Ver los logs para confirmar que la migración se ejecutó
docker-compose logs -f app_mayorista
```

### Opción 2: Ejecución Local

```bash
# 1. Detener la aplicación si está corriendo
Ctrl+C

# 2. Compilar y ejecutar
go run cmd/tienda3d/main.go
```

## 📋 Qué Hace la Migración

```sql
-- 1. Agrega la columna si no existe
ALTER TABLE products ADD COLUMN IF NOT EXISTS active BOOLEAN DEFAULT true;

-- 2. Pone todos los productos existentes en activo
UPDATE products SET active = true WHERE active IS NULL;

-- 3. Crea índice para mejorar consultas
CREATE INDEX IF NOT EXISTS idx_products_active ON products(active);
```

## 🔄 Nuevo Comportamiento Post-Migración

### Primera Importación Después de la Migración

1. **Todos los productos arrancan en `active = true`** (gracias a la migración)
2. Al ejecutar la importación:
   - Se marcan TODOS como `active = false` al inicio
   - Los que vienen en los archivos → `active = true`
   - Los que NO vienen → quedan con `active = false` (deprecados)

### Productos que NO aparecerán más

- ❌ **Watch Serie 10 42 mm** (en `texto.txt` con "Sin Stock")
- ❌ Cualquier producto que no esté en `Colores.xlsx` NI en `texto.txt`
- ❌ Productos con precio `$0` o "Sin Stock" en `texto.txt`

### Productos que SÍ aparecerán

- ✅ **Watch Serie 11 42 mm** ($446 en `texto.txt`)
- ✅ **Watch Serie 11 46 mm** ($473 en `texto.txt`)
- ✅ Todos los productos con precio válido en `texto.txt`
- ✅ Productos en `Colores.xlsx` con precio en `texto.txt`
- ✅ Notebooks/tablets que solo están en `texto.txt` (sin colores)

## 🧪 Verificación

Después de reiniciar la aplicación:

### 1. Verificar que la migración se ejecutó

Buscar en los logs:
```
INF migrar/seed
```

### 2. Verificar en la base de datos (opcional)

```sql
-- Ver si la columna existe
SELECT column_name, data_type, column_default 
FROM information_schema.columns 
WHERE table_name = 'products' AND column_name = 'active';

-- Contar productos activos vs inactivos
SELECT active, COUNT(*) 
FROM products 
GROUP BY active;
```

Deberías ver todos los productos en `active = true` inicialmente.

### 3. Hacer la primera importación

1. Ir a `/admin/products`
2. Subir `Colores.xlsx` y `texto 15-11.txt`
3. Ingresar tipo de cambio y margen
4. Importar

Deberías ver en la respuesta:
```json
{
  "created_products": XX,
  "updated_products": YY,
  "deprecated_products": ZZ,  // ← Productos que se deprecaron
  ...
}
```

### 4. Verificar en la web

- Buscar "watch ser"
- **NO debería aparecer**: Watch Serie 10 42 mm
- **SÍ debería aparecer**: Watch Serie 11 42 mm y 46 mm

## 🎯 Resumen de Cambios

| Antes | Después |
|-------|---------|
| Productos sin `active` aparecían siempre | Solo aparecen los con `active=true` |
| "Sin Stock" en texto.txt se importaba | Se ignora y depreca |
| Productos viejos no se eliminaban | Se marcan como `active=false` |
| No había log de deprecados | Log completo con slugs deprecados |

## 🆘 Troubleshooting

### Si los productos viejos siguen apareciendo:

1. Verificar que la aplicación se reinició correctamente
2. Hacer una importación completa para que se aplique la nueva lógica
3. Verificar en la BD: `SELECT slug, active FROM products WHERE slug LIKE '%watch%';`

### Si necesitas reactivar un producto manualmente:

```sql
UPDATE products SET active = true WHERE slug = 'nombre-del-producto';
```

### Si necesitas ver todos los productos deprecados:

```sql
SELECT slug, name, base_price 
FROM products 
WHERE active = false 
ORDER BY name;
```

---

**Última actualización**: 16/11/2025

