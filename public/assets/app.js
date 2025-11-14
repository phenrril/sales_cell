// UI scripts unificados (module)
// - Nav drawer
// - Carousel home
// - Modal "Cómo comprar"
// - Products drawer/sheet + load more
// - Carrito: cálculo de envío
// - SW registration (idle)
// - Dark mode toggle
// - Footer reveal on scroll

// Nav drawer
(function(){
  const btn=document.querySelector('.nav-toggle');
  const body=document.body;
  const backdrop=document.querySelector('.nav-backdrop');
  if(!btn)return;
  function close(){body.classList.remove('nav-open');btn.setAttribute('aria-expanded','false');}
  btn.addEventListener('click',()=>{const open=body.classList.toggle('nav-open');btn.setAttribute('aria-expanded',open);});
  backdrop&&backdrop.addEventListener('click',close);
  window.addEventListener('keydown',e=>{if(e.key==='Escape' && body.classList.contains('nav-open')) close();});
})();

// Dark mode removido - solo modo claro

// Footer: revelar al final del scroll con IntersectionObserver
(function(){
  const sentinel=document.getElementById('footerSentinel');
  const footer=document.querySelector('.footer');
  if(!sentinel||!footer||!('IntersectionObserver' in window)){
    if(footer) footer.classList.add('revealed');
    return;
  }
  const io=new IntersectionObserver((entries)=>{
    for(const e of entries){
      if(e.isIntersecting){ footer.classList.add('revealed'); }
      else { footer.classList.remove('revealed'); }
    }
  },{root:null,threshold:0.01});
  io.observe(sentinel);
})();

// Home carousel + modal
(function(){
  // Nuevo: Banner carousel full width
  const bRoot=document.getElementById('bannerCarousel');
  if(bRoot){
    const interval=parseInt(bRoot.getAttribute('data-interval')||'7000',10);
    const bSlides=[...bRoot.querySelectorAll('.bc-slide')];
    const dotsC=document.getElementById('bcDots');
    bSlides.forEach((_,i)=>{const d=document.createElement('div');d.className='dot'+(i===0?' active':'');d.dataset.i=i;d.onclick=()=>go(i,true);dotsC&&dotsC.appendChild(d);});
    let idx=0,timer=null;function go(n,manual){if(!bSlides.length) return; bSlides[idx].classList.remove('active'); if(dotsC&&dotsC.children[idx]) dotsC.children[idx].classList.remove('active'); idx=(n+bSlides.length)%bSlides.length; bSlides[idx].classList.add('active'); if(dotsC&&dotsC.children[idx]) dotsC.children[idx].classList.add('active'); if(manual){restart();}}
    function next(){go(idx+1);} function start(){timer=setInterval(next,interval);} function restart(){clearInterval(timer);start();} start();
    // Swipe
    let startX=0,isSwiping=false;
    bRoot.addEventListener('touchstart',e=>{startX=e.touches[0].clientX;isSwiping=true;},{passive:true});
    bRoot.addEventListener('touchmove',e=>{if(!isSwiping)return;const dx=e.touches[0].clientX-startX;if(Math.abs(dx)>10){e.preventDefault();}},{passive:false});
    bRoot.addEventListener('touchend',e=>{if(!isSwiping)return;isSwiping=false;const dx=e.changedTouches[0].clientX-startX;if(Math.abs(dx)>50){go(dx>0?idx-1:idx+1,true);}},{passive:true});
  }
  const btn=document.getElementById('btnHowBuy');
  const bd=document.getElementById('howBuyBackdrop');
  const modal=bd?bd.querySelector('.modal'):null;
  const closeBtn=document.getElementById('howBuyClose');
  const okBtn=document.getElementById('howBuyOk');
  function open(){ if(!bd||!modal) return; bd.hidden=false; bd.classList.add('show'); modal.classList.add('show'); document.body.style.overflow='hidden'; closeBtn&&closeBtn.focus(); }
  function close(){ if(!bd||!modal) return; bd.classList.remove('show'); modal.classList.remove('show'); document.body.style.overflow=''; setTimeout(()=>{bd.hidden=true;},140); btn&&btn.focus(); }
  btn&&btn.addEventListener('click',open);
  closeBtn&&closeBtn.addEventListener('click',close);
  okBtn&&okBtn.addEventListener('click',close);
  bd&&bd.addEventListener('click',e=>{if(e.target===bd) close();});
  document.addEventListener('keydown',e=>{if(e.key==='Escape' && bd && !bd.hidden) close();});
})();

// Products drawer/sheet + chips + load more
(function(){
  const filterBtn=document.querySelector('.btn-filter');
  const sortBtn=document.querySelector('.btn-sort');
  const drawer=document.getElementById('filterDrawer');
  const drawerClose=drawer?drawer.querySelector('.drawer-close'):null;
  const panel=drawer?drawer.querySelector('.drawer-panel'):null;
  const sheet=document.getElementById('sortSheet');
  const form=document.getElementById('filtersForm');
  const sortInput=document.getElementById('sortInput');

  function openDrawer(){
    if(!drawer) return;
    drawer.hidden=false;
    drawer.setAttribute('aria-hidden','false');
    filterBtn&&filterBtn.setAttribute('aria-expanded','true');
    document.body.classList.add('drawer-open');
    const first=panel&&panel.querySelector('input, select, button');
    if(first) first.focus();
  }
  function closeDrawer(){
    if(!drawer) return;
    drawer.hidden=true;
    drawer.setAttribute('aria-hidden','true');
    filterBtn&&filterBtn.setAttribute('aria-expanded','false');
    document.body.classList.remove('drawer-open');
  }
  function openSheet(){ if(!sheet) return; sheet.hidden=false; sortBtn&&sortBtn.setAttribute('aria-expanded','true'); }
  function closeSheet(){ if(!sheet) return; sheet.hidden=true;  sortBtn&&sortBtn.setAttribute('aria-expanded','false'); }

  filterBtn&&filterBtn.addEventListener('click',openDrawer);
  drawerClose&&drawerClose.addEventListener('click',closeDrawer);
  sortBtn&&sortBtn.addEventListener('click',()=>{ if(sheet&&sheet.hidden){ openSheet(); } else { closeSheet(); } });
  window.addEventListener('keydown',(e)=>{
    if(e.key==='Escape'){
      if(drawer&&!drawer.hidden) closeDrawer();
      if(sheet&&!sheet.hidden) closeSheet();
    }
  });
  if(drawer){ drawer.addEventListener('click',(e)=>{ if(e.target===drawer) closeDrawer(); }); }

  // Ordenar: setear hidden input y enviar el form
  document.querySelectorAll('#sortSheet [data-sort]').forEach(btn=>{
    btn.addEventListener('click',()=>{
      if(!sortInput || !form) return;
      sortInput.value=btn.getAttribute('data-sort')||'';
      form.submit();
    });
  });

  // Chips limpiar (si existen)
  document.querySelectorAll('.chip[data-clear]').forEach(btn=>{
    btn.addEventListener('click',()=>{
      const key=btn.getAttribute('data-clear');
      const url=new URL(window.location.href);
      if(key) url.searchParams.delete(key);
      window.location.href=url.pathname+(url.searchParams.toString()?"?"+url.searchParams.toString():"");
    });
  });

  const loadBtn=document.getElementById('loadMore');
  const cards=document.querySelector('.cards');
  const statusEl=document.getElementById('loadMoreStatus');
  function announce(msg){if(statusEl){statusEl.textContent=msg;}}
  if(loadBtn && cards){
    loadBtn.addEventListener('click',async()=>{
      const next=loadBtn.getAttribute('data-next');
      if(!next){loadBtn.disabled=true;return}
      const oldText=loadBtn.textContent; loadBtn.disabled=true; loadBtn.textContent='Cargando...'; announce('Cargando más productos');
      try{const res=await fetch(next,{credentials:'same-origin'}); if(!res.ok) throw new Error('HTTP '+res.status); const html=await res.text(); const parser=new DOMParser(); const doc=parser.parseFromString(html,'text/html'); const newCards=doc.querySelectorAll('.cards > .card'); newCards.forEach(n=>cards.appendChild(n)); const newBtn=doc.getElementById('loadMore'); const newNext=newBtn?newBtn.getAttribute('data-next'):''; if(newNext){loadBtn.setAttribute('data-next',newNext);loadBtn.disabled=false;loadBtn.textContent=oldText;announce('Se cargaron más productos');} else {loadBtn.setAttribute('data-next','');loadBtn.disabled=true;loadBtn.textContent='No hay más';announce('No hay más productos');}}
      catch(err){loadBtn.disabled=false; loadBtn.textContent=oldText; announce('Error al cargar');}
    });
  }
})();

// Carrito: cálculo de envío y total (compatible con CSP)
(function(){
  const form=document.getElementById('checkoutForm'); if(!form) return;
  const shipRadios=form.querySelectorAll('input[name="shipping"]');
  const paymentRadios=form.querySelectorAll('input[name="payment_method"]');
  const envioGroup=document.getElementById('envioGroup');
  const cadeteGroup=document.getElementById('cadeteGroup');
  const provinceSelect=document.getElementById('provinceSelect');
  const shipCostEl=document.getElementById('shipCost');
  const grandEl=document.getElementById('grandTotal');
  const subtotalEl=document.getElementById('subtotalVal');
  const discountRow=document.getElementById('discountRow');
  const discountAmountEl=document.getElementById('discountAmount');
  const discountSummaryRow=document.getElementById('discountSummaryRow');
  const discountSummary=document.getElementById('discountSummary');
  const base=parseFloat(((grandEl && grandEl.textContent) || '').replace(/[^0-9.,]/g,'').replace(',','.'))||0;
  const CADETE_COST=5000;
  const COSTS=(()=>{ const m={}; document.querySelectorAll('#pcData [data-prov]').forEach(n=>{ const k=n.getAttribute('data-prov'); const v=parseFloat(n.getAttribute('data-cost')||'0'); if(k){ m[k]=v; } }); return m; })();
  const phone = form.querySelector('input[name="phone"]');
  const addrCadete = form.querySelector('input[name="address_cadete"]');
  const addrEnvio = form.querySelector('input[name="address_envio"]');
  const postal = form.querySelector('input[name="postal_code"]');
  const dni = form.querySelector('input[name="dni"]');
  function setRequired(el,flag){ if(!el) return; if(flag){el.setAttribute('required','required')} else {el.removeAttribute('required')} }
  function calcCost(){
    let method='retiro'; shipRadios.forEach(r=>{ if(r.checked) method=r.value });
    let paymentMethod='mercadopago'; paymentRadios.forEach(r=>{ if(r.checked) paymentMethod=r.value });
    if(envioGroup) envioGroup.style.display='none'; if(cadeteGroup) cadeteGroup.style.display='none';
    setRequired(phone,false); setRequired(addrCadete,false); setRequired(addrEnvio,false); setRequired(provinceSelect,false); setRequired(postal,false); setRequired(dni,false);
    let cost=0;
    if(method==='envio'){
      if(envioGroup) envioGroup.style.display='flex';
      setRequired(phone,true); setRequired(addrEnvio,true); setRequired(provinceSelect,true); setRequired(postal,true); setRequired(dni,true);
      const prov=provinceSelect?provinceSelect.value:''; if(prov && COSTS[prov]!=null){cost=COSTS[prov];}
    } else if(method==='cadete') {
      if(cadeteGroup) cadeteGroup.style.display='flex';
      setRequired(phone,true); setRequired(addrCadete,true);
      cost=CADETE_COST;
    }
    if(shipCostEl) shipCostEl.textContent='$'+cost.toFixed(2);
    const subtotal=base+cost;
    // Sin descuentos - el cliente paga el precio total
    const discount=0;
    if(discountRow) discountRow.style.display='none';
    if(discountSummaryRow) discountSummaryRow.style.display='none';
    const total=subtotal.toFixed(2);
    if(grandEl) grandEl.textContent='$'+total;
    if(subtotalEl) subtotalEl.textContent='$'+subtotal.toFixed(2);
  }
  shipRadios.forEach(r=>r.addEventListener('change',calcCost));
  paymentRadios.forEach(r=>r.addEventListener('change',calcCost));
  provinceSelect && provinceSelect.addEventListener('change',calcCost);
  calcCost();
})();

// Registrar Service Worker en idle para no bloquear carga
if ('serviceWorker' in navigator) {
  const registerSW = () => navigator.serviceWorker.register('/public/sw.js').catch(()=>{});
  if (window.requestIdleCallback) requestIdleCallback(registerSW, {timeout: 2000});
  else window.addEventListener('load', registerSW, {once:true});
}

// Producto: carrusel de imágenes (CSP-safe, sin inline)
(function(){
  const root=document.getElementById('pdCarousel'); if(!root) return;
  const slides=[...root.querySelectorAll('.pd-slide')];
  const thumbs=[...root.querySelectorAll('.pd-thumb')];
  const total=slides.length; if(total===0) return;
  const prevBtn=root.querySelector('.pd-nav.prev');
  const nextBtn=root.querySelector('.pd-nav.next');
  if(total===1){ if(prevBtn) prevBtn.style.display='none'; if(nextBtn) nextBtn.style.display='none'; const tb=root.querySelector('.pd-thumbs'); if(tb) tb.style.display='none'; return; }
  let idx=0; let auto=null; const AUTOPLAY=5000;
  function show(n){
    slides[idx].classList.remove('active'); thumbs[idx].classList.remove('active');
    idx=(n+total)%total;
    slides[idx].classList.add('active'); thumbs[idx].classList.add('active');
  }
  function next(){ show(idx+1); }
  function prev(){ show(idx-1); }
  function start(){ auto=setInterval(next,AUTOPLAY); }
  function restart(){ if(auto) clearInterval(auto); start(); }
  root.addEventListener('click', (e)=>{
    const t=e.target.closest('.pd-thumb');
    if(t){ const i=parseInt(t.getAttribute('data-index')||'-1'); if(!isNaN(i)) { show(i); restart(); } return; }
    if(e.target.closest('.pd-nav.prev')){ prev(); restart(); return; }
    if(e.target.closest('.pd-nav.next')){ next(); restart(); return; }
  });
  // Swipe táctil
  let startX=0,isSwiping=false;
  root.addEventListener('touchstart',e=>{startX=e.touches[0].clientX;isSwiping=true;},{passive:true});
  root.addEventListener('touchmove',e=>{ if(!isSwiping) return; const dx=e.touches[0].clientX-startX; if(Math.abs(dx)>10){ e.preventDefault(); } },{passive:false});
  root.addEventListener('touchend',e=>{ if(!isSwiping) return; isSwiping=false; const dx=e.changedTouches[0].clientX-startX; if(Math.abs(dx)>50){ if(dx>0) prev(); else next(); restart(); } },{passive:true});
  // Teclado
  root.addEventListener('keydown',e=>{ if(e.key==='ArrowRight'){ next(); restart(); } else if(e.key==='ArrowLeft'){ prev(); restart(); } });
  root.tabIndex=0; start();
})();

// Producto: compartir (CSP-safe)
(function(){
  const bar=document.getElementById('pdShareBar'); if(!bar) return;
  const msgEl=document.getElementById('shareMsg');
  const titleEl=document.querySelector('.pd-title');
  const productName=titleEl?((titleEl.textContent||'').trim()):'';
  const buildURL=()=>window.location.href.split('#')[0];
  function flash(t){ if(!msgEl) return; msgEl.textContent=t; msgEl.style.display='inline'; msgEl.classList.add('show'); setTimeout(()=>{ msgEl.style.display='none'; msgEl.classList.remove('show'); },2500); }
  bar.addEventListener('click', async e=>{
    const btn=e.target.closest('.share-btn'); if(!btn) return;
    e.preventDefault();
    const url=buildURL();
    const text=`Mira este producto: ${productName} - ${url}`;
    const kind=btn.getAttribute('data-share');
    if(kind==='copy'){
      let copied=false;
      try{ if(navigator.clipboard && window.isSecureContext){ await navigator.clipboard.writeText(text); copied=true; } }catch{}
      if(!copied){ try{ const ta=document.createElement('textarea'); ta.value=text; ta.style.position='fixed'; ta.style.top='-1000px'; document.body.appendChild(ta); ta.focus(); ta.select(); document.execCommand('copy'); ta.remove(); copied=true; }catch{} }
      flash(copied?'Enlace copiado':'No se pudo copiar');
    } else if(kind==='whatsapp'){
      const enc=encodeURIComponent(text);
      const w1=`https://wa.me/?text=${enc}`;
      const w2=`https://api.whatsapp.com/send?text=${enc}`;
      const win=window.open(w1,'_blank','noopener');
      if(!win || win.closed){ setTimeout(()=>{ window.open(w2,'_blank','noopener') || (location.href=w1); },50); }
    } else if(kind==='instagram'){
      const shareData={title:productName,text:`Mira este producto: ${productName}`,url};
      if(navigator.share){ try{ await navigator.share(shareData); }catch{} }
      else {
        let copied=false; try{ if(navigator.clipboard && window.isSecureContext){ await navigator.clipboard.writeText(text); copied=true; } }catch{}
        if(!copied){ try{ const ta=document.createElement('textarea'); ta.value=text; ta.style.position='fixed'; ta.style.top='-1000px'; document.body.appendChild(ta); ta.focus(); ta.select(); document.execCommand('copy'); ta.remove(); copied=true; }catch{} }
        window.open('https://www.instagram.com/direct/new/','_blank','noopener');
        flash(copied?'Texto copiado. Pega en Instagram':'No se pudo copiar');
      }
    } else if(kind==='x'){
      const u=encodeURIComponent(url);
      const t=encodeURIComponent(`Mira este producto: ${productName}`);
      const xUrl=`https://twitter.com/intent/tweet?url=${u}&text=${t}`;
      window.open(xUrl,'_blank','noopener');
    } else if(kind==='threads'){
      const u=encodeURIComponent(url);
      const t=encodeURIComponent(`Mira este producto: ${productName}`);
      const thUrl=`https://www.threads.net/intent/post?text=${t}%20${u}`;
      window.open(thUrl,'_blank','noopener');
    }
  });
})();

// Producto: selector de color (CSP-safe, sin inline)
(function(){
  const form=document.querySelector('.pd-form'); if(!form) return;
  const inp=document.getElementById('colorInput'); if(!inp) return;
  const swc=document.getElementById('cpSwatches');
  const preview=document.getElementById('colorPreview');
  const customRow=document.getElementById('customColorRow');
  const customInput=document.getElementById('customColorInput');
  const otherBtn=form.querySelector('.swatch-other');
  const defaultColor=inp.value||'#111827';

  function setPreview(c){ if(preview){ preview.style.background = (c&&c.trim()) || defaultColor; } }
  function setActive(btn){ if(!swc) return; swc.querySelectorAll('.swatch').forEach(x=>{ x.classList.remove('active'); x.setAttribute('aria-checked','false'); }); if(btn){ btn.classList.add('active'); btn.setAttribute('aria-checked','true'); } }

  if(swc){
    swc.addEventListener('click',e=>{
      const b=e.target.closest('.swatch'); if(!b) return;
      e.preventDefault();
      setActive(b);
      if(b.classList.contains('swatch-other')){
        if(customRow) customRow.style.display='';
        if(customInput) customInput.focus();
        inp.value = (customInput && customInput.value.trim()) || '';
      } else {
        if(customRow) customRow.style.display='none';
        inp.value = b.getAttribute('data-color') || '';
      }
      setPreview(inp.value);
    });
  }

  if(customInput){
    customInput.addEventListener('input',()=>{
      const val=customInput.value.trim();
      inp.value=val;
      if(otherBtn && val){ otherBtn.style.setProperty('--c', val); }
      setPreview(val);
    });
  }

  form.addEventListener('submit',()=>{ if(!inp.value.trim()){ inp.value=defaultColor; } });
  setPreview(defaultColor);
})();


// Admin: productos (form + tabla + dropzone) sin inline JS
(function(){
  const form=document.getElementById('prodForm'); if(!form) return;
  const tbl=document.getElementById('prodTable');
  const fSlug=document.getElementById('pfSlug');
  const fName=document.getElementById('pfName');
  const fCat=document.getElementById('pfCategory');
  const fDesc=document.getElementById('pfShort');
  const fPrice=document.getElementById('pfPrice');
  const fReady=document.getElementById('pfReady');
  const fWidth=document.getElementById('pfWidth');
  const fHeight=document.getElementById('pfHeight');
  const fDepth=document.getElementById('pfDepth');
  const btnDel=document.getElementById('pfDelete');
  const btnReset=document.getElementById('pfReset');
  const btnSubmit=document.getElementById('pfSubmit');
  const modeBadge=document.getElementById('formMode');
  const imagesInput=document.getElementById('pfImages');
  const dropZone=document.getElementById('dropZone');
  const preview=document.getElementById('preview');
  const dzCount=document.querySelector('.dz-count');

  function setModeEdit(edit){
    if(!modeBadge||!btnSubmit) return;
    if(edit){ modeBadge.textContent='Edición'; modeBadge.classList.remove('create'); modeBadge.classList.add('edit'); btnSubmit.textContent='Actualizar'; }
    else { modeBadge.textContent='Creación'; modeBadge.classList.remove('edit'); modeBadge.classList.add('create'); btnSubmit.textContent='Crear'; }
  }
  function fill(p){
    if(!p) return;
    if(fSlug) fSlug.value=p.Slug||''; if(fName) fName.value=p.Name||''; if(fCat) fCat.value=p.Category||''; if(fDesc) fDesc.value=p.ShortDesc||''; if(fPrice) fPrice.value=p.BasePrice!=null?p.BasePrice:''; if(fReady) fReady.checked=!!p.ReadyToShip; if(fWidth) fWidth.value=p.WidthMM||0; if(fHeight) fHeight.value=p.HeightMM||0; if(fDepth) fDepth.value=p.DepthMM||0; if(btnDel) btnDel.style.display=''; setModeEdit(true);
  }
  function clear(){ if(form) form.reset(); if(fSlug) fSlug.value=''; if(btnDel) btnDel.style.display='none'; if(fWidth) fWidth.value=''; if(fHeight) fHeight.value=''; if(fDepth) fDepth.value=''; if(imagesInput) imagesInput.value=''; if(preview) preview.innerHTML=''; if(dzCount) dzCount.textContent='0 archivos'; setModeEdit(false); }

  if(tbl){ tbl.addEventListener('click', async e=>{
    const btn=e.target.closest('button'); if(!btn) return; const tr=btn.closest('tr'); if(!tr) return; const slug=tr.getAttribute('data-slug')||'';
    if(btn.getAttribute('data-act')==='edit'){
      const res=await fetch('/api/products/'+encodeURIComponent(slug)); if(res.ok){ const p=await res.json(); fill(p);} return;
    }
    if(btn.getAttribute('data-act')==='del'){
      if(!confirm('Eliminar producto y todos sus datos?')) return; const res=await fetch('/api/products/'+encodeURIComponent(slug),{method:'DELETE'}); if(res.ok){ location.reload(); } else { alert('Error eliminando'); } return;
    }
  }); }

  form.addEventListener('submit', async e=>{
    e.preventDefault();
    const slug=(fSlug&&fSlug.value.trim())||'';
    const payload={ name:(fName&&fName.value.trim())||'', category:(fCat&&fCat.value.trim())||'', short_desc:(fDesc&&fDesc.value)||'', base_price:parseFloat((fPrice&&fPrice.value)||'0'), ready_to_ship:!!(fReady&&fReady.checked), width_mm:parseFloat((fWidth&&fWidth.value)||'0'), height_mm:parseFloat((fHeight&&fHeight.value)||'0'), depth_mm:parseFloat((fDepth&&fDepth.value)||'0') };
    if(!payload.name){ alert('Nombre requerido'); return; }
    if(payload.base_price<0){ alert('Precio inválido'); return; }
    let method='POST', url='/api/products'; if(slug){ method='PUT'; url='/api/products/'+encodeURIComponent(slug); }
    const res=await fetch(url,{method, headers:{'Content-Type':'application/json'}, body:JSON.stringify(payload)});
    if(!res.ok){ alert('Error guardando'); return; }
    const prod=await res.json(); const finalSlug=(prod&&prod.Slug)||slug;
    if(imagesInput && imagesInput.files && imagesInput.files.length>0){
      const fd=new FormData(); fd.append('existing_slug', finalSlug);
      for(const f of imagesInput.files){ fd.append('images', f); }
      const upRes=await fetch('/api/products/upload',{method:'POST', body:fd});
      if(!upRes.ok){ alert('Producto guardado, pero error subiendo imágenes'); location.reload(); return; }
    }
    location.reload();
  });

  if(btnReset) btnReset.addEventListener('click', clear);
  if(btnDel) btnDel.addEventListener('click', async ()=>{ const slug=(fSlug&&fSlug.value.trim())||''; if(!slug) return; if(!confirm('Eliminar producto y sus imágenes?')) return; const res=await fetch('/api/products/'+encodeURIComponent(slug),{method:'DELETE'}); if(res.ok){ clear(); location.reload(); } else { alert('Error'); } });

  function refreshPreview(){ if(!preview||!imagesInput||!dzCount) return; preview.innerHTML=''; const files=Array.from(imagesInput.files||[]); dzCount.textContent=files.length+(files.length===1?' archivo':' archivos'); files.slice(0,6).forEach(f=>{ const r=new FileReader(); r.onload=ev=>{ const img=document.createElement('img'); img.src=ev.target.result; img.alt=f.name; img.style.width='52px'; img.style.height='52px'; img.style.objectFit='cover'; img.style.borderRadius='10px'; img.style.border='1px solid #223140'; preview.appendChild(img); }; r.readAsDataURL(f); }); }
  if(imagesInput) imagesInput.addEventListener('change', refreshPreview);
  if(dropZone){ ['dragenter','dragover'].forEach(ev=>dropZone.addEventListener(ev,e=>{e.preventDefault(); dropZone.classList.add('drag');})); ['dragleave','drop'].forEach(ev=>dropZone.addEventListener(ev,e=>{e.preventDefault(); dropZone.classList.remove('drag');})); dropZone.addEventListener('drop', e=>{ const files=[...e.dataTransfer.files].filter(f=>f.type.startsWith('image/')); if(files.length){ const dt=new DataTransfer(); files.forEach(f=>dt.items.add(f)); if(imagesInput) imagesInput.files=dt.files; refreshPreview(); }}); }
})();
