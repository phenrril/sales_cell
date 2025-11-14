// Checkout por pasos - Navegación y validación
let currentStep = 1;
const totalSteps = 4;

// Datos del checkout guardados en sesión
let checkoutData = {
  step1: {},
  step2: {},
  step3: {},
  step4: {}
};

// Costos de envío por provincia
const provinceCosts = {};
(function() {
  const pcData = document.getElementById('pcData');
  if (pcData) {
    pcData.querySelectorAll('[data-prov]').forEach(el => {
      const prov = el.getAttribute('data-prov');
      const cost = parseFloat(el.getAttribute('data-cost') || '0');
      if (prov) {
        provinceCosts[prov] = cost;
      }
    });
  }
})();

// Cargar datos guardados al iniciar
(function() {
  loadCheckoutData();
  updateProgressBar();
  updateStepDisplay();
})();

function goToStep(step) {
  if (step < 1 || step > totalSteps) return;
  
  // Validar paso actual antes de avanzar
  if (step > currentStep) {
    if (!validateCurrentStep()) {
      return;
    }
  }
  
  currentStep = step;
  updateStepDisplay();
  updateProgressBar();
  updateTitle();
  saveCheckoutData();
}

function validateAndGoToStep(fromStep, toStep) {
  if (validateCurrentStep()) {
    goToStep(toStep);
  }
}

function validateCurrentStep() {
  const step = document.getElementById(`step${currentStep}`);
  if (!step) return true;
  
  switch(currentStep) {
    case 1:
      // Paso 1 no requiere validación adicional (solo productos)
      return true;
      
    case 2:
      return validateStep2();
      
    case 3:
      return validateStep3();
      
    case 4:
      return validateStep4();
      
    default:
      return true;
  }
}

function validateStep2() {
  const firstName = document.getElementById('firstName');
  const lastName = document.getElementById('lastName');
  const dni = document.getElementById('dni');
  const email = document.getElementById('email');
  const areaCode = document.getElementById('areaCode');
  const phoneNumber = document.getElementById('phoneNumber');
  
  let isValid = true;
  
  // Validar nombre
  if (!firstName.value.trim()) {
    showError(firstName, 'El nombre es obligatorio');
    isValid = false;
  } else {
    clearError(firstName);
  }
  
  // Validar apellido
  if (!lastName.value.trim()) {
    showError(lastName, 'El apellido es obligatorio');
    isValid = false;
  } else {
    clearError(lastName);
  }
  
  // Validar DNI (numérico, 7-8 dígitos)
  const dniValue = dni.value.trim();
  if (!dniValue) {
    showError(dni, 'El DNI es obligatorio');
    isValid = false;
  } else if (!/^\d{7,8}$/.test(dniValue)) {
    showError(dni, 'El DNI debe tener entre 7 y 8 dígitos numéricos');
    isValid = false;
  } else {
    clearError(dni);
  }
  
  // Validar email
  const emailValue = email.value.trim();
  if (!emailValue) {
    showError(email, 'El email es obligatorio');
    isValid = false;
  } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(emailValue)) {
    showError(email, 'El email no es válido');
    isValid = false;
  } else {
    clearError(email);
  }
  
  // Validar código de área
  if (!areaCode.value.trim()) {
    showError(areaCode, 'El código de área es obligatorio');
    isValid = false;
  } else {
    clearError(areaCode);
  }
  
  // Validar número de teléfono
  if (!phoneNumber.value.trim()) {
    showError(phoneNumber, 'El número de teléfono es obligatorio');
    isValid = false;
  } else {
    clearError(phoneNumber);
  }
  
  if (isValid) {
    // Guardar datos del paso 2
    checkoutData.step2 = {
      firstName: firstName.value.trim(),
      lastName: lastName.value.trim(),
      dni: dniValue,
      email: emailValue,
      phoneType: document.getElementById('phoneType').value,
      areaCode: areaCode.value.trim(),
      phoneNumber: phoneNumber.value.trim()
    };
    saveStepData(2, checkoutData.step2);
  }
  
  return isValid;
}

function validateStep3() {
  const shippingMethod = document.querySelector('input[name="shipping_method"]:checked');
  if (!shippingMethod) {
    alert('Por favor seleccioná un método de entrega');
    return false;
  }
  
  const method = shippingMethod.value;
  let isValid = true;
  
  if (method === 'retiro') {
    // Retiro no requiere campos adicionales
    checkoutData.step3 = { shipping_method: 'retiro' };
    saveStepData(3, checkoutData.step3);
    return true;
  }
  
  if (method === 'cadete') {
    const cadeteAddress = document.getElementById('cadeteAddress');
    if (!cadeteAddress.value.trim()) {
      showError(cadeteAddress, 'La dirección es obligatoria');
      isValid = false;
    } else {
      clearError(cadeteAddress);
      checkoutData.step3 = {
        shipping_method: 'cadete',
        address: cadeteAddress.value.trim()
      };
      saveStepData(3, checkoutData.step3);
    }
  }
  
  if (method === 'envio') {
    const postalCode = document.getElementById('postalCode');
    const province = document.getElementById('province');
    const locality = document.getElementById('locality');
    const street = document.getElementById('street');
    const streetNumber = document.getElementById('streetNumber');
    const noNumber = document.querySelector('input[name="no_number"]').checked;
    
    if (!postalCode.value.trim()) {
      showError(postalCode, 'El código postal es obligatorio');
      isValid = false;
    } else if (!/^\d{4,5}$/.test(postalCode.value.trim())) {
      showError(postalCode, 'El código postal debe tener 4 o 5 dígitos');
      isValid = false;
    } else {
      clearError(postalCode);
    }
    
    if (!province.value) {
      showError(province, 'La provincia es obligatoria');
      isValid = false;
    } else {
      clearError(province);
    }
    
    if (!locality.value.trim()) {
      showError(locality, 'La localidad es obligatoria');
      isValid = false;
    } else {
      clearError(locality);
    }
    
    if (!street.value.trim()) {
      showError(street, 'La calle es obligatoria');
      isValid = false;
    } else {
      clearError(street);
    }
    
    if (!noNumber && !streetNumber.value.trim()) {
      showError(streetNumber, 'La altura es obligatoria');
      isValid = false;
    } else {
      clearError(streetNumber);
    }
    
    if (isValid) {
      checkoutData.step3 = {
        shipping_method: 'envio',
        postal_code: postalCode.value.trim(),
        province: province.value,
        locality: locality.value.trim(),
        street: street.value.trim(),
        street_number: noNumber ? '' : streetNumber.value.trim(),
        floor: document.getElementById('floor').value.trim(),
        apartment: document.getElementById('apartment').value.trim(),
        delivery_notes: document.getElementById('deliveryNotes').value.trim()
      };
      saveStepData(3, checkoutData.step3);
    }
  }
  
  return isValid;
}

function validateStep4() {
  const paymentMethod = document.querySelector('input[name="payment_method"]:checked');
  if (!paymentMethod) {
    alert('Por favor seleccioná un método de pago');
    return false;
  }
  
  checkoutData.step4 = {
    payment_method: paymentMethod.value
  };
  saveStepData(4, checkoutData.step4);
  return true;
}

function selectShipping(method) {
  const shippingFields = document.getElementById('shippingFields');
  const envioFields = document.getElementById('envioFields');
  const cadeteFields = document.getElementById('cadeteFields');
  
  // Actualizar radio buttons
  document.querySelectorAll('input[name="shipping_method"]').forEach(radio => {
    radio.checked = radio.value === method;
  });
  
  // Mostrar/ocultar campos según método
  if (method === 'retiro') {
    shippingFields.style.display = 'none';
  } else {
    shippingFields.style.display = 'block';
    if (method === 'envio') {
      envioFields.style.display = 'block';
      cadeteFields.style.display = 'none';
    } else if (method === 'cadete') {
      envioFields.style.display = 'none';
      cadeteFields.style.display = 'block';
    }
  }
  
  // Actualizar resumen de envío y total
  updateShippingSummary();
  updateTotalSummary();
}

function selectPayment(method) {
  document.querySelectorAll('input[name="payment_method"]').forEach(radio => {
    radio.checked = radio.value === method;
  });
  updatePaymentSummary();
  updateTotalSummary();
}

function updateStepDisplay() {
  for (let i = 1; i <= totalSteps; i++) {
    const step = document.getElementById(`step${i}`);
    if (step) {
      step.style.display = i === currentStep ? 'block' : 'none';
    }
  }
  
  // Mostrar/ocultar botón de volver
  const btnBack = document.getElementById('btnBack');
  if (btnBack) {
    btnBack.style.display = currentStep > 1 ? 'flex' : 'none';
  }
}

function updateProgressBar() {
  const progress = ((currentStep - 1) / (totalSteps - 1)) * 100;
  const progressLine = document.getElementById('progressLine');
  if (progressLine) {
    progressLine.style.width = progress + '%';
  }
  
  // Actualizar círculos de pasos
  for (let i = 1; i <= totalSteps; i++) {
    const circle = document.getElementById(`stepCircle${i}`);
    const label = document.querySelector(`.progress-step[data-step="${i}"] .step-label`);
    
    if (circle && label) {
      if (i < currentStep) {
        // Paso completado
        circle.style.background = '#10b981';
        circle.style.color = '#fff';
        circle.innerHTML = '✓';
        label.style.color = '#10b981';
      } else if (i === currentStep) {
        // Paso actual
        circle.style.background = '#0076C7';
        circle.style.color = '#fff';
        circle.innerHTML = i;
        label.style.color = '#0076C7';
      } else {
        // Paso pendiente
        circle.style.background = '#e5e7eb';
        circle.style.color = '#6b7280';
        circle.innerHTML = i;
        label.style.color = '#6b7280';
      }
    }
  }
}

function updateTitle() {
  const titles = {
    1: 'Carrito de compras',
    2: 'Datos personales',
    3: 'Método de entrega',
    4: 'Método de pago'
  };
  const titleEl = document.getElementById('checkoutTitle');
  if (titleEl) {
    titleEl.textContent = titles[currentStep] || 'Carrito de compras';
  }
}

function showError(input, message) {
  clearError(input);
  input.style.borderColor = '#dc2626';
  const errorDiv = document.createElement('div');
  errorDiv.className = 'field-error';
  errorDiv.style.color = '#dc2626';
  errorDiv.style.fontSize = '12px';
  errorDiv.style.marginTop = '4px';
  errorDiv.textContent = message;
  input.parentNode.appendChild(errorDiv);
}

function clearError(input) {
  input.style.borderColor = '';
  const errorDiv = input.parentNode.querySelector('.field-error');
  if (errorDiv) {
    errorDiv.remove();
  }
}

function updateShippingSummary() {
  const shippingMethod = document.querySelector('input[name="shipping_method"]:checked');
  const shippingCostEl = document.getElementById('shippingCostSummary');
  
  if (!shippingMethod || !shippingCostEl) return;
  
  if (shippingMethod.value === 'retiro') {
    shippingCostEl.textContent = 'Gratis';
  } else if (shippingMethod.value === 'cadete') {
    shippingCostEl.textContent = '$5.000,00';
  } else if (shippingMethod.value === 'envio') {
    const province = document.getElementById('province');
    if (province && province.value && provinceCosts[province.value] !== undefined) {
      const cost = provinceCosts[province.value];
      shippingCostEl.textContent = cost > 0 ? `$${cost.toFixed(2).replace('.', ',')}` : 'Gratis';
    } else {
      shippingCostEl.textContent = 'Según provincia';
    }
  }
  
  updateTotalSummary();
}

function updatePaymentSummary() {
  const paymentMethod = document.querySelector('input[name="payment_method"]:checked');
  const discountSummary = document.getElementById('discountSummary');
  
  if (!paymentMethod) return;
  
  // Sin descuentos - siempre ocultar el descuento
  if (discountSummary) {
    discountSummary.style.display = 'none';
  }
  
  updateTotalSummary();
}

function updateTotalSummary() {
  // Obtener subtotal base
  const totalEl = document.getElementById('totalAmount');
  if (!totalEl) return;
  
  const baseTotal = parseFloat(totalEl.textContent.replace(/[^0-9.,]/g, '').replace(',', '.')) || 0;
  
  // Obtener costo de envío
  let shippingCost = 0;
  const shippingMethod = document.querySelector('input[name="shipping_method"]:checked');
  if (shippingMethod) {
    if (shippingMethod.value === 'cadete') {
      shippingCost = 5000;
    } else if (shippingMethod.value === 'envio') {
      const province = document.getElementById('province');
      if (province && province.value && provinceCosts[province.value] !== undefined) {
        shippingCost = provinceCosts[province.value];
      }
    }
  }
  
  // Sin descuentos - el cliente paga el precio total
  const discount = 0;
  
  // Calcular total (sin descuentos)
  const total = baseTotal + shippingCost;
  
  // Actualizar display
  if (totalEl) {
    totalEl.textContent = total.toFixed(2).replace('.', ',');
  }
  
  // Ocultar descuento siempre
  const discountSummary = document.getElementById('discountSummary');
  if (discountSummary) {
    discountSummary.style.display = 'none';
  }
}

// Guardar datos en sesión
async function saveStepData(step, data) {
  try {
    const response = await fetch('/api/checkout/step', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        step: step,
        data: data
      })
    });
    
    if (!response.ok) {
      console.error('Error guardando datos del paso', step);
    }
  } catch (error) {
    console.error('Error guardando datos:', error);
  }
}

// Cargar datos guardados
async function loadCheckoutData() {
  try {
    const response = await fetch('/api/checkout/data');
    if (response.ok) {
      const data = await response.json();
      if (data) {
        checkoutData = data;
        populateFormFields();
      }
    }
  } catch (error) {
    console.error('Error cargando datos:', error);
  }
}

// Guardar todos los datos del checkout
function saveCheckoutData() {
  // Los datos se guardan automáticamente al validar cada paso
}

// Poblar campos del formulario con datos guardados
function populateFormFields() {
  // Paso 2
  if (checkoutData.step2) {
    const data = checkoutData.step2;
    if (data.firstName) document.getElementById('firstName').value = data.firstName;
    if (data.lastName) document.getElementById('lastName').value = data.lastName;
    if (data.dni) document.getElementById('dni').value = data.dni;
    if (data.email) document.getElementById('email').value = data.email;
    if (data.phoneType) document.getElementById('phoneType').value = data.phoneType;
    if (data.areaCode) document.getElementById('areaCode').value = data.areaCode;
    if (data.phoneNumber) document.getElementById('phoneNumber').value = data.phoneNumber;
  }
  
  // Paso 3
  if (checkoutData.step3) {
    const data = checkoutData.step3;
    if (data.shipping_method) {
      selectShipping(data.shipping_method);
      if (data.shipping_method === 'envio') {
        if (data.postal_code) document.getElementById('postalCode').value = data.postal_code;
        if (data.province) document.getElementById('province').value = data.province;
        if (data.locality) document.getElementById('locality').value = data.locality;
        if (data.street) document.getElementById('street').value = data.street;
        if (data.street_number) document.getElementById('streetNumber').value = data.street_number;
        if (data.floor) document.getElementById('floor').value = data.floor;
        if (data.apartment) document.getElementById('apartment').value = data.apartment;
        if (data.delivery_notes) document.getElementById('deliveryNotes').value = data.delivery_notes;
      } else if (data.shipping_method === 'cadete') {
        if (data.address) document.getElementById('cadeteAddress').value = data.address;
      }
    }
  }
  
  // Paso 4
  if (checkoutData.step4) {
    const data = checkoutData.step4;
    if (data.payment_method) {
      selectPayment(data.payment_method);
    }
  }
}

// Finalizar checkout
async function finalizeCheckout() {
  if (!validateStep4()) {
    return;
  }
  
  // Mostrar loading
  const btn = event.target;
  const originalText = btn.textContent;
  btn.disabled = true;
  btn.textContent = 'Procesando...';
  
  // Validar que todos los datos necesarios estén presentes
  if (!checkoutData.step2 || !checkoutData.step3 || !checkoutData.step4) {
    alert('Por favor completá todos los pasos antes de finalizar');
    btn.disabled = false;
    btn.textContent = originalText;
    return;
  }
  
  try {
    // Enviar todos los datos al backend
    const response = await fetch('/cart/checkout', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        step2: checkoutData.step2,
        step3: checkoutData.step3,
        step4: checkoutData.step4
      })
    });
    
    if (!response.ok) {
      let errorMessage = 'Error al procesar la compra';
      try {
        const error = await response.json();
        errorMessage = error.error || error.message || errorMessage;
        console.error('Error del servidor:', error);
        
        // Mensajes más específicos para errores comunes
        if (errorMessage.includes('credenciales de MercadoPago') || errorMessage.includes('MP_ACCESS_TOKEN')) {
          errorMessage = 'Error de configuración: Las credenciales de MercadoPago no son válidas. Por favor contactá al administrador.';
        } else if (errorMessage.includes('403') || errorMessage.includes('UNAUTHORIZED')) {
          errorMessage = 'Error de autenticación con MercadoPago. Por favor contactá al administrador.';
        }
      } catch (e) {
        // Si no se puede parsear el JSON, usar el mensaje por defecto
        errorMessage = `Error ${response.status}: ${response.statusText}`;
        console.error('Error parseando respuesta:', e);
      }
      alert(errorMessage);
      btn.disabled = false;
      btn.textContent = originalText;
      return;
    }
    
    const result = await response.json();
    console.log('Respuesta del servidor:', result);
    
    if (result.success) {
      if (result.redirect_url) {
        // Verificar si es una URL de MercadoPago o una ruta local
        if (result.redirect_url.startsWith('http://') || result.redirect_url.startsWith('https://')) {
          // Es una URL de MercadoPago, redirigir directamente
          window.location.href = result.redirect_url;
        } else {
          // Es una ruta local, redirigir también
          window.location.href = result.redirect_url;
        }
      } else if (result.order_id) {
        // Orden creada, redirigir a página de confirmación
        window.location.href = `/pay/${result.order_id}`;
      } else {
        // Fallback: redirigir al carrito con mensaje de éxito
        window.location.href = '/cart?success=1';
      }
    } else {
      alert(result.error || result.message || 'Error al procesar la compra');
      btn.disabled = false;
      btn.textContent = originalText;
    }
  } catch (error) {
    console.error('Error finalizando checkout:', error);
    alert('Error de conexión. Por favor verificá tu conexión e intentá nuevamente.');
    btn.disabled = false;
    btn.textContent = originalText;
  }
}

// Event listeners
document.addEventListener('DOMContentLoaded', function() {
  // Listener para cambios en método de envío
  document.querySelectorAll('input[name="shipping_method"]').forEach(radio => {
    radio.addEventListener('change', function() {
      selectShipping(this.value);
    });
  });
  
  // Listener para cambios en método de pago
  document.querySelectorAll('input[name="payment_method"]').forEach(radio => {
    radio.addEventListener('change', function() {
      selectPayment(this.value);
    });
  });
  
  // Listener para cambios en provincia (actualizar costo de envío)
  const provinceSelect = document.getElementById('province');
  if (provinceSelect) {
    provinceSelect.addEventListener('change', function() {
      updateShippingSummary();
      updateTotalSummary();
    });
  }
  
  // Inicializar resumen
  updateShippingSummary();
  updatePaymentSummary();
  updateTotalSummary();
});

