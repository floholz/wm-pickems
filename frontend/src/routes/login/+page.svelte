<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';

	let identity = $state('');
	let password = $state('');
	let error = $state('');
	let busy = $state(false);

	// After signing in, resume an invite if one was carried in the URL.
	let invite = $derived($page.url.searchParams.get('invite'));
	function dest() {
		return invite ? `/join/${invite}` : '/';
	}
	let registerHref = $derived(
		invite ? `/register?invite=${encodeURIComponent(invite)}` : '/register'
	);

	async function submit(e: Event) {
		e.preventDefault();
		error = '';
		busy = true;
		try {
			await auth.login(identity, password);
			goto(dest());
		} catch {
			error = 'Invalid email or password.';
		} finally {
			busy = false;
		}
	}

	async function google() {
		error = '';
		busy = true;
		try {
			await auth.loginGoogle();
			goto(dest());
		} catch (e: unknown) {
			error =
				(e as { message?: string })?.message ?? 'Google sign-in failed.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="auth">
	<h1>WM Tips</h1>
	<p class="muted">Predict the World Cup. Beat your friends.</p>

	<form class="card" onsubmit={submit}>
		<div class="field">
			<label for="id">Email</label>
			<input
				id="id"
				class="input"
				type="email"
				bind:value={identity}
				autocomplete="email"
				required
			/>
		</div>
		<div class="field">
			<label for="pw">Password</label>
			<input
				id="pw"
				class="input"
				type="password"
				bind:value={password}
				autocomplete="current-password"
				required
			/>
		</div>
		{#if error}<p class="error">{error}</p>{/if}
		<button class="btn" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
		<div class="sep"><span>or</span></div>
		<button
			type="button"
			class="gsi"
			disabled={busy}
			onclick={google}
			aria-label="Continue with Google"
		>
			<svg class="gsi-logo" viewBox="0 0 48 48" aria-hidden="true">
				<path
					fill="#EA4335"
					d="M24 9.5c3.54 0 6.71 1.22 9.21 3.6l6.85-6.85C35.9 2.38 30.47 0 24 0 14.62 0 6.51 5.38 2.56 13.22l7.98 6.19C12.43 13.72 17.74 9.5 24 9.5z"
				/>
				<path
					fill="#4285F4"
					d="M46.98 24.55c0-1.57-.15-3.09-.38-4.55H24v9.02h12.94c-.58 2.96-2.26 5.48-4.78 7.18l7.73 6c4.51-4.18 7.09-10.36 7.09-17.65z"
				/>
				<path
					fill="#FBBC05"
					d="M10.53 28.59c-.48-1.45-.76-2.99-.76-4.59s.27-3.14.76-4.59l-7.98-6.19C.92 16.46 0 20.12 0 24c0 3.88.92 7.54 2.56 10.78l7.97-6.19z"
				/>
				<path
					fill="#34A853"
					d="M24 48c6.48 0 11.93-2.13 15.89-5.81l-7.73-6c-2.15 1.45-4.92 2.3-8.16 2.3-6.26 0-11.57-4.22-13.47-9.91l-7.98 6.19C6.51 42.62 14.62 48 24 48z"
				/>
			</svg>
			<span class="gsi-text">Continue with Google</span>
		</button>
		<p class="muted switch">
			No account? <a href={registerHref}>Create one</a>
		</p>
	</form>
</div>

<style>
	@import url('https://fonts.googleapis.com/css2?family=Roboto:wght@500&display=swap');

	.auth {
		max-width: 380px;
		margin: 12dvh auto 0;
	}
	h1 {
		margin: 0;
		font-size: 2rem;
	}
	.muted {
		margin: 0.25rem 0 1.5rem;
	}
	.switch {
		text-align: center;
		margin: 1rem 0 0;
	}
	.sep {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin: 0.9rem 0;
		color: var(--muted);
		font-size: 0.8rem;
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}
	.sep::before,
	.sep::after {
		content: '';
		flex: 1;
		height: 1px;
		background: var(--border);
	}

	/* Google "Sign in with Google" button — light theme, per Google's
	   Identity branding guidelines. Colors, logo, font and capitalization
	   must not be altered. https://developers.google.com/identity/branding-guidelines
	   (Roboto is imported at the top of this stylesheet.) */
	.gsi {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 12px;
		width: 100%;
		height: 40px;
		padding: 0 12px;
		background: #ffffff;
		border: 1px solid #747775;
		border-radius: 4px;
		color: #1f1f1f;
		font-family: 'Roboto', arial, sans-serif;
		font-size: 14px;
		font-weight: 500;
		letter-spacing: 0.25px;
		text-transform: none;
		cursor: pointer;
		transition: background-color 0.15s ease, border-color 0.15s ease;
	}
	.gsi:hover:not(:disabled) {
		/* Google light-theme hover state layer: #303030 @ ~8% over white */
		background: #f7f8f8;
		border-color: #747775;
		box-shadow: 0 1px 2px rgba(60, 64, 67, 0.3);
	}
	.gsi:focus-visible {
		outline: 2px solid #8ab4f8;
		outline-offset: 2px;
	}
	.gsi:disabled {
		opacity: 0.38;
		cursor: default;
	}
	.gsi-logo {
		width: 18px;
		height: 18px;
		flex: none;
	}
	.gsi-text {
		line-height: 1;
	}
</style>
