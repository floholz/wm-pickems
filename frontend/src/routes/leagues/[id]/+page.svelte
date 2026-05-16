<script lang="ts">
	import { page } from '$app/stores';
	import { api, type LeaderboardRow } from '$lib/api';
	import { auth } from '$lib/auth.svelte';
	import {
		Eye,
		EyeOff,
		Copy,
		Share2,
		ChevronDown,
		Telescope
	} from '@lucide/svelte';

	interface Cfg {
		match: {
			tendency: number;
			exact: number;
			totalGoals: number;
			goalDiff: number;
		};
		forecast: {
			groupPosition: number;
			perfectGroupBonus: number;
			advance: number;
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

	let revealed = $state(false);
	let openRow = $state<string | null>(null);

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
		cfg = null;
		Promise.all([api.leaderboard(lid), api.myLeagues()])
			.then(([lb, mine]) => {
				league = lb.league;
				rows = lb.rows;
				cfg = (lb.scoring as Cfg | undefined) ?? null;
				invite = mine.leagues.find((l) => l.id === lid)?.inviteCode ?? '';
			})
			.catch(() => (error = 'Could not load this league.'))
			.finally(() => (loaded = true));
	});

	let sorted = $derived(
		[...rows].sort((a, b) => b[tab] - a[tab])
	);
	let fcView = $derived(tab === 'forecastPoints');

	function copyInvite() {
		navigator.clipboard?.writeText(invite);
	}

	let linkCopied = $state(false);
	let copyTimer: ReturnType<typeof setTimeout>;
	function shareInvite() {
		const url = `${window.location.origin}/join/${invite}`;
		navigator.clipboard?.writeText(url);
		linkCopied = true;
		clearTimeout(copyTimer);
		copyTimer = setTimeout(() => (linkCopied = false), 1800);
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
		<div class="irow">
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
		</div>
		<button class="btn share" onclick={shareInvite}>
			<Share2 size={16} />
			{linkCopied ? 'Link copied!' : 'Share invite link'}
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
				<tr>
					<th>#</th>
					<th>Player</th>
					{#if fcView}
						<th class="num ext" title="Correct group positions">Grp</th>
						<th class="num ext" title="Correct advancers (group stage)">Adv</th>
						<th class="num ext" title="Predicted teams that reached the Round of 32">R32</th>
						<th class="num ext" title="…Round of 16">R16</th>
						<th class="num ext" title="…Quarter-finals">QF</th>
						<th class="num ext" title="…Semi-finals">SF</th>
						<th class="num ext" title="…the Final">F</th>
						<th class="num ext" title="Champion predicted correctly">Win</th>
					{:else}
						<th class="num ext" title="Matches predicted">Pred</th>
						<th class="num ext" title="Forecast points">FC</th>
						<th class="num ext" title="Exact scores (tiebreak 1)">Exact</th>
						<th class="num ext" title="Correct winners (tiebreak 2)">Win</th>
						<th class="num ext" title="Goal-diff error (tiebreak 3, lower is better)">GD&Delta;</th>
					{/if}
					<th class="num pts">Pts</th>
				</tr>
			</thead>
			<tbody>
				{#each sorted as r, i (r.userId)}
					{@const f = r.forecast ?? {}}
					<tr
						class:lead={r.userId === auth.user?.id}
						class="main"
						class:open={openRow === r.userId}
						onclick={() =>
							(openRow = openRow === r.userId ? null : r.userId)}
					>
						<td class="rank">{i + 1}</td>
						<td class="player">
							<div class="pwrap">
								<span class="pname">{r.name}</span>
								<a
									class="fclink"
									href={`/forecast/${r.userId}`}
									title="View {r.name}'s forecast"
									onclick={(e) => e.stopPropagation()}
								>
									<Telescope size={15} />
								</a>
								<ChevronDown size={14} class="rx" />
							</div>
						</td>
						{#if fcView}
							<td class="num ext digits">{f.groups ?? 0}</td>
							<td class="num ext digits">{f.advance ?? 0}</td>
							<td class="num ext digits">{f.R32 ?? 0}</td>
							<td class="num ext digits">{f.R16 ?? 0}</td>
							<td class="num ext digits">{f.QF ?? 0}</td>
							<td class="num ext digits">{f.SF ?? 0}</td>
							<td class="num ext digits">{f.FINAL ?? 0}</td>
							<td class="num ext digits">{f.champion ? '✓' : '–'}</td>
						{:else}
							<td class="num ext digits">{r.predicted}</td>
							<td class="num ext digits">{r.forecastPoints}</td>
							<td class="num ext digits">{r.exactScores}</td>
							<td class="num ext digits">{r.correctWinners}</td>
							<td class="num ext digits">{r.gdDeviation}</td>
						{/if}
						<td class="num pts digits">{r[tab]}</td>
					</tr>
					{#if openRow === r.userId}
						<tr class="detail">
							<td colspan="12">
								{#if fcView}
									<div class="stats">
										<span><i>Correct group positions</i><b>{f.groups ?? 0}</b></span>
										<span><i>Correct advancers</i><b>{f.advance ?? 0}</b></span>
										<span><i>Reached Round of 32</i><b>{f.R32 ?? 0}</b></span>
										<span><i>Reached Round of 16</i><b>{f.R16 ?? 0}</b></span>
										<span><i>Reached Quarter-finals</i><b>{f.QF ?? 0}</b></span>
										<span><i>Reached Semi-finals</i><b>{f.SF ?? 0}</b></span>
										<span><i>Reached the Final</i><b>{f.FINAL ?? 0}</b></span>
										<span><i>Champion correct</i><b>{f.champion ? 'Yes' : 'No'}</b></span>
									</div>
								{:else}
									<div class="stats">
										<span><i>Matches predicted</i><b>{r.predicted}</b></span>
										<span><i>Tip points</i><b>{r.tipsPoints}</b></span>
										<span><i>Forecast points</i><b>{r.forecastPoints}</b></span>
										<span><i>Exact scores</i><b>{r.exactScores}</b></span>
										<span><i>Correct winners</i><b>{r.correctWinners}</b></span>
										<span><i>Goal-diff error</i><b>{r.gdDeviation}</b></span>
									</div>
								{/if}
							</td>
						</tr>
					{/if}
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

			<h4>Per match (your Tip) — max {cfg.match.tendency +
					cfg.match.exact +
					cfg.match.totalGoals +
					cfg.match.goalDiff} pt</h4>
			<ul class="leg">
				<li>
					<span>Correct result — group: 1 / X / 2; knockout: the team
						that advances</span><b>{cfg.match.tendency} pt</b>
				</li>
				<li><span>Exact score</span><b>+{cfg.match.exact} pt</b></li>
				<li><span>Correct total number of goals</span><b>+{cfg.match.totalGoals} pt</b></li>
				<li><span>Correct goal difference</span><b>+{cfg.match.goalDiff} pt</b></li>
			</ul>
			<p class="muted small">
				Knockout games have no draw — the result point is for the team
				that goes through. If a knockout game is decided in extra time,
				the score points use the after-extra-time score.
			</p>

			<h4>Tournament Forecast</h4>
			<ul class="leg">
				<li><span>Each team in its correct final group position</span><b>{cfg.forecast.groupPosition} pt</b></li>
				<li><span>Whole group ordered perfectly (bonus)</span><b>+{cfg.forecast.perfectGroupBonus} pt</b></li>
				<li>
					<span>Each team you predicted to advance (group top 2, or a
						best-third pick) that actually advances</span
					><b>{cfg.forecast.advance} pt</b>
				</li>
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
	.irow {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.share {
		margin-top: 0.85rem;
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
	.lb th.num,
	.lb td.num {
		text-align: right;
	}

	/* Pts is the focus — set it apart from the stat columns. */
	.lb th.pts,
	.lb td.pts {
		padding-left: 1.15rem;
		border-left: 1px solid var(--border);
		font-size: 1.02rem;
	}
	.lb th.pts {
		font-size: 0.8rem;
	}

	/* Extra tiebreaker columns: desktop only. */
	.ext {
		display: none;
	}
	.player {
		width: 100%;
	}
	.pwrap {
		display: flex;
		align-items: center;
		gap: 0.4rem;
	}
	.pname {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.fclink {
		display: inline-grid;
		place-items: center;
		color: var(--muted);
		flex: none;
	}
	.fclink:hover {
		color: var(--accent);
	}
	:global(.lb .rx) {
		color: var(--muted);
		transition: transform 0.15s ease;
		margin-left: auto;
	}
	tr.main.open :global(.rx) {
		transform: rotate(180deg);
	}
	tr.main {
		cursor: pointer;
	}
	.detail td {
		padding: 0 0.4rem 0.7rem;
	}
	.stats {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.4rem 1rem;
	}
	.stats span {
		display: flex;
		justify-content: space-between;
		gap: 0.6rem;
		padding: 0.35rem 0;
		border-bottom: 1px solid var(--border);
	}
	.stats i {
		color: var(--muted);
		font-style: normal;
		font-size: 0.85rem;
	}
	.stats b {
		font-family: var(--font-mono);
	}

	@media (min-width: 760px) {
		.ext {
			display: table-cell;
		}
		:global(.lb .rx) {
			display: none;
		}
		tr.main {
			cursor: default;
		}
		.detail {
			display: none;
		}
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
