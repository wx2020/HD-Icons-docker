// static/js/sw.js
const CACHE_NAME = 'icon-library-v1';
const ASSETS_TO_CACHE = [
  '/',
  '/static/assets/favicon.ico',
  '/static/fonts/font.ttf',
  '/static/css/xdtx.css', // 自定义 CSS 文件
  '/static/js/xdtx.js',      // 自定义 JS 文件
];

// 安装 Service Worker
self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then((cache) => cache.addAll(ASSETS_TO_CACHE))
      .then(() => self.skipWaiting())
  );
});

// 激活 Service Worker
self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((cacheNames) => {
      return Promise.all(
        cacheNames.map((cacheName) => {
          if (cacheName !== CACHE_NAME) {
            return caches.delete(cacheName);
          }
        })
      );
    }).then(() => self.clients.claim())
  );
});

// 拦截网络请求
self.addEventListener('fetch', (event) => {
  event.respondWith(
    caches.match(event.request)
      .then((response) => {
        // 如果缓存中有资源，直接返回
        if (response) {
          return response;
        }

        // 否则，从网络获取资源并缓存
        return fetch(event.request).then((response) => {
          // 只缓存图片资源
        //   if (event.request.url.endsWith('.png') || event.request.url.endsWith('.jpg') || event.request.url.endsWith('.svg')) {
          if (/\.(png|jpg|svg|jpeg|ico|gif|webp|apng|tif|tiff|bmp)$/i.test(event.request.url)) {
            const responseToCache = response.clone();
            caches.open(CACHE_NAME).then((cache) => {
              cache.put(event.request, responseToCache);
            });
          }
          return response;
        });
      })
  );
});