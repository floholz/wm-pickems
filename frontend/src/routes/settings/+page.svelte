<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { goto } from '$app/navigation';
	import Avatar from '$lib/components/Avatar.svelte';

	const MAX_AVATAR_BYTES = 5 * 1024 * 1024; // PocketBase users-avatar default

	let name = $state(auth.user?.name ?? '');
	let avatarFile = $state<File | null>(null);
	let previewUrl = $state<string | null>(null);
	let error = $state('');
	let saved = $state(false);
	let busy = $state(false);
	let fileInput: HTMLInputElement;

	let resetBusy = $state(false);
	let resetSent = $state(false);
	let resetError = $state('');

	async function sendReset() {
		if (!auth.user?.email) return;
		resetError = '';
		resetSent = false;
		resetBusy = true;
		try {
			await auth.requestPasswordReset(auth.user.email);
			resetSent = true;
		} catch (err: unknown) {
			resetError =
				(err as { message?: string })?.message ??
				'Could not send reset email.';
		} finally {
			resetBusy = false;
		}
	}

	// Revoke the object URL when it's replaced or the page unmounts.
	$effect(() => {
		const url = previewUrl;
		return () => {
			if (url) URL.revokeObjectURL(url);
		};
	});

	function pickFile(e: Event) {
		const file = (e.target as HTMLInputElement).files?.[0];
		if (!file) return;
		if (!file.type.startsWith('image/')) {
			error = 'Please choose an image file.';
			return;
		}
		if (file.size > MAX_AVATAR_BYTES) {
			error = 'Image must be 5 MB or smaller.';
			return;
		}
		error = '';
		saved = false;
		avatarFile = file;
		previewUrl = URL.createObjectURL(file);
	}

	async function submit(e: Event) {
		e.preventDefault();
		error = '';
		saved = false;
		const trimmed = name.trim();
		if (trimmed.length < 1 || trimmed.length > 48) {
			error = 'Display name must be between 1 and 48 characters.';
			return;
		}
		busy = true;
		try {
			await auth.updateProfile({ name: trimmed, avatarFile });
			avatarFile = null;
			previewUrl = null;
			if (fileInput) fileInput.value = '';
			saved = true;
		} catch (err: unknown) {
			error =
				(err as { message?: string })?.message ??
				'Could not save changes.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="settings">
	<h1>Settings</h1>
	<p class="muted">Manage how you appear to friends.</p>

	<form class="card" onsubmit={submit}>
		<div class="avatar-row">
			<Avatar
				name={name || auth.user?.name || '?'}
				src={previewUrl ?? auth.user?.avatarUrl}
				size={96}
			/>
			<div>
				<button
					type="button"
					class="btn secondary"
					onclick={() => fileInput.click()}
					disabled={busy}
				>
					Change photo
				</button>
				<p class="muted hint">PNG or JPG, up to 5 MB.</p>
			</div>
			<input
				bind:this={fileInput}
				type="file"
				accept="image/*"
				class="hidden-file"
				onchange={pickFile}
			/>
		</div>

		<div class="field">
			<label for="dn">Display name</label>
			<input
				id="dn"
				class="input"
				bind:value={name}
				maxlength="48"
				autocomplete="name"
				required
			/>
		</div>

		{#if error}<p class="error">{error}</p>{/if}
		{#if saved}<p class="ok">Saved.</p>{/if}

		<button class="btn" disabled={busy}>{busy ? 'Saving…' : 'Save changes'}</button>
	</form>

	<section class="card">
		<h3>Password</h3>
		<p class="muted small">
			We'll email a reset link to <strong>{auth.user?.email ?? ''}</strong>.
			Click it to choose a new password.
		</p>
		{#if resetError}<p class="error">{resetError}</p>{/if}
		{#if resetSent}
			<p class="ok">Reset email sent — check your inbox.</p>
		{/if}
		<button
			type="button"
			class="btn secondary"
			onclick={sendReset}
			disabled={resetBusy || resetSent}
		>
			{resetBusy ? 'Sending…' : resetSent ? 'Sent' : 'Send reset link'}
		</button>
	</section>

	<p class="muted switch"><a href="/">Back</a></p>
</div>

<style>
	.settings {
		max-width: 380px;
		margin: 8dvh auto 0;
	}
	h1 {
		margin: 0;
		font-size: 1.8rem;
	}
	.muted {
		margin: 0.25rem 0 1.5rem;
	}
	.avatar-row {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin-bottom: 1.25rem;
	}
	.hint {
		margin: 0.5rem 0 0;
		font-size: 0.8rem;
	}
	.hidden-file {
		display: none;
	}
	.ok {
		color: var(--success);
		font-size: 0.9rem;
	}
	.small {
		font-size: 0.85rem;
		margin: 0.25rem 0 0.9rem;
	}
	h3 {
		margin: 0 0 0.5rem;
		font-size: 1rem;
	}
	.switch {
		text-align: center;
		margin: 1rem 0 0;
	}
</style>
