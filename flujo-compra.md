# Diagrama de Flujo - Proceso de Compra

```mermaid
flowchart TD
    Start([Usuario en página de producto])
    
    %% Agregar al Carrito
    Start --> SelectProduct[Usuario selecciona producto y color]
    SelectProduct --> AddToCart{Agregar al carrito?}
    AddToCart -->|POST /cart| ValidateProduct[Validar producto existe]
    ValidateProduct -->|Producto no encontrado| ErrorProduct[Error 404]
    ValidateProduct -->|Producto encontrado| NormalizeColor[Normalizar color<br/>Usar primer color disponible<br/>o #111827 por defecto]
    NormalizeColor --> SaveCart[Guardar item en cookie/sesión<br/>Slug + Color + Qty: 1 + Price]
    SaveCart --> ResponseCart{Respuesta}
    ResponseCart -->|JSON| ReturnJSON[Respuesta JSON<br/>status: ok, items count]
    ResponseCart -->|Redirect| RedirectProduct[Redirect a /product/{slug}?added=1]
    
    %% Ver Carrito
    ReturnJSON --> ViewCart[Ver carrito]
    RedirectProduct --> ViewCart
    ViewCart -->|GET /cart| ReadCart[Leer items de cookie/sesión]
    ReadCart --> Aggregate[Agregar items por Slug+Color<br/>Buscar productos en BD]
    Aggregate --> CalcTotal[Calcular total]
    CalcTotal --> ShowCart[Mostrar carrito con:<br/>- Líneas agregadas<br/>- Total<br/>- Provincias disponibles]
    
    %% Actualizar Carrito
    ShowCart --> UpdateCart{Acción en carrito?}
    UpdateCart -->|Incrementar/Decrementar| UpdateQty[POST /cart/update<br/>op: inc/dec/set]
    UpdateQty --> RecalcCart[Recalcular cantidades<br/>Eliminar si qty = 0]
    RecalcCart --> SaveUpdatedCart[Guardar carrito actualizado]
    SaveUpdatedCart --> RefreshCart[Redirect a /cart]
    RefreshCart --> ShowCart
    
    UpdateCart -->|Eliminar item| RemoveItem[POST /cart/remove<br/>Slug + Color]
    RemoveItem --> RemoveFromCart[Eliminar item del carrito]
    RemoveFromCart --> RefreshCart
    
    %% Iniciar Checkout
    UpdateCart -->|Finalizar compra| InitCheckout[Iniciar checkout<br/>Paso 1: Revisar carrito]
    InitCheckout --> Step2[Paso 2: Datos personales<br/>Email, Nombre, Apellido<br/>DNI, Teléfono]
    Step2 --> Step3[Paso 3: Método de envío<br/>- Envío: provincia, dirección,<br/>código postal, notas<br/>- Cadete: dirección, provincia<br/>- Retiro: sin datos adicionales]
    Step3 --> Step4[Paso 4: Método de pago<br/>- Transferencia: 15% descuento<br/>- MercadoPago: sin descuento]
    
    %% Procesar Checkout
    Step4 --> FinalizeCheckout[POST /cart/checkout<br/>JSON: step2, step3, step4]
    FinalizeCheckout --> ValidateCheckout{Validar datos?}
    
    ValidateCheckout -->|Envío sin datos| ErrorEnvio[Error: faltan datos de envío]
    ValidateCheckout -->|DNI/Código Postal inválido| ErrorFormat[Error: formato inválido]
    ValidateCheckout -->|Carrito vacío| ErrorEmpty[Error: carrito vacío]
    ValidateCheckout -->|Datos válidos| ReadCartItems[Leer items del carrito]
    
    ReadCartItems --> FindOrCreateCustomer[Buscar cliente por email]
    FindOrCreateCustomer -->|No existe| CreateCustomer[Crear nuevo cliente<br/>ID, Email, Name, Phone]
    FindOrCreateCustomer -->|Existe| UpdateCustomer[Actualizar cliente<br/>Name, Phone]
    CreateCustomer --> CreateOrder
    UpdateCustomer --> CreateOrder
    
    %% Crear Orden
    CreateOrder[Crear orden:<br/>- Status: awaiting_payment<br/>- Email, Name, Phone, DNI<br/>- ShippingMethod, PaymentMethod<br/>- CustomerID]
    CreateOrder --> AddOrderItems[Agregar items de carrito<br/>a OrderItems]
    AddOrderItems --> CalcShipping[Calcular costo envío:<br/>- Envío: según provincia<br/>- Cadete: $5000<br/>- Retiro: $0]
    CalcShipping --> CalcSubtotal[Calcular subtotal<br/>Items + Envío]
    CalcSubtotal --> ApplyDiscount{Método de pago?}
    ApplyDiscount -->|Transferencia| Discount15[Descuento 15%<br/>DiscountAmount = Subtotal * 0.15]
    ApplyDiscount -->|MercadoPago| NoDiscount[Sin descuento]
    Discount15 --> CalcTotalOrder[Total = Subtotal - Discount]
    NoDiscount --> CalcTotalOrder
    CalcTotalOrder --> SaveOrder[Guardar orden en BD]
    
    SaveOrder --> ClearCart[Limpiar carrito y<br/>datos de checkout]
    ClearCart --> RoutePayment{Método de pago?}
    
    %% Transferencia
    RoutePayment -->|Transferencia| TransferOrder[Actualizar orden:<br/>Status: awaiting_payment<br/>MPStatus: transferencia_pending]
    TransferOrder --> SendNotifyPending[Enviar email<br/>notificación pendiente]
    SendNotifyPending --> RedirectTransfer[Redirect a /pay/{order_id}?status=pending]
    
    %% MercadoPago
    RoutePayment -->|MercadoPago| ValidateMP{MP disponible?}
    ValidateMP -->|No| ErrorMP[Error: servicio no disponible]
    ValidateMP -->|Sí| CreateMPPreference[Crear preferencia MP<br/>Items, Payer, BackURLs<br/>NotificationURL: /webhooks/mp<br/>ExternalReference: order_id|signature]
    CreateMPPreference -->|Error| ErrorMPCreate[Error al crear preferencia]
    CreateMPPreference -->|Éxito| SaveMPPrefID[Guardar MPPreferenceID<br/>en orden]
    SaveMPPrefID --> RedirectMP[Redirect a URL de MercadoPago]
    
    %% Página de Pago
    RedirectTransfer --> PayPage[Página /pay/{order_id}]
    RedirectMP --> ProcessMPPayment[Usuario paga en MercadoPago]
    ProcessMPPayment --> MPReturn[MercadoPago retorna a<br/>/pay/{order_id}?status=approved/pending]
    MPReturn --> PayPage
    
    %% Webhook MercadoPago
    ProcessMPPayment --> WebhookMP[MercadoPago envía webhook<br/>POST /webhooks/mp]
    WebhookMP --> ExtractPaymentID[Extraer payment ID]
    ExtractPaymentID --> GetPaymentInfo[GET /v1/payments/{id}<br/>Obtener status y external_reference]
    GetPaymentInfo --> VerifyRef[Verificar external_reference<br/>order_id|signature]
    VerifyRef -->|Válido| FindOrderMP[Buscar orden por ID]
    VerifyRef -->|Inválido| WebhookEnd[Responder 200 OK]
    FindOrderMP --> UpdateOrderStatus{Status del pago?}
    UpdateOrderStatus -->|approved| ApproveOrder[Actualizar orden:<br/>MPStatus: approved<br/>Status: finished]
    UpdateOrderStatus -->|pending/in_process| PendingOrder[Actualizar orden:<br/>MPStatus: pending<br/>Status: awaiting_payment]
    UpdateOrderStatus -->|rejected| RejectOrder[Actualizar orden:<br/>MPStatus: rejected<br/>Status: cancelled]
    
    ApproveOrder --> CheckNotified{Notified = false?}
    CheckNotified -->|Sí| SetNotified[Notified = true]
    SetNotified --> SaveOrderWebhook[Guardar orden]
    SaveOrderWebhook --> SendConfirmEmail[Enviar email<br/>confirmación asíncrono]
    SendConfirmEmail --> WebhookEnd
    
    PendingOrder --> SaveOrderWebhook
    RejectOrder --> SaveOrderWebhook
    
    %% Página de estado de pago
    PayPage --> CheckPayStatus[Verificar status de orden]
    CheckPayStatus -->|pending| ShowPending[Mostrar: Pedido recibido<br/>Por favor realiza transferencia]
    CheckPayStatus -->|approved| ShowApproved[Mostrar: Pago aprobado<br/>Gracias por tu compra]
    CheckPayStatus -->|rejected| ShowRejected[Mostrar: Pago rechazado]
    
    %% Confirmación manual (Admin)
    ShowPending --> AdminConfirm{Admin confirma pago?}
    AdminConfirm -->|POST /admin/confirm-payment| ValidateAdmin[Validar sesión admin]
    ValidateAdmin --> ValidateTransferMethod{Es transferencia?}
    ValidateTransferMethod -->|No| ErrorNotTransfer[Error: no es transferencia]
    ValidateTransferMethod -->|Sí| ManualApprove[Actualizar orden:<br/>Status: finished<br/>MPStatus: approved<br/>Notified: true]
    ManualApprove --> SaveManualOrder[Guardar orden]
    SaveManualOrder --> SendManualEmail[Enviar email<br/>confirmación asíncrono]
    
    %% Estados finales
    SendConfirmEmail --> OrderFinished([Orden finalizada])
    SendManualEmail --> OrderFinished
    ShowApproved --> OrderFinished
    ShowRejected --> OrderCancelled([Orden cancelada])
    ErrorProduct --> EndError([Error])
    ErrorEnvio --> EndError
    ErrorFormat --> EndError
    ErrorEmpty --> EndError
    ErrorMP --> EndError
    ErrorMPCreate --> EndError
    
    style Start fill:#e1f5ff
    style OrderFinished fill:#c8e6c9
    style OrderCancelled fill:#ffcdd2
    style EndError fill:#ffcdd2
    style ProcessMPPayment fill:#fff9c4
    style WebhookMP fill:#fff9c4
    style AdminConfirm fill:#f3e5f5
```

## Descripción de los Procesos Principales

### 1. Agregar al Carrito
- El usuario selecciona un producto y un color en la página del producto
- Se envía POST a `/cart` con `slug` y `color`
- Si el color está vacío, se usa el primer color disponible o `#111827` por defecto
- El color se normaliza a nombre genérico
- Se guarda en cookie/sesión como `cartPayload` con items `{slug, color, qty: 1, price}`
- Respuesta: JSON con status o redirect a página del producto

### 2. Ver y Editar Carrito
- GET `/cart` lee items de cookie/sesión
- Se agregan items por `slug+color` para evitar duplicados
- Se buscan productos en BD para obtener nombre, imagen y precio actualizado
- Se calcula el total
- Se pueden incrementar/decrementar cantidades o eliminar items
- POST `/cart/update` permite `inc`, `dec` o `set` cantidad
- POST `/cart/remove` elimina un item específico

### 3. Checkout por Pasos
El checkout se realiza en 4 pasos:

**Paso 1**: Revisar carrito

**Paso 2**: Datos personales
- Email, Nombre, Apellido
- DNI, Código de área, Teléfono

**Paso 3**: Método de envío
- **Envío**: Requiere provincia, dirección, código postal, DNI, notas de entrega
- **Cadete**: Requiere dirección (provincia por defecto: Santa Fe)
- **Retiro**: Sin datos adicionales

**Paso 4**: Método de pago
- **Transferencia**: Aplica 15% de descuento
- **MercadoPago**: Sin descuento

### 4. Procesamiento de Checkout
- POST `/cart/checkout` con JSON `{step2, step3, step4}`
- Validaciones:
  - Carrito no vacío
  - Datos de envío completos (si aplica)
  - DNI: 7-8 dígitos
  - Código postal: 4-5 dígitos
- Se busca o crea cliente en BD
- Se crea orden con estado `awaiting_payment`
- Se calculan costos:
  - Envío según provincia (ej: $9000)
  - Cadete: $5000
  - Descuento 15% si es transferencia
- Total = (Items + Envío) - Descuento

### 5. Proceso de Pago

#### Transferencia Bancaria:
- Orden queda en estado `awaiting_payment` con `MPStatus: transferencia_pending`
- Se envía email de notificación pendiente
- Redirect a `/pay/{order_id}?status=pending`
- Admin puede confirmar manualmente el pago vía `/admin/confirm-payment`
- Al confirmar: orden pasa a `finished`, se envía email de confirmación

#### MercadoPago:
- Se crea preferencia de pago con:
  - Items de la orden
  - Email del pagador
  - BackURLs (success, pending, failure) → `/pay/{order_id}`
  - NotificationURL: `/webhooks/mp`
  - ExternalReference: `{order_id}|{signature}`
- Redirect a URL de MercadoPago
- Usuario completa el pago en MercadoPago
- MercadoPago retorna a `/pay/{order_id}` y envía webhook

### 6. Webhook de MercadoPago
- POST `/webhooks/mp` recibe notificación
- Se extrae el `payment_id` del webhook
- Se consulta estado del pago en API de MercadoPago
- Se verifica `external_reference` para obtener `order_id`
- Se actualiza orden según status:
  - `approved`: `finished`, envía email confirmación
  - `pending/in_process`: `awaiting_payment`
  - `rejected`: `cancelled`

### 7. Estados de la Orden
- `pending_quote`: Pendiente de cotización
- `quoted`: Cotizado
- `awaiting_payment`: Esperando pago
- `in_print`: En impresión
- `finished`: Finalizada (pago confirmado)
- `shipped`: Enviada
- `cancelled`: Cancelada

### 8. Notificaciones por Email
- Se envían emails asíncronamente cuando:
  - Se crea orden pendiente (transferencia)
  - Pago aprobado (MercadoPago o confirmación manual)
- El campo `Notified` previene envíos duplicados
