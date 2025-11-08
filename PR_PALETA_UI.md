# PR: Nueva paleta de colores y soporte dark mode

## Descripción
Se implementa una nueva paleta de colores consistente para todo el sitio, con variables CSS para modo claro y oscuro, utilidades semánticas y mapeo de componentes clave (header, nav, footer, botones, links, cards, inputs, formularios, tablas, paginación, badges, alerts). Se asegura contraste mínimo AA en tipografía y estados interactivos.

## Cambios principales
- Variables CSS globales en `public/assets/styles.css`:
  - `--color-bg`, `--color-surface`, `--color-text`, `--color-muted`, `--color-border`
  - `--color-brand`, `--color-brand-hover`, `--color-accent`, `--color-success`, `--color-warning`, `--color-danger`
  - Perfil `.dark` para modo oscuro (activable con clase en `html` o `body`).
- Utilidades semánticas:
  - `.bg-bg`, `.bg-surface`, `.text-text`, `.text-muted`, `.border-border`, `.bg-brand`, `.hover:bg-brand-hover`, `.text-brand`, `.hover:text-brand-hover`, `.ring-brand`.
- Componentes actualizados para usar tokens: header/nav, promobar, subnav, hero, cards, product detail, tablas del carrito, formularios/inputs, drawer/sheet, paginación, footer, etc.
- Checkout (`internal/views/checkout.html`) refactorizado a tokens.

## Accesibilidad
- Texto sobre fondos `--color-bg` y `--color-surface` usa `--color-text`.
- Botón primario usa texto blanco sobre `--color-brand`/`--color-brand-hover`.
- Focus visible con `.ring-brand` (outline de marca).

## Cómo activar Dark Mode
- Agregar la clase `dark` en `html` o `body` para forzar modo oscuro.
  - Ej: `<body class="site dark">`.

## Verificación visual (QA)
Marcar cada vista como OK tras validar contrastes, estados hover/focus y consistencia de tokens.

- Home
  - [ ] Hero: textos y CTA.
  - [ ] Grid de productos/cards.
  - [ ] Banners/carousels.
- PLP/Category
  - [ ] Filtros (drawer/sheet), chips, contadores.
  - [ ] Cards y paginación.
- PDP
  - [ ] Galería, thumbs, navegación, nota informativa.
  - [ ] CTA “Agregar al carrito”.
- Carrito/Checkout
  - [ ] Tabla, cantidades, totales.
  - [ ] Formulario checkout (inputs, focus, botón primario).
- Login/Registro (si aplica)
  - [ ] Inputs y estados de foco.
- 404/Empty states
  - [ ] Tipografía y links.
- Header/Nav/Footer
  - [ ] Bordes, sombras y contraste de texto.

## Notas
- No se modifican imágenes ni logos.
- Se mantienen espaciados y tipografías existentes.
- Se evitaron rojos como acción por defecto, `danger` reservado a errores.
