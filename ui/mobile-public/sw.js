// Gwen's phone app keeps its own files so it opens when the laptop is asleep and can say so
// (plan.md 3.14.11). It never caches anything under /v1/: what the laptop says is always asked fresh.
const CACHE = "gwen-shell-v1";

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(CACHE).then((c) => c.addAll(["/", "/manifest.webmanifest", "/icon-192.png"])).then(() => self.skipWaiting()));
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)))).then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);
  if (event.request.method !== "GET" || url.origin !== location.origin || url.pathname.startsWith("/v1/")) return;
  // Pages: the network first, so an update shows at once; the kept copy when the laptop can't be reached.
  if (event.request.mode === "navigate") {
    event.respondWith(
      fetch(event.request)
        .then((res) => {
          if (res.ok) {
            const copy = res.clone();
            caches.open(CACHE).then((c) => c.put("/", copy));
          }
          return res;
        })
        .catch(() => caches.match("/").then((res) => res ?? Response.error())),
    );
    return;
  }
  // Built files have their hash in the name: keep them once fetched.
  event.respondWith(
    caches.match(event.request).then(
      (hit) =>
        hit ??
        fetch(event.request).then((res) => {
          if (res.ok && (url.pathname.startsWith("/assets/") || url.pathname.startsWith("/icon-"))) {
            const copy = res.clone();
            caches.open(CACHE).then((c) => c.put(event.request, copy));
          }
          return res;
        }),
    ),
  );
});

// Notifications (plan.md 3.14.11): while you are away from the laptop it pushes when a session needs you, finishes, or
// stops, or Gwen waits for your OK. A tap opens that place in the app, in the window already open when there is one.
self.addEventListener("push", (event) => {
  let m = {};
  try {
    m = event.data ? event.data.json() : {};
  } catch {
    m = { body: event.data ? event.data.text() : "" };
  }
  event.waitUntil(
    self.registration.showNotification(m.title || "Gwen", {
      body: m.body || "Something needs you.",
      tag: m.tag || undefined,
      renotify: !!m.tag,
      icon: "/icon-192.png",
      badge: "/icon-192.png",
      data: { url: m.url || "/" },
    }),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = new URL((event.notification.data && event.notification.data.url) || "/", self.location.origin).href;
  event.waitUntil(
    (async () => {
      const open = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
      const win = open.find((c) => new URL(c.url).origin === self.location.origin);
      if (win) {
        await win.focus();
        if (win.url !== url) await win.navigate(url).catch(() => self.clients.openWindow(url));
        return;
      }
      await self.clients.openWindow(url);
    })(),
  );
});
