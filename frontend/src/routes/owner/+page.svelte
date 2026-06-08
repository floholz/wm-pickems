<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth } from '$lib/auth.svelte';
	import { api, type OwnerStats, type SyncStatus } from '$lib/api';
	import { Users, Trophy, Bell, RefreshCw, Activity, Check, X } from '@lucide/svelte';

	let stats = $state<OwnerStats | null>(null);
	let error = $state('');
	let loaded = $state(false);
	let busy = $state(false);

	// Results-sync status (separate load so a slow API-Football status call
	// doesn't hold up the stats).
	let sync = $state<SyncStatus | null>(null);
	let syncLoaded = $state(false);
	let syncBusy = $state(false);
	let syncMsg = $state('');

	async function loadSync() {
		try {
			sync = await api.syncStatus();
		} catch {
			/* leave null — card shows unavailable */
		} finally {
			syncLoaded = true;
		}
	}

	async function runSync() {
		syncBusy = true;
		syncMsg = '';
		try {
			const r = await api.syncRun();
			await loadSync();
			syncMsg =
				r.updated != null
					? `Synced — ${r.updated} match${r.updated === 1 ? '' : 'es'} updated.`
					: 'Synced.';
		} catch (e) {
			syncMsg = e instanceof Error ? e.message : 'Sync failed.';
		} finally {
			syncBusy = false;
		}
	}

	// Coarse "x ago" for the last-sync timestamp.
	function ago(iso: string): string {
		const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
		if (s < 60) return 'just now';
		if (s < 3600) return `${Math.floor(s / 60)} min ago`;
		if (s < 86400) return `${Math.floor(s / 3600)} h ago`;
		return `${Math.floor(s / 86400)} d ago`;
	}

	const sourceLabel: Record<string, string> = {
		'api-football': 'API-Football',
		openfootball: 'openfootball',
		none: 'None'
	};

	// Humanise the common "*/N * * * *" cadence; fall back to the raw expression.
	function cronHuman(expr: string): string {
		const m = expr.match(/^\*\/(\d+) \* \* \* \*$/);
		return m ? `${m[1]} min` : expr;
	}

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
	$effect(() => {
		if (auth.isOwner && !syncLoaded) loadSync();
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

{#if auth.isOwner}
	<section class="card">
		<h2 class="sec"><Activity size={18} /> Results sync</h2>
		{#if !syncLoaded}
			<p class="muted">Loading…</p>
		{:else if !sync}
			<p class="muted">Status unavailable.</p>
		{:else}
			<div class="srows">
				<div class="srow">
					<span class="k">Source</span>
					<span class="v"
						><span class="srcpill" class:af={sync.source === 'api-football'}
							>{sourceLabel[sync.source] ?? sync.source}</span
						></span
					>
				</div>
				<div class="srow">
					<span class="k">Auto-sync</span>
					<span class="v"
						>{sync.autoSync ? `every ${cronHuman(sync.cron)}` : 'off'}</span
					>
				</div>
				<div class="srow">
					<span class="k">Last sync</span>
					<span class="v">
						{#if sync.lastRun}
							{#if sync.lastRun.ok}<Check size={14} class="okico" />{:else}<X
									size={14}
									class="errico"
								/>{/if}
							<span title={new Date(sync.lastRun.at).toLocaleString()}
								>{ago(sync.lastRun.at)}</span
							>
							· {sync.lastRun.updated} updated
						{:else}
							never
						{/if}
					</span>
				</div>
				{#if sync.account?.subscription}
					<div class="srow">
						<span class="k">Plan</span>
						<span class="v"
							>{sync.account.subscription.plan ?? '—'}{#if sync.account.requests}
								· {sync.account.requests.current ?? 0}/{sync.account.requests
									.limit_day ?? '—'} today{/if}</span
						>
					</div>
				{:else if sync.accountError}
					<div class="srow">
						<span class="k">Account</span><span class="v muted"
							>{sync.accountError}</span
						>
					</div>
				{/if}
			</div>

			{#if sync.lastRun && !sync.lastRun.ok && sync.lastRun.error}
				<p class="err">{sync.lastRun.error}</p>
			{/if}

			<div class="syncactions">
				<button
					class="btn"
					disabled={syncBusy || sync.source === 'none'}
					onclick={runSync}
				>
					<RefreshCw size={16} class={syncBusy ? 'spin' : ''} />
					{syncBusy ? 'Syncing…' : 'Sync now'}
				</button>
				{#if syncMsg}<span class="syncmsg">{syncMsg}</span>{/if}
			</div>
		{/if}
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

	/* ---- results sync card ---- */
	.srows {
		display: flex;
		flex-direction: column;
	}
	.srow {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.55rem 0;
		border-bottom: 1px solid var(--border);
	}
	.srow:last-child {
		border-bottom: none;
	}
	.srow .k {
		color: var(--muted);
		font-size: 0.88rem;
	}
	.srow .v {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		font-weight: 600;
		text-align: right;
	}
	.srow .v.muted {
		font-weight: 400;
		color: var(--muted);
	}
	.srcpill {
		padding: 0.15rem 0.55rem;
		border-radius: var(--radius-pill);
		border: 1px solid var(--border);
		background: var(--surface-2);
		font-size: 0.8rem;
		color: var(--muted);
	}
	.srcpill.af {
		color: var(--accent);
		border-color: color-mix(in srgb, var(--accent) 45%, var(--border));
	}
	:global(.srow .okico) {
		color: var(--success);
	}
	:global(.srow .errico) {
		color: var(--danger);
	}
	.syncactions {
		display: flex;
		align-items: center;
		gap: 0.8rem;
		flex-wrap: wrap;
		margin-top: 1rem;
	}
	.syncactions .btn {
		width: auto;
	}
	:global(.syncactions .spin) {
		animation: spin 0.8s linear infinite;
	}
	.syncmsg {
		font-size: 0.85rem;
		color: var(--muted);
	}
</style>
