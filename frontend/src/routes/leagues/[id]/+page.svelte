<script lang="ts">
	import { page } from '$app/stores';
	import { api, type LeaderboardRow } from '$lib/api';

	let id = $derived($page.params.id ?? '');
	let league = $state<{ id: string; name: string } | null>(null);
	let rows = $state<LeaderboardRow[]>([]);
	let invite = $state('');
	let loaded = $state(false);
	let error = $state('');
	let tab = $state<'total' | 'tipsPoints' | 'forecastPoints'>('total');

	$effect(() => {
		const lid = id;
		loaded = false;
		Promise.all([api.leaderboard(lid), api.myLeagues()])
			.then(([lb, mine]) => {
				league = lb.league;
				rows = lb.rows;
				invite = mine.leagues.find((l) => l.id === lid)?.inviteCode ?? '';
			})
			.catch(() => (error = 'Could not load this league.'))
			.finally(() => (loaded = true));
	});

	let sorted = $derived(
		[...rows].sort((a, b) => b[tab] - a[tab])
	);

	function copyInvite() {
		navigator.clipboard?.writeText(invite);
	}
</script>

<a href="/leagues" class="muted back">← Leagues</a>

{#if error}
	<p class="error">{error}</p>
{:else if !loaded}
	<p class="muted">Loading…</p>
{:else if league}
	<p class="kicker">League</p>
	<h1>{league.name}</h1>

	<section class="card invite">
		<div>
			<div class="muted small">Invite code</div>
			<div class="code">{invite}</div>
		</div>
		<div class="spacer"></div>
		<button class="btn secondary copy" onclick={copyInvite}>Copy</button>
	</section>

	<section class="card">
		<div class="tabs">
			<button class:active={tab === 'total'} onclick={() => (tab = 'total')}>Overall</button>
			<button class:active={tab === 'tipsPoints'} onclick={() => (tab = 'tipsPoints')}>Tips</button>
			<button class:active={tab === 'forecastPoints'} onclick={() => (tab = 'forecastPoints')}>Forecast</button>
		</div>

		<table class="lb">
			<thead>
				<tr><th>#</th><th>Player</th><th class="num">Pts</th></tr>
			</thead>
			<tbody>
				{#each sorted as r, i (r.userId)}
					<tr class:lead={i === 0}>
						<td class="rank">{i + 1}</td>
						<td>{r.name}</td>
						<td class="num digits">{r[tab]}</td>
					</tr>
				{/each}
			</tbody>
		</table>
		<p class="muted small note">
			Points update automatically as results come in. Ties break on exact
			scores, then correct winners, then goal-difference accuracy.
		</p>
	</section>
{/if}

<style>
	.back {
		display: inline-block;
		margin: 0.5rem 0 0.75rem;
	}
	h1 {
		margin: 0 0 1rem;
	}
	.invite {
		display: flex;
		align-items: center;
	}
	.small {
		font-size: 0.8rem;
	}
	.code {
		font-weight: 800;
		letter-spacing: 0.2em;
		font-size: 1.3rem;
	}
	.copy {
		width: auto;
	}
	.tabs {
		display: flex;
		gap: 0.4rem;
		margin-bottom: 0.75rem;
	}
	.tabs button {
		flex: 1;
		padding: 0.5rem;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--muted);
		font-weight: 600;
	}
	.tabs button.active {
		color: var(--accent-fg);
		background: var(--accent);
		border-color: var(--accent);
	}
	.lb {
		width: 100%;
		border-collapse: collapse;
	}
	.lb th,
	.lb td {
		text-align: left;
		padding: 0.6rem 0.4rem;
		border-bottom: 1px solid var(--border);
	}
	.lb th {
		color: var(--muted);
		font-size: 0.8rem;
		font-weight: 600;
	}
	.num {
		text-align: right;
	}
	.rank {
		width: 2rem;
		color: var(--muted);
		font-family: var(--font-mono);
	}
	tr.lead td {
		background: color-mix(in srgb, var(--accent) 9%, transparent);
	}
	tr.lead .rank {
		color: var(--accent);
		font-weight: 800;
	}
	.note {
		margin: 0.75rem 0 0;
	}
</style>
