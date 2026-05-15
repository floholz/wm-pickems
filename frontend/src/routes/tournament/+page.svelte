<script lang="ts">
	import { tipsStore, type Match } from '$lib/tips.svelte';
	import Flag from '$lib/components/Flag.svelte';

	let view = $state<'groups' | 'bracket'>('groups');

	$effect(() => {
		if (!tipsStore.loaded) tipsStore.load().catch(() => {});
	});

	function played(m: Match) {
		return m.status === 'finished' || !!m.finalizedAt;
	}

	interface Standing {
		id: string;
		p: number;
		w: number;
		d: number;
		l: number;
		gf: number;
		ga: number;
		pts: number;
	}

	// Live group tables from finished group matches.
	let groups = $derived.by(() => {
		const byG: Record<string, Record<string, Standing>> = {};
		for (const m of tipsStore.matches) {
			if (m.stage !== 'group' || !played(m)) continue;
			const g = m.groupLetter;
			(byG[g] ||= {});
			for (const id of [m.homeTeam, m.awayTeam])
				byG[g][id] ||= {
					id,
					p: 0,
					w: 0,
					d: 0,
					l: 0,
					gf: 0,
					ga: 0,
					pts: 0
				};
			const H = byG[g][m.homeTeam];
			const A = byG[g][m.awayTeam];
			H.p++;
			A.p++;
			H.gf += m.ftHome;
			H.ga += m.ftAway;
			A.gf += m.ftAway;
			A.ga += m.ftHome;
			if (m.ftHome > m.ftAway) {
				H.w++;
				A.l++;
				H.pts += 3;
			} else if (m.ftHome < m.ftAway) {
				A.w++;
				H.l++;
				A.pts += 3;
			} else {
				H.d++;
				A.d++;
				H.pts++;
				A.pts++;
			}
		}
		return Object.entries(byG)
			.map(([letter, tbl]) => ({
				letter,
				rows: Object.values(tbl).sort(
					(a, b) =>
						b.pts - a.pts ||
						b.gf - b.ga - (a.gf - a.ga) ||
						b.gf - a.gf
				)
			}))
			.sort((a, b) => a.letter.localeCompare(b.letter));
	});

	const stages = ['R32', 'R16', 'QF', 'SF', '3RD', 'FINAL'];
	const stageName: Record<string, string> = {
		R32: 'Round of 32',
		R16: 'Round of 16',
		QF: 'Quarter-finals',
		SF: 'Semi-finals',
		'3RD': 'Third place',
		FINAL: 'Final'
	};
	let bracket = $derived(
		stages.map((s) => ({
			stage: s,
			matches: tipsStore.matches
				.filter((m) => m.stage === s)
				.sort((a, b) => a.num - b.num)
		}))
	);

	function tn(id: string) {
		return tipsStore.team(id);
	}
	function scoreText(m: Match) {
		if (!played(m)) return '';
		let s = `${m.ftHome}–${m.ftAway}`;
		if (m.etHome || m.etAway) s = `${m.etHome}–${m.etAway} a.e.t.`;
		if (m.penHome || m.penAway) s += ` (${m.penHome}–${m.penAway} pen)`;
		return s;
	}
</script>

<p class="kicker">World Cup 2026</p>
<h1>The Tournament</h1>

<div class="seg" style="margin:1rem 0 1.25rem">
	<button class:on={view === 'groups'} onclick={() => (view = 'groups')}>Group tables</button>
	<button class:on={view === 'bracket'} onclick={() => (view = 'bracket')}>Bracket</button>
</div>

{#if !tipsStore.loaded}
	<p class="muted">Loading…</p>
{:else if view === 'groups'}
	{#if groups.length === 0}
		<div class="card empty">
			<p class="muted">No group matches played yet. Tables light up as results come in.</p>
		</div>
	{:else}
		<div class="gwrap stagger">
			{#each groups as g (g.letter)}
				<section class="card grp">
					<div class="ghead"><span class="gl">{g.letter}</span> Group {g.letter}</div>
					<table>
						<thead>
							<tr><th></th><th>Team</th><th>P</th><th>GD</th><th>Pts</th></tr>
						</thead>
						<tbody>
							{#each g.rows as r, i (r.id)}
								<tr class:adv={i < 2} class:third={i === 2}>
									<td class="rk">{i + 1}</td>
									<td class="tm">
										<Flag iso2={tn(r.id)?.iso2 ?? ''} code={tn(r.id)?.fifaCode ?? ''} />
										<span>{tn(r.id)?.name ?? '?'}</span>
									</td>
									<td class="digits">{r.p}</td>
									<td class="digits">{r.gf - r.ga > 0 ? '+' : ''}{r.gf - r.ga}</td>
									<td class="digits pts">{r.pts}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</section>
			{/each}
		</div>
	{/if}
{:else}
	<div class="stagger">
		{#each bracket as col (col.stage)}
			<h3 class="rname">{stageName[col.stage]}</h3>
			{#each col.matches as m (m.id)}
				{@const H = tn(m.homeTeam)}
				{@const A = tn(m.awayTeam)}
				{@const done = played(m)}
				<div class="bm card">
					<div class="side" class:won={done && m.advancer === m.homeTeam}>
						{#if H}<Flag iso2={H.iso2} code={H.fifaCode} />{/if}
						<span class="nm" class:ph={!H}>{H?.name ?? m.homeLabel}</span>
					</div>
					<div class="mid digits">
						{#if done}{scoreText(m)}{:else}<span class="vs">vs</span>{/if}
					</div>
					<div class="side right" class:won={done && m.advancer === m.awayTeam}>
						<span class="nm" class:ph={!A}>{A?.name ?? m.awayLabel}</span>
						{#if A}<Flag iso2={A.iso2} code={A.fifaCode} />{/if}
					</div>
				</div>
			{/each}
		{/each}
	</div>
{/if}

<style>
	.gwrap {
		display: grid;
		gap: 0.85rem;
	}
	@media (min-width: 760px) {
		.gwrap {
			grid-template-columns: 1fr 1fr;
		}
	}
	.ghead {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		font-size: 0.85rem;
		margin-bottom: 0.6rem;
	}
	.gl {
		display: grid;
		place-items: center;
		width: 26px;
		height: 26px;
		border-radius: 7px;
		background: var(--accent);
		color: var(--accent-fg);
		font-family: var(--font-display);
		font-size: 0.95rem;
	}
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th {
		text-align: right;
		font-size: 0.66rem;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--muted);
		padding: 0 0.4rem 0.4rem;
	}
	th:nth-child(2) {
		text-align: left;
	}
	td {
		padding: 0.45rem 0.4rem;
		border-top: 1px solid var(--border);
		text-align: right;
	}
	.rk {
		width: 1.5rem;
		color: var(--muted);
		text-align: center;
	}
	.tm {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		text-align: left;
		font-weight: 600;
	}
	.pts {
		color: var(--accent);
		font-weight: 700;
	}
	tr.adv .rk {
		color: var(--accent);
		font-weight: 800;
	}
	tr.adv td {
		background: color-mix(in srgb, var(--accent) 7%, transparent);
	}
	tr.third .rk {
		color: var(--warning);
	}
	.rname {
		font-family: var(--font-display);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--muted);
		margin: 1.4rem 0 0.6rem;
	}
	.bm {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.7rem 0.9rem;
	}
	.bm + .bm {
		margin-top: 0.5rem;
	}
	.side {
		flex: 1;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		min-width: 0;
	}
	.side.right {
		justify-content: flex-end;
	}
	.nm {
		font-weight: 700;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.nm.ph {
		color: var(--muted);
		font-weight: 500;
	}
	.side.won .nm {
		color: var(--accent);
	}
	.mid {
		min-width: 4.5rem;
		text-align: center;
		font-size: 0.95rem;
		color: var(--text);
	}
	.vs {
		color: var(--muted);
		font-family: var(--font);
		font-size: 0.8rem;
	}
	.empty {
		text-align: center;
		padding: 2.5rem 1rem;
	}
</style>
