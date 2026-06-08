<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import {
		api,
		type NotifyPolicy,
		type NotifyChannel,
		type NotifyPolicyPayload
	} from '$lib/api';
	import { NOTIFY_EVENTS } from '$lib/notify';
	import { Megaphone } from '@lucide/svelte';

	// Global notification delivery policy: master per-channel kill switches +
	// per-event overrides. A toggle reads as "delivering" (on) vs "suppressed"
	// (off); flipping it persists immediately. Owner/admin only — rendered inside
	// an admin-gated page.
	const CHANNELS: NotifyChannel[] = ['email', 'push'];
	let policy = $state<NotifyPolicy | null>(null);
	let policyBusy = $state(false);
	let policyError = $state('');

	$effect(() => {
		if (auth.isAdmin && !policy) {
			api.notifyPolicy()
				.then((p) => (policy = p))
				.catch(() => (policyError = 'Could not load notification policy.'));
		}
	});

	// Master switch is on unless explicitly false; an event delivers on a channel
	// unless its override marks it suppressed.
	const masterOn = (ch: NotifyChannel) => policy?.channels[ch] !== false;
	const eventOn = (key: string, ch: NotifyChannel) => !policy?.disabled[key]?.[ch];

	// Persist a patch optimistically, reverting to the server's truth on failure.
	async function savePolicy(patch: NotifyPolicyPayload, optimistic: NotifyPolicy) {
		const prev = policy;
		policy = optimistic;
		policyBusy = true;
		policyError = '';
		try {
			policy = await api.updateNotifyPolicy(patch);
		} catch (e) {
			policy = prev;
			policyError = e instanceof Error ? e.message : 'Could not save.';
		} finally {
			policyBusy = false;
		}
	}

	function toggleMaster(ch: NotifyChannel) {
		if (!policy) return;
		const next = !masterOn(ch);
		savePolicy(
			{ channels: { [ch]: next } },
			{ ...policy, channels: { ...policy.channels, [ch]: next } }
		);
	}

	function toggleEvent(key: string, ch: NotifyChannel) {
		if (!policy) return;
		const suppress = eventOn(key, ch); // currently delivering → suppress it
		const disabled: NotifyPolicy['disabled'] = {};
		for (const [k, v] of Object.entries(policy.disabled)) disabled[k] = { ...v };
		const row = { ...(disabled[key] ?? {}) };
		if (suppress) row[ch] = true;
		else delete row[ch];
		if (Object.keys(row).length) disabled[key] = row;
		else delete disabled[key];
		savePolicy({ disabled }, { ...policy, disabled });
	}
</script>

<section class="card policy">
	<h2 class="sec"><Megaphone size={18} /> Notification delivery</h2>
	<p class="policy-intro">
		Master switches pause a whole channel platform-wide — use them when a
		provider is down (e.g. mail suspended). The grid below silences individual
		events. Paused channels are greyed out for users in their settings.
	</p>
	{#if policyError}<p class="err">{policyError}</p>{/if}
	{#if !policy}
		<p class="muted">Loading…</p>
	{:else}
		<div class="masters">
			{#each CHANNELS as ch (ch)}
				<button
					type="button"
					role="switch"
					aria-checked={masterOn(ch)}
					class="master"
					class:on={masterOn(ch)}
					onclick={() => toggleMaster(ch)}
					disabled={policyBusy}
				>
					<span class="m-knob"></span>
					<span class="m-label">{ch === 'email' ? 'Email' : 'Push'}</span>
					<span class="m-state">{masterOn(ch) ? 'On' : 'Paused'}</span>
				</button>
			{/each}
		</div>

		<ul class="grid">
			<li class="grow notify-head">
				<span></span>
				<span class="col-label">Email</span>
				<span class="col-label">Push</span>
			</li>
			{#each NOTIFY_EVENTS as ev (ev.key)}
				<li class="grow">
					<div class="g-text">
						<span class="g-label">{ev.label}</span>
						<span class="muted g-hint">{ev.hint}</span>
					</div>
					{#each CHANNELS as ch (ch)}
						{@const on = masterOn(ch) && eventOn(ev.key, ch)}
						<button
							type="button"
							role="switch"
							aria-checked={on}
							aria-label={`${ev.label} — ${ch}`}
							title={!masterOn(ch) ? `${ch} is paused by the master switch` : undefined}
							class="toggle"
							class:on
							onclick={() => toggleEvent(ev.key, ch)}
							disabled={policyBusy || !masterOn(ch)}
						>
							<span class="knob"></span>
						</button>
					{/each}
				</li>
			{/each}
		</ul>
	{/if}
</section>

<style>
	.sec {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		margin: 0 0 0.9rem;
		font-size: 0.95rem;
		font-weight: 700;
		color: var(--muted);
	}
	.err {
		color: var(--danger);
		font-size: 0.85rem;
	}
	.policy {
		margin-bottom: 1.4rem;
	}
	.policy-intro {
		margin: 0 0 1rem;
		font-size: 0.85rem;
		line-height: 1.5;
		color: var(--muted);
	}
	.masters {
		display: flex;
		flex-wrap: wrap;
		gap: 0.7rem;
		margin-bottom: 1.2rem;
	}
	.master {
		display: inline-flex;
		align-items: center;
		gap: 0.55rem;
		padding: 0.5rem 0.85rem 0.5rem 0.55rem;
		border: 1px solid var(--border);
		border-radius: var(--radius-pill);
		background: var(--surface-2);
		color: var(--text);
		cursor: pointer;
		transition:
			border-color 0.15s ease,
			background 0.15s ease;
	}
	.master:disabled {
		opacity: 0.6;
		cursor: default;
	}
	.master .m-knob {
		width: 18px;
		height: 18px;
		border-radius: 50%;
		background: var(--muted);
		transition: background 0.15s ease;
	}
	.master.on {
		border-color: color-mix(in srgb, var(--accent) 55%, var(--border));
	}
	.master.on .m-knob {
		background: var(--accent);
	}
	.master .m-label {
		font-weight: 600;
		font-size: 0.9rem;
	}
	.master .m-state {
		font-size: 0.72rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--muted);
	}
	.master.on .m-state {
		color: var(--success);
	}
	.grid {
		list-style: none;
		padding: 0;
		margin: 0;
	}
	.grow {
		display: grid;
		grid-template-columns: 1fr 44px 44px;
		align-items: center;
		gap: 0.6rem;
		padding: 0.55rem 0;
		border-top: 1px solid var(--border);
	}
	.grow.notify-head {
		border-top: none;
		padding-bottom: 0.3rem;
	}
	.col-label {
		font-size: 0.72rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--muted);
		text-align: center;
	}
	.g-text {
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
		min-width: 0;
	}
	.g-label {
		font-size: 0.9rem;
		font-weight: 600;
	}
	.g-hint {
		font-size: 0.76rem;
		line-height: 1.4;
	}
	.toggle {
		justify-self: center;
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
		opacity: 0.5;
		cursor: default;
	}
	.toggle.on {
		background: var(--accent);
		border-color: var(--accent);
	}
	.toggle .knob {
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
