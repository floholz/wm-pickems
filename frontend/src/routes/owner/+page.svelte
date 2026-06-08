<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth } from '$lib/auth.svelte';
	import { api, type OwnerStats } from '$lib/api';
	import { Users, Trophy, Bell, RefreshCw } from '@lucide/svelte';

	let stats = $state<OwnerStats | null>(null);
	let error = $state('');
	let loaded = $state(false);
	let busy = $state(false);

	// Gate: send anyone who isn't the owner home. The auth store hydrates
	// synchronously from the stored token, so isOwner is accurate on mount.
	$effect(() => {
		if (!auth.isOwner) goto('/');
	});

	async function load() {
		busy = true;
		error = '';
		try {
			stats = await api.ownerStats();
		} catch {
			error = 'Could not load stats.';
		} finally {
			loaded = true;
			busy = false;
		}
	}

	$effect(() => {
		if (auth.isOwner && !loaded) load();
	});
</script>

<div class="head">
	<div>
		<p class="kicker">App owner</p>
		<h1>Owner stats</h1>
	</div>
	{#if auth.isOwner}
		<button class="btn ghost refresh" onclick={load} disabled={busy} title="Refresh">
			<RefreshCw size={16} class={busy ? 'spin' : ''} /> Refresh
		</button>
	{/if}
</div>

{#if !auth.isOwner}
	<p class="muted">Restricted.</p>
{:else if !loaded}
	<p class="muted">Loading…</p>
{:else if error}
	<p class="err">{error}</p>
{:else if stats}
	<section class="card">
		<h2 class="sec"><Users size={18} /> Users</h2>
		<div class="grid">
			<div class="stat">
				<span class="num digits">{stats.users}</span>
				<span class="lbl">Total users</span>
				<span class="hint">bots excluded</span>
			</div>
			<div class="stat">
				<span class="num digits">+{stats.usersLast24h}</span>
				<span class="lbl">New · last 24h</span>
			</div>
			<div class="stat">
				<span class="num digits">{stats.activeUsers}</span>
				<span class="lbl">Active</span>
				<span class="hint">≥3 tips or a full forecast</span>
			</div>
		</div>
	</section>

	<section class="card">
		<h2 class="sec"><Trophy size={18} /> Leagues</h2>
		<div class="grid">
			<div class="stat">
				<span class="num digits">{stats.leagues}</span>
				<span class="lbl">Leagues</span>
				<span class="hint">user-created (Global excluded)</span>
			</div>
			<div class="stat">
				<span class="num digits">{stats.activeLeagues}</span>
				<span class="lbl">Active</span>
				<span class="hint">&gt;1 member &amp; some tips</span>
			</div>
		</div>
	</section>

	<section class="card">
		<h2 class="sec"><Bell size={18} /> Notifications</h2>
		<div class="grid">
			<div class="stat">
				<span class="num digits">{stats.pushEnabled}</span>
				<span class="lbl">Push enabled</span>
			</div>
			<div class="stat">
				<span class="num digits">{stats.notifyDisabled}</span>
				<span class="lbl">Opted out</span>
				<span class="hint">disabled ≥1 notification</span>
			</div>
		</div>
	</section>
{/if}

<style>
	.head {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 1rem;
		flex-wrap: wrap;
	}
	.refresh {
		width: auto;
	}
	:global(.refresh .spin) {
		animation: spin 0.8s linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	.sec {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		margin: 0 0 0.9rem;
		font-size: 0.95rem;
		font-weight: 700;
		color: var(--muted);
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
		gap: 0.75rem;
	}
	.stat {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		padding: 0.9rem 1rem;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
	}
	.num {
		font-size: 1.9rem;
		font-weight: 800;
		line-height: 1.1;
		color: var(--accent);
	}
	.lbl {
		font-weight: 600;
	}
	.hint {
		font-size: 0.78rem;
		color: var(--muted);
	}
	.err {
		color: var(--danger);
	}
</style>
