const CACHE_NAME = 'newmobile-v2';
const urlsToCache = [
  '/',
  '/public/assets/styles.css',
  '/public/assets/img/newmobile.png',
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
  self.skipWaiting();
});

self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys().then(cacheNames =>
      Promise.all(
        cacheNames
          .filter(name => name !== CACHE_NAME)
          .map(name => caches.delete(name))
      )
    ).then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', event => {
  event.respondWith(
    caches.match(event.request)
      .then(response => response || fetch(event.request))
  );
});