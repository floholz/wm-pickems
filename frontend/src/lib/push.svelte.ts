// Reactive Web Push state for the current device. Mirrors pwa.svelte.ts: a
// single singleton (`push`) exposes whether push is supported, the current
// permission, whether this device is subscribed, and enable()/disable()
// actions that manage the PushManager subscription and sync it to the backend.
import { pb } from './pb';

// VAPID public keys are base64url; PushManager wants a Uint8Array.
function urlBase64ToUint8Array(base64: string): Uint8Array<ArrayBuffer> {
	const padding = '='.repeat((4 - (base64.length % 4)) % 4);
	const b64 = (base64 + padding).replace(/-/g, '+').replace(/_/g, '/');
	const raw = atob(b64);
	const buf = new ArrayBuffer(raw.length);
	const out = new Uint8Array(buf);
	for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
	return out;
}

class Push {
	supported = $state(false);
	permission = $state<NotificationPermission>('default');
	subscribed = $state(false);
	ready = $state(false); // initial subscription check has completed
	busy = $state(false);
	error = $state('');

	constructor() {
		if (typeof window === 'undefined') return;
		this.supported =
			'serviceWorker' in navigator &&
			'PushManager' in window &&
			'Notification' in window;
		if (!this.supported) {
			this.ready = true;
			return;
		}
		this.permission = Notification.permission;
		void this.refresh();
	}

	// Sync `subscribed` with the actual PushManager state.
	private async refresh() {
		try {
			const reg = await navigator.serviceWorker.ready;
			const sub = await reg.pushManager.getSubscription();
			this.subscribed = !!sub;
		} catch {
			this.subscribed = false;
		} finally {
			this.ready = true;
		}
	}

	// True when the browser blocked notifications (user must re-allow in
	// browser settings — we can't re-prompt).
	get blocked() {
		return this.permission === 'denied';
	}

	async enable() {
		if (!this.supported || this.busy) return;
		this.error = '';
		this.busy = true;
		try {
			this.permission = await Notification.requestPermission();
			if (this.permission !== 'granted') {
				this.error =
					this.permission === 'denied'
						? 'Notifications are blocked in your browser settings.'
						: 'Permission was not granted.';
				return;
			}
			const { publicKey } = await pb.send<{ publicKey: string }>(
				'/api/push/key',
				{ method: 'GET' }
			);
			if (!publicKey) {
				this.error = 'Push is not configured on the server.';
				return;
			}
			const reg = await navigator.serviceWorker.ready;
			const sub = await reg.pushManager.subscribe({
				userVisibleOnly: true,
				applicationServerKey: urlBase64ToUint8Array(publicKey)
			});
			const json = sub.toJSON() as {
				endpoint: string;
				keys: { p256dh: string; auth: string };
			};
			await pb.send('/api/push/subscribe', { method: 'POST', body: json });
			this.subscribed = true;
		} catch (err: unknown) {
			this.error =
				(err as { message?: string })?.message ??
				'Could not enable push notifications.';
		} finally {
			this.busy = false;
		}
	}

	// Ask the server to push a test notification to this account's devices.
	// Returns how many endpoints accepted it (for surfacing in the UI).
	async test(): Promise<{ sent: number; total: number }> {
		this.error = '';
		const res = await pb.send<{ sent: number; total: number }>(
			'/api/push/test',
			{ method: 'POST' }
		);
		return res;
	}

	async disable() {
		if (!this.supported || this.busy) return;
		this.error = '';
		this.busy = true;
		try {
			const reg = await navigator.serviceWorker.ready;
			const sub = await reg.pushManager.getSubscription();
			if (sub) {
				const endpoint = sub.endpoint;
				await sub.unsubscribe();
				await pb
					.send('/api/push/unsubscribe', { method: 'POST', body: { endpoint } })
					.catch(() => {});
			}
			this.subscribed = false;
		} catch (err: unknown) {
			this.error =
				(err as { message?: string })?.message ??
				'Could not disable push notifications.';
		} finally {
			this.busy = false;
		}
	}
}

export const push = new Push();
