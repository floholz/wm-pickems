import { pb } from './pb';

// Reactive auth state backed by PocketBase's authStore. Svelte 5 runes class;
// a single shared instance is exported below.
class Auth {
	user = $state<{
		id: string;
		name: string;
		email: string;
		avatarUrl: string | null;
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
			avatarUrl
		};
	}

	get isAuthed() {
		return this.user !== null;
	}

	async login(identity: string, password: string) {
		await pb.collection('users').authWithPassword(identity, password);
	}

	// Google OAuth2 (popup flow). Creates or signs into the matching account;
	// the avatar/name are pulled from the Google profile by the server.
	async loginGoogle() {
		await pb.collection('users').authWithOAuth2({ provider: 'google' });
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
