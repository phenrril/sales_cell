# Guía de Importación de Productos 📦

## Resumen

El sistema permite importar productos masivamente usando **dos archivos**:
1. **XLSX de colores**: Con productos y sus colores/stock
2. **TXT de precios**: Con lista de precios en USD

## Cómo funciona

### Método Tradicional (Recomendado) ⚡

El método tradicional ahora incluye **normalización inteligente automática**:

#### ✅ **Normalizaciones automáticas**

1. **Capacidades**:
   - `256GB` → `256 GB`
   - `12/512GB` → `12/512 GB`
   - `1TB` → `1 TB`

2. **Sufijos ignorados**:
   - `5G DS` = `5G` = `DS` = (sin sufijo)
   - `WiFi` = `Wi-Fi` = (sin WiFi)
   - `4G` = `LTE` = (sin sufijo)

3. **Caracteres especiales**:
   - Espacios múltiples normalizados
   - `Samsung S25+` = `Samsung S25 +` = `Samsung S25`

4. **Colores removidos**:
   - `Moto G35 Negro` → `Moto G35`
   - `iPhone 17 Azul Oscuro` → `iPhone 17`
   - `(Negro,Blanco,Azul...)` → (removido)

5. **Matching fuzzy en 3 niveles**:
   - **Nivel 1**: Match exacto
   - **Nivel 2**: Match con normalizaciones
   - **Nivel 3**: Match parcial (contiene)

### Ejemplos de matching exitoso

| XLSX | Texto de precios | ¿Matchea? |
|------|------------------|-----------|
| `Moto G55 8/256 GB 5G DS (Negro)` | `Moto G55 8/256 GB 5G DS` | ✅ Sí |
| `Samsung S25+ 12/256GB` | `Samsung S25+ 12/256 GB 5G DS` | ✅ Sí |
| `iPhone 17 256 GB` | `iPhone 17 256GB` | ✅ Sí |
| `Xiaomi Redmi 15C 8/256 GB` | `Xiaomi Redmi 15C 8/256 GB` | ✅ Sí |

## Cómo importar

### Paso a paso

1. **Prepara tus archivos**:
   - XLSX con colores/stock
   - TXT con precios en formato: `Producto X GB   $XXX`

2. **En Admin > Productos**:
   - Click en "📄 XLSX Colores" → selecciona tu archivo
   - Click en "📄 Archivo de precios USD (.txt)" → selecciona tu texto
   - Ingresa TC USD→ARS (ej: `1485`)
   - Ingresa Margen % (ej: `20`)
   - **NO marques el checkbox de OpenAI** (usar método tradicional)
   - Click "⬆️ Importar"

3. **Espera ~1 segundo** (instantáneo)

4. **Revisa resultados**:
   - Alert mostrará cuántos se importaron
   - Ve a `/admin/uncharged` para ver los que no matchearon

### Logs en consola

```
INF importación tradicional completada creados=45 actualizados=38 sin_precio=9 tasa_match=90.2
```

- **creados**: Productos nuevos
- **actualizados**: Productos existentes con nuevo precio
- **sin_precio**: Productos que no matchearon
- **tasa_match**: % de éxito

## Formato de archivos

### Archivo XLSX (Colores.xlsx)

```
Columna A: (Categoría/Título de sección)
Columna B: Nombre del producto
Columna C: Color
Columna D: Stock (Disponible/Bajo/Sin Stock)
```

Ejemplo:
```
         B                           C        D
    Motorola                                  
    Moto G55 8/256 GB 5G DS      Negro     Disponible
    Moto G55 8/256 GB 5G DS      Blanco    Bajo
    Moto G55 8/256 GB 5G DS      Azul      Sin Stock
```

### Archivo TXT (texto.txt)

```
Nombre del producto    $precio_usd
```

Ejemplo:
```
Motorola
Moto G55 8/256 GB 5G DS   $255
Moto G35 4/256 GB 5G      $160
Samsung S25+ 12/256 GB 5G DS   $831
iPhone 17 256 GB    Sin Stock
```

**Importante**:
- Productos con "Sin Stock" → NO se importan (se ignoran)
- Si querés importarlos sin precio, poné `$0` en vez de "Sin Stock"
- Tabs o espacios entre nombre y precio están OK
- Líneas vacías se ignoran
- El sistema **agrupa duplicados** en el reporte: si un producto tiene 5 colores sin precio, se muestra 1 vez con "(×5 colores)"

## Comportamiento en importaciones sucesivas

### ✅ **Preservado (NO se toca)**

- **Imágenes de productos**: Se mantienen todas
- **Stock de variantes existentes**: Si el XLSX no trae stock o viene vacío, se mantiene el actual
- **Variantes no incluidas**: No se eliminan, solo se actualizan las que vienen

### 🔄 **Actualizado**

- **Precios**: Se actualizan según TC y márgenes nuevos
- **Stock**: Se actualiza solo si viene dato en XLSX
- **Nuevos colores**: Se agregan como variantes nuevas

## Reporte de importación

Después de importar, ve a **Admin > Sin precio** (`/admin/uncharged`) para ver:

- ✅ Productos creados (con links)
- ✅ Productos actualizados (con links)
- ✅ Variantes creadas por color
- ✅ Variantes actualizadas
- ⚠️ **Productos sin precio** (tabla con razones específicas)

### Razones de productos sin precio

El sistema ahora indica **por qué** cada producto no se importó:

| Ícono | Razón | Descripción | Solución |
|-------|-------|-------------|----------|
| 📦 | **Sin Stock** | Marcado como "Sin Stock" en archivo de precios | Normal, o cambiar a `$0` si querés importarlo |
| ❌ | **No en precios** | Producto no existe en archivo de precios | Agregar línea al archivo TXT |
| 🔀 | **Formato diferente** | Producto existe pero nombre no coincide | Corregir formato para que sean iguales |
| ⚠️ | **Precio inválido** | Encontrado pero precio no parseable | Verificar formato de precio en TXT |
| 🤖 | **OpenAI sin match** | OpenAI no pudo matchear (solo si usaste OpenAI) | Usar método tradicional |

## Solución de problemas

### Muchos productos "sin precio"

**Causas comunes**:

1. **Productos con "Sin Stock" en TXT**:
   - Estos se ignoran automáticamente
   - Solución: Si querés importarlos, cambiar `Sin Stock` por `$0`

2. **Nombres diferentes en XLSX vs TXT**:
   - Ejemplo: XLSX tiene `Moto G55 8/256` pero TXT dice `Moto G55 8-256`
   - El sistema intenta matchear pero puede fallar

**Cómo diagnosticar**:

1. Ve a `/admin/uncharged` después de importar
2. Verás una tabla con:
   ```
   Producto                      | Colores | Razón
   ----------------------------- | ------- | -------------------
   iPhone 17 256 GB              | ×5      | 📦 Sin Stock
   Moto G55 8/256 GB 5G DS       | ×2      | 🔀 Formato diferente
   Samsung A26 8/256 GB 5G DS    | ×3      | 📦 Sin Stock
   iPad Air 13" M3 256 GB WiFi   | ×1      | ❌ No en precios
   ```

3. **Interpretar las razones**:
   - **📦 Sin Stock**: Producto marcado como "Sin Stock" en el archivo de precios → Decisión tuya si importarlo
   - **❌ No en precios**: No existe en el archivo TXT → Agregar la línea de precio
   - **🔀 Formato diferente**: Nombres no coinciden → Unificar formato
   - **⚠️ Precio inválido**: Problema parseando precio → Verificar formato `$XXX`

**Soluciones**:
- Productos con "Sin Stock": Decidir si importarlos con `$0` o dejarlos
- Nombres inconsistentes: Usar el mismo formato en XLSX y TXT
- Productos faltantes: Agregar al archivo de precios
- Verificar logs de consola para ver intentos de match

### Productos duplicados

- El sistema usa el **slug** (nombre normalizado) como clave
- Si dos productos tienen nombres muy similares, se consideran el mismo
- Revisar el listado después de importar

### Stock no se actualiza

- Verificar que la columna D del XLSX tenga valores: "Disponible", "Bajo", "Sin Stock"
- Si viene vacía, se preserva el stock actual (comportamiento deseado)

## Tips para mejores resultados

1. **Consistencia en nombres**: 
   - Usar mismo formato en XLSX y TXT
   - Ejemplo: `Moto G55 8/256 GB 5G DS` en ambos

2. **Formato de capacidades**:
   - Puede ser `256GB` o `256 GB` (se normaliza automáticamente)
   - Puede ser `12/512GB` o `12/512 GB` (se normaliza)

3. **Sufijos opcionales**:
   - `5G DS`, `5G`, `DS` se ignoran al matchear
   - Podés tener `Moto G55` en XLSX y `Moto G55 5G DS` en TXT → matchea

4. **Revisar el reporte**:
   - Siempre revisar `/admin/uncharged` después de importar
   - Completar manualmente los que no matchearon

---

## Método OpenAI (Opcional, Experimental)

Si el método tradicional no funciona bien (< 80% match), podés probar con OpenAI:

- ✅ Checkbox "Usar OpenAI"
- ⚠️ Requiere API key en `.env`: `OPENAI_API_KEY=sk-...`
- ⚠️ Tarda 30-120 segundos
- ⚠️ Cuesta ~$0.02 USD por importación
- ⚠️ Experimental, puede tener timeouts

**Recomendación**: Usar método tradicional por defecto.

---

**Última actualización**: 30/09/2025

