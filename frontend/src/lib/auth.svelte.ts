import { pb } from './pb';

// Reactive auth state backed by PocketBase's authStore. Svelte 5 runes class;
// a single shared instance is exported below.
class Auth {
	user = $state<{
		id: string;
		name: string;
		email: string;
		avatarUrl: string | null;
		role: string; // "owner" | "admin" | "bot"; empty => normal member
		// Per-event email toggles; absent/missing entries default to ON.
		notifyPrefs: Record<string, { email?: boolean }>;
	} | null>(null);

	constructor() {
		this.sync();
		pb.authStore.onChange(() => this.sync());
	}

	private sync() {
		const r = pb.authStore.record;
		if (!pb.authStore.isValid || !r) {
			this.user = null;
			return;
		}
		// Avatar comes from the PocketBase file field; Google OAuth (added
		// later) maps its avatar URL into this same field, so the UI needs
		// no change when that lands.
		const avatarUrl = r.avatar
			? pb.files.getURL(r, r.avatar as string)
			: null;
		this.user = {
			id: r.id,
			name: (r.name as string) || r.email,
			email: r.email,
			avatarUrl,
			role: (r.role as string) || 'member',
			notifyPrefs:
				(r.notifyPrefs as Record<string, { email?: boolean }>) || {}
		};
	}

	get isAuthed() {
		return this.user !== null;
	}

	// App-level admin, the trust boundary for admin-only UI. Distinct from a
	// PocketBase superuser. The server enforces the marker; this is only for
	// showing/hiding admin affordances. Owner inherits admin.
	get isAdmin() {
		return this.user?.role === 'admin' || this.user?.role === 'owner';
	}

	// App owner (role=owner) — unlocks the owner stats page.
	get isOwner() {
		return this.user?.role === 'owner';
	}

	async login(identity: string, password: string) {
		await pb.collection('users').authWithPassword(identity, password);
	}

	// Google OAuth2 (popup flow). Creates or signs into the matching account;
	// the avatar/name are pulled from the Google profile by the server.
	async loginGoogle() {
		await pb.collection('users').authWithOAuth2({ provider: 'google' });
	}

	// Update the signed-in user's display name and (optionally) avatar.
	// FormData carries the text field and the optional image in one request;
	// authRefresh re-pulls the auth record so onChange → sync() propagates the
	// change to the UserMenu and anywhere else reading auth.user.
	async updateProfile(opts: { name: string; avatarFile?: File | null }) {
		if (!this.user) throw new Error('Not signed in.');
		const body = new FormData();
		body.set('name', opts.name.trim());
		if (opts.avatarFile) body.set('avatar', opts.avatarFile);
		await pb.collection('users').update(this.user.id, body);
		await pb.collection('users').authRefresh();
	}

	// Save the per-event email notification toggles onto the user record.
	async updateNotifyPrefs(prefs: Record<string, { email?: boolean }>) {
		if (!this.user) throw new Error('Not signed in.');
		await pb.collection('users').update(this.user.id, { notifyPrefs: prefs });
		await pb.collection('users').authRefresh();
	}

	// Send a password reset email to the given address. PocketBase always
	// returns true and never reveals whether the address exists, so we treat
	// every non-thrown response as success.
	async requestPasswordReset(email: string) {
		await pb.collection('users').requestPasswordReset(email);
	}

	// Apply a reset token (from the emailed link) and set a new password.
	// PocketBase invalidates the auth store on success, so callers should
	// route the user to /login afterwards.
	async confirmPasswordReset(
		token: string,
		password: string,
		passwordConfirm: string
	) {
		await pb
			.collection('users')
			.confirmPasswordReset(token, password, passwordConfirm);
	}

	async register(name: string, email: string, password: string) {
		await pb.collection('users').create({
			name,
			email,
			password,
			passwordConfirm: password
		});
		await this.login(email, password);
	}

	logout() {
		pb.authStore.clear();
	}
}

export const auth = new Auth();
