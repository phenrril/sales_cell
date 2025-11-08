const CACHE_NAME = 'chroma3d-v1';
const urlsToCache = [
  '/',
  '/public/assets/styles.css',
  '/public/assets/img/chroma-logo.png',
  '/public/assets/img/img1.webp',
  '/public/assets/img/img2.webp',
  '/public/assets/img/img3.webp',
  '/public/assets/img/img4.webp'
];

self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then(cache => cache.addAll(urlsToCache))
  );
});

self.addEventListener('fetch', event => {
  event.respondWith(
    caches.match(event.request)
      .then(response => response || fetch(event.request))
  );
});