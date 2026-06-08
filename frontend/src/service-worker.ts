/// <reference types="@sveltejs/kit" />
/// <reference lib="webworker" />

// Minimal offline-capable service worker (makes the app installable and the
// shell available offline). API/auth calls always go to the network.
import { build, files, version } from '$service-worker';

const sw = self as unknown as ServiceWorkerGlobalScope;
const CACHE = `wmtips-${version}`;

// Precache the app shell + light static assets. Skip the heavy stuff
// (flags / screenshots) — those are cached on demand instead.
const PRECACHE = [
	...build,
	...files.filter(
		(f) => !f.startsWith('/flags/') && !f.startsWith('/screenshots/')
	)
];

sw.addEventListener('install', (e) => {
	e.waitUntil(
		caches
			.open(CACHE)
			.then(async (c) => {
				await c.addAll(PRECACHE);
				// App-shell entry (adapter-static fallback) — best effort.
				await c.add('/').catch(() => {});
			})
			.then(() => sw.skipWaiting())
	);
});

sw.addEventListener('activate', (e) => {
	e.waitUntil(
		caches
			.keys()
			.then((keys) =>
				Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)))
			)
			.then(() => sw.clients.claim())
	);
});

sw.addEventListener('fetch', (e) => {
	const req = e.request;
	const url = new URL(req.url);

	// Only handle same-origin GETs; never the API / PocketBase routes.
	if (
		req.method !== 'GET' ||
		url.origin !== location.origin ||
		url.pathname.startsWith('/api/') ||
		url.pathname.startsWith('/_/')
	) {
		return;
	}

	// SPA navigations: serve the cached app shell when offline.
	if (req.mode === 'navigate') {
		e.respondWith(
			fetch(req).catch(
				async () =>
					(await caches.match('/')) ??
					(await caches.match('/index.html')) ??
					Response.error()
			)
		);
		return;
	}

	// Static assets: cache-first, fall back to network and cache the result.
	e.respondWith(
		caches.match(req).then(
			(hit) =>
				hit ??
				fetch(req).then((res) => {
					if (res.ok && res.type === 'basic') {
						const copy = res.clone();
						caches.open(CACHE).then((c) => c.put(req, copy));
					}
					return res;
				})
		)
	);
});

// ---- Web Push ----

// Show a notification when the server pushes one. Payload is the JSON sent by
// internal/push: { title, body, url, tag }.
sw.addEventListener('push', (e) => {
	let data: {
		title?: string;
		body?: string;
		url?: string;
		tag?: string;
		icon?: string;
		requireInteraction?: boolean;
	} = {};
	try {
		data = e.data?.json() ?? {};
	} catch {
		data = { body: e.data?.text() };
	}
	const title = data.title || 'WM Pickems';
	// High-priority messages (requireInteraction) stay on screen until acted on
	// and buzz; `vibrate` isn't in the TS NotificationOptions type yet.
	const opts: NotificationOptions & { vibrate?: number[] } = {
		body: data.body ?? '',
		// Per-event contextual icon (server-provided); monochrome badge for the
		// status bar.
		icon: data.icon || '/icons/notif/default.png',
		badge: '/icons/badge.png',
		tag: data.tag,
		data: { url: data.url || '/' },
		requireInteraction: data.requireInteraction ?? false
	};
	if (data.requireInteraction) opts.vibrate = [200, 100, 200];
	e.waitUntil(sw.registration.showNotification(title, opts));
});

// Focus an existing tab (or open one) at the notification's URL on click.
sw.addEventListener('notificationclick', (e) => {
	e.notification.close();
	const url = (e.notification.data as { url?: string })?.url || '/';
	e.waitUntil(
		(async () => {
			const all = await sw.clients.matchAll({
				type: 'window',
				includeUncontrolled: true
			});
			for (const c of all) {
				if ('focus' in c) {
					await c.focus();
					if ('navigate' in c && new URL(c.url).pathname !== url) {
						try {
							await (c as WindowClient).navigate(url);
						} catch {
							/* cross-origin or not allowed — ignore */
						}
					}
					return;
				}
			}
			await sw.clients.openWindow(url);
		})()
	);
});
