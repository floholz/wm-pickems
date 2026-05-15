<script lang="ts">
	import { page } from '$app/stores';
	import { api, type LeaderboardRow } from '$lib/api';
	import { pb } from '$lib/pb';
	import { Eye, EyeOff, Copy } from '@lucide/svelte';

	interface Cfg {
		match: {
			tendency: number;
			exact: number;
			totalGoals: number;
			goalDiff: number;
			koOtBonus: boolean;
			advancer: number;
		};
		forecast: {
			groupPosition: number;
			perfectGroupBonus: number;
			thirdQualifier: number;
			round: Record<string, number>;
		};
		tiebreakers: string[];
	}
	let cfg = $state<Cfg | null>(null);

	const tbLabel: Record<string, string> = {
		points: 'Total points',
		exactScores: 'Most exact scores',
		correctWinners: 'Most correct winners',
		goalDiffDeviation: 'Smallest goal-difference error vs. results',
		earliestEdit: 'Earliest last edit (submitted first)'
	};
	const roundLabel: Record<string, string> = {
		R32: 'Round of 32',
		R16: 'Round of 16',
		QF: 'Quarter-final',
		SF: 'Semi-final',
		FINAL: 'Final',
		CHAMPION: 'Champion'
	};

	async function loadConfig(lid: string) {
		try {
			const lg = await pb.collection('leagues').getOne(lid);
			const scId = lg.scoringConfig as string;
			const rec = scId
				? await pb.collection('scoring_configs').getOne(scId)
				: await pb
						.collection('scoring_configs')
						.getFirstListItem('isDefault = true');
			cfg = rec.config as Cfg;
		} catch {
			/* legend just won't show */
		}
	}

	let revealed = $state(false);

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
		loadConfig(lid);
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
		<div class="ic">
			<div class="muted small">Invite code</div>
			<div class="code" class:masked={!revealed}>
				{revealed ? invite : '•'.repeat(invite.length || 6)}
			</div>
		</div>
		<div class="spacer"></div>
		<button
			class="btn secondary eye"
			aria-label={revealed ? 'Hide code' : 'Reveal code'}
			onclick={() => (revealed = !revealed)}
		>
			{#if revealed}<EyeOff size={18} />{:else}<Eye size={18} />{/if}
		</button>
		<button class="btn secondary copy" onclick={copyInvite}>
			<Copy size={16} /> Copy
		</button>
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
			Points update automatically as results come in.
		</p>
	</section>

	{#if cfg}
		<details class="card legend">
			<summary>How points work</summary>

			<h4>Per match (your Tip)</h4>
			<ul class="leg">
				<li><span>Correct result (1 / X / 2)</span><b>{cfg.match.tendency} pt</b></li>
				<li><span>Exact score</span><b>+{cfg.match.exact} pt</b></li>
				<li><span>Correct total number of goals</span><b>+{cfg.match.totalGoals} pt</b></li>
				<li><span>Correct goal difference</span><b>+{cfg.match.goalDiff} pt</b></li>
				<li><span>Knockout: correct team advances</span><b>+{cfg.match.advancer} pt</b></li>
				{#if cfg.match.koOtBonus}
					<li>
						<span>Knockout going to extra time: the score rules also
							apply to your after-extra-time prediction</span>
						<b>bonus</b>
					</li>
				{/if}
			</ul>
			<p class="muted small">
				A perfect score therefore earns
				{cfg.match.tendency +
					cfg.match.exact +
					cfg.match.totalGoals +
					cfg.match.goalDiff} pt (group stage).
			</p>

			<h4>Tournament Forecast</h4>
			<ul class="leg">
				<li><span>Each team in its correct final group position</span><b>{cfg.forecast.groupPosition} pt</b></li>
				<li><span>Whole group ordered perfectly (bonus)</span><b>+{cfg.forecast.perfectGroupBonus} pt</b></li>
				<li><span>Each correctly predicted best-third qualifier</span><b>{cfg.forecast.thirdQualifier} pt</b></li>
			</ul>
			<p class="muted small">
				Reaching a knockout round (per correctly predicted team):
			</p>
			<ul class="leg">
				{#each Object.entries(roundLabel) as [k, lbl] (k)}
					{#if cfg.forecast.round[k] != null}
						<li><span>{lbl}</span><b>{cfg.forecast.round[k]} pt</b></li>
					{/if}
				{/each}
			</ul>

			<h4>Tiebreakers (in order)</h4>
			<ol class="tiebreak">
				{#each cfg.tiebreakers as t (t)}
					<li>{tbLabel[t] ?? t}</li>
				{/each}
			</ol>
		</details>
	{/if}
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
		gap: 0.5rem;
	}
	.ic {
		min-width: 0;
	}
	.small {
		font-size: 0.8rem;
	}
	.code {
		font-family: var(--font-mono);
		font-weight: 700;
		letter-spacing: 0.2em;
		font-size: 1.3rem;
	}
	.code.masked {
		color: var(--muted);
		letter-spacing: 0.15em;
	}
	.eye {
		width: auto;
		padding: 0.7rem;
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
	.legend summary {
		cursor: pointer;
		font-weight: 700;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		font-size: 0.85rem;
		color: var(--accent);
	}
	.legend h4 {
		margin: 1rem 0 0.5rem;
		font-size: 0.95rem;
	}
	.legend .small {
		margin: 0.4rem 0 0;
	}
	ul.leg {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	ul.leg li {
		display: flex;
		align-items: baseline;
		gap: 0.75rem;
		padding: 0.4rem 0;
		border-bottom: 1px solid var(--border);
	}
	ul.leg li span {
		flex: 1;
	}
	ul.leg li b {
		font-family: var(--font-mono);
		color: var(--accent);
		white-space: nowrap;
	}
	ol.tiebreak {
		margin: 0.5rem 0 0;
		padding-left: 1.3rem;
		line-height: 1.8;
	}
	ol.tiebreak li {
		padding-left: 0.3rem;
	}
</style>
