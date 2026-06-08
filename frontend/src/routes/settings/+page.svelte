<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { push } from '$lib/push.svelte';
	import { goto } from '$app/navigation';
	import Avatar from '$lib/components/Avatar.svelte';
	import { NOTIFY_EVENTS } from '$lib/notify';
	import { api, type NotifyPolicy } from '$lib/api';

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

	// Notification preferences. Each event defaults to ON when no pref is stored.
	type Channel = 'email' | 'push';
	let prefs = $state<Record<string, { email?: boolean; push?: boolean }>>({
		...(auth.user?.notifyPrefs ?? {})
	});
	let notifyBusy = $state(false);
	let notifyError = $state('');

	// Global delivery policy (admin-controlled). When a channel is paused
	// platform-wide its toggles are shown forced-off and disabled, so users see
	// it's out of their hands rather than silently not arriving.
	let policy = $state<NotifyPolicy | null>(null);
	$effect(() => {
		api.notifyPolicy()
			.then((p) => (policy = p))
			.catch(() => (policy = null));
	});
	// Force-disabled if the master channel switch is off, or this event's channel
	// is overridden off. Unknown policy (load failed) = not forced (fail open).
	const forcedOff = (key: string, ch: Channel) =>
		!!policy && (policy.channels[ch] === false || policy.disabled[key]?.[ch] === true);
	const emailPaused = $derived(!!policy && policy.channels.email === false);
	const pushPaused = $derived(!!policy && policy.channels.push === false);

	// Absent pref defaults to ON (matches the backend default-on semantics). A
	// forced-off channel reads as off regardless of the stored pref.
	const isOn = (key: string, ch: Channel) =>
		!forcedOff(key, ch) && prefs[key]?.[ch] !== false;

	let testMsg = $state('');
	let testBusy = $state(false);
	async function sendTest() {
		testMsg = '';
		testBusy = true;
		try {
			const { sent, total } = await push.test();
			testMsg =
				sent > 0
					? `Test sent to ${sent}/${total} device(s) — watch for a notification.`
					: `No device accepted it (${total} tried).`;
		} catch (e: unknown) {
			testMsg = (e as { message?: string })?.message ?? 'Test failed.';
		} finally {
			testBusy = false;
		}
	}

	async function toggleNotify(key: string, ch: Channel) {
		if (forcedOff(key, ch)) return; // admin-paused — not user-changeable
		const next = {
			...prefs,
			[key]: { ...prefs[key], [ch]: !isOn(key, ch) }
		};
		const prev = prefs;
		prefs = next;
		notifyError = '';
		notifyBusy = true;
		try {
			await auth.updateNotifyPrefs(next);
		} catch (err: unknown) {
			prefs = prev; // revert on failure
			notifyError =
				(err as { message?: string })?.message ??
				'Could not save notification settings.';
		} finally {
			notifyBusy = false;
		}
	}

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

	<section class="card">
		<h3>Notifications</h3>
		<p class="muted small">
			Choose how we reach you for each event. Email goes to
			<strong>{auth.user?.email ?? ''}</strong>; push arrives on this device.
		</p>

		<div class="push-device">
			{#if !push.supported}
				<p class="muted small">
					Push isn't supported in this browser. On iPhone/iPad, add the app to
					your Home Screen first.
				</p>
			{:else if push.blocked}
				<p class="muted small">
					Push is blocked in your browser settings — re-allow notifications for
					this site to enable it.
				</p>
			{:else if push.subscribed}
				<div class="push-row">
					<span class="ok small">✓ Push enabled on this device</span>
					<div class="push-actions">
						<button
							type="button"
							class="btn secondary tiny"
							onclick={sendTest}
							disabled={testBusy}
						>
							{testBusy ? 'Sending…' : 'Send test'}
						</button>
						<button
							type="button"
							class="btn secondary tiny"
							onclick={() => push.disable()}
							disabled={push.busy}
						>
							{push.busy ? 'Working…' : 'Disable'}
						</button>
					</div>
				</div>
				{#if testMsg}<p class="muted small">{testMsg}</p>{/if}
			{:else}
				<button
					type="button"
					class="btn secondary"
					onclick={() => push.enable()}
					disabled={push.busy}
				>
					{push.busy ? 'Enabling…' : 'Enable push on this device'}
				</button>
			{/if}
			{#if push.error}<p class="error small">{push.error}</p>{/if}
		</div>

		{#if notifyError}<p class="error">{notifyError}</p>{/if}
		{#if emailPaused || pushPaused}
			<p class="paused-note small">
				{#if emailPaused && pushPaused}
					Email and push notifications are temporarily paused by the admins.
				{:else if emailPaused}
					Email notifications are temporarily paused by the admins.
				{:else}
					Push notifications are temporarily paused by the admins.
				{/if}
			</p>
		{/if}
		<ul class="notify-list">
			<li class="notify-row notify-head">
				<span></span>
				<span class="col-label">Email</span>
				<span class="col-label">Push</span>
			</li>
			{#each NOTIFY_EVENTS as ev (ev.key)}
				<li class="notify-row">
					<div class="notify-text">
						<span class="notify-label">{ev.label}</span>
						<span class="muted notify-hint">{ev.hint}</span>
					</div>
					{#each ['email', 'push'] as const as ch}
						<button
							type="button"
							role="switch"
							aria-checked={isOn(ev.key, ch)}
							aria-label={`${ev.label} — ${ch}${forcedOff(ev.key, ch) ? ' (paused by admins)' : ''}`}
							title={forcedOff(ev.key, ch) ? 'Paused by the admins' : undefined}
							class="toggle"
							class:on={isOn(ev.key, ch)}
							class:forced={forcedOff(ev.key, ch)}
							onclick={() => toggleNotify(ev.key, ch)}
							disabled={notifyBusy || forcedOff(ev.key, ch) || (ch === 'push' && !push.subscribed)}
						>
							<span class="knob"></span>
						</button>
					{/each}
				</li>
			{/each}
		</ul>
		{#if push.supported && !push.subscribed}
			<p class="muted hint">Enable push above to use the Push toggles.</p>
		{/if}
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
	.notify-list {
		list-style: none;
		margin: 0.5rem 0 0;
		padding: 0;
	}
	.push-device {
		margin: 0 0 0.5rem;
	}
	.push-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
	}
	.push-actions {
		display: flex;
		gap: 0.5rem;
	}
	.btn.tiny {
		padding: 0.3rem 0.7rem;
		font-size: 0.8rem;
	}
	.notify-row {
		display: grid;
		grid-template-columns: 1fr 44px 44px;
		align-items: center;
		gap: 1rem;
		padding: 0.85rem 0;
		border-top: 1px solid var(--border);
	}
	.notify-head {
		padding: 0.2rem 0 0.4rem;
		border-top: none;
	}
	.col-label {
		text-align: center;
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--muted);
	}
	.notify-text {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
	}
	.notify-label {
		font-size: 0.95rem;
		font-weight: 600;
	}
	.notify-hint {
		font-size: 0.8rem;
		line-height: 1.4;
	}
	.toggle {
		flex: none;
		width: 44px;
		height: 26px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--border);
		background: var(--surface-2);
		padding: 2px;
		cursor: pointer;
		transition:
			background 0.15s ease,
			border-color 0.15s ease;
	}
	.toggle:disabled {
		opacity: 0.6;
		cursor: default;
	}
	/* Admin-paused: dim further and strike a faint diagonal so it reads as
	   "locked off by someone else", distinct from a user-chosen off. */
	.toggle.forced {
		opacity: 0.4;
		background: repeating-linear-gradient(
			-45deg,
			var(--surface-2),
			var(--surface-2) 4px,
			var(--border) 4px,
			var(--border) 5px
		);
	}
	.toggle.on {
		background: var(--accent);
		border-color: var(--accent);
	}
	.paused-note {
		margin: 0 0 0.6rem;
		padding: 0.5rem 0.7rem;
		border-radius: var(--radius-sm);
		background: color-mix(in srgb, var(--warning) 12%, transparent);
		border: 1px solid color-mix(in srgb, var(--warning) 35%, var(--border));
		color: var(--text);
	}
	.knob {
		display: block;
		width: 20px;
		height: 20px;
		border-radius: 50%;
		background: var(--text);
		transition: transform 0.15s ease;
	}
	.toggle.on .knob {
		transform: translateX(18px);
		background: var(--accent-fg);
	}
</style>
