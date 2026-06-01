<script lang="ts">
	import { tipsStore, type Match } from '$lib/tips.svelte';
	import Flag from './Flag.svelte';
	import { ChevronDown } from '@lucide/svelte';

	// A projected group table: played matches count as-is, the rest use the
	// user's saved picks. Collapsed by default; updates live as tips are saved.
	let { matches }: { matches: Match[] } = $props();
	let open = $state(false);

	type Row = { id: string; pts: number; gf: number; ga: number; pld: number };

	let rows = $derived.by(() => {
		const tbl: Record<string, Row> = {};
		const ensure = (id: string) =>
			(tbl[id] ||= { id, pts: 0, gf: 0, ga: 0, pld: 0 });
		// Seed all four teams so the group shows in full, then fill it in.
		for (const m of matches) {
			if (m.homeTeam) ensure(m.homeTeam);
			if (m.awayTeam) ensure(m.awayTeam);
		}
		for (const m of matches) {
			if (!m.homeTeam || !m.awayTeam) continue;
			const played = m.status === 'finished' || !!m.finalizedAt;
			let h: number, a: number;
			if (played) {
				h = m.ftHome;
				a = m.ftAway;
			} else {
				const tip = tipsStore.tips[m.id];
				if (!tip) continue; // not played, not tipped → no contribution yet
				h = tip.ftHome;
				a = tip.ftAway;
			}
			const H = ensure(m.homeTeam);
			const A = ensure(m.awayTeam);
			H.pld++;
			A.pld++;
			H.gf += h;
			H.ga += a;
			A.gf += a;
			A.ga += h;
			if (h > a) H.pts += 3;
			else if (a > h) A.pts += 3;
			else {
				H.pts++;
				A.pts++;
			}
		}
		const name = (id: string) => tipsStore.team(id)?.name ?? '';
		return Object.values(tbl).sort(
			(x, y) =>
				y.pts - x.pts ||
				y.gf - y.ga - (x.gf - x.ga) ||
				y.gf - x.gf ||
				name(x.id).localeCompare(name(y.id))
		);
	});

	let counted = $derived(rows.reduce((n, r) => n + r.pld, 0));
	const gd = (r: Row) => `${r.gf - r.ga >= 0 ? '+' : ''}${r.gf - r.ga}`;
</script>

<div class="gs">
	<button class="gs-toggle" onclick={() => (open = !open)} aria-expanded={open}>
		<span>Projected table</span>
		<ChevronDown size={15} class="gs-cv {open ? 'up' : ''}" />
	</button>

	{#if open}
		{#if counted === 0}
			<p class="muted small note">
				Tip this group’s matches to see the projected standings.
			</p>
		{:else}
			<table class="gs-tbl">
				<thead>
					<tr>
						<th></th>
						<th class="tl">Team</th>
						<th>P</th>
						<th>GD</th>
						<th>Pts</th>
					</tr>
				</thead>
				<tbody>
					{#each rows as r, i (r.id)}
						<tr class:adv={i < 2}>
							<td class="rk">{i + 1}</td>
							<td class="tl">
								<span class="tm">
									<Flag
										iso2={tipsStore.team(r.id)?.iso2 ?? ''}
										code={tipsStore.team(r.id)?.fifaCode ?? ''}
									/>
									<span class="nm">{tipsStore.team(r.id)?.name ?? r.id}</span>
								</span>
							</td>
							<td>{r.pld}</td>
							<td>{gd(r)}</td>
							<td class="pts">{r.pts}</td>
						</tr>
					{/each}
				</tbody>
			</table>
			<p class="muted small note">
				Your picks, with played results counted. Top 2 advance.
			</p>
		{/if}
	{/if}
</div>

<style>
	.gs {
		margin: 0.5rem 0 0.2rem;
	}
	.gs-toggle {
		width: 100%;
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.4rem;
		padding: 0.45rem;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--muted);
		font-weight: 600;
		font-size: 0.82rem;
	}
	:global(.gs .gs-cv) {
		transition: transform 0.15s ease;
	}
	:global(.gs .gs-cv.up) {
		transform: rotate(180deg);
	}
	.gs-tbl {
		width: 100%;
		border-collapse: collapse;
		margin-top: 0.4rem;
		font-size: 0.85rem;
	}
	.gs-tbl th {
		font-size: 0.7rem;
		font-weight: 700;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--muted);
		text-align: center;
		padding: 0.25rem 0.4rem;
	}
	.gs-tbl td {
		text-align: center;
		padding: 0.4rem;
		border-top: 1px solid var(--border);
	}
	.gs-tbl .tl {
		text-align: left;
	}
	.tm {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		min-width: 0;
	}
	.nm {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-weight: 600;
	}
	.rk {
		color: var(--muted);
		font-variant-numeric: tabular-nums;
	}
	.pts {
		font-weight: 800;
	}
	tr.adv .rk {
		color: var(--accent);
		font-weight: 800;
	}
	tr.adv td {
		background: color-mix(in srgb, var(--accent) 8%, transparent);
	}
	.note {
		margin: 0.5rem 0 0;
		text-align: center;
	}
	.small {
		font-size: 0.78rem;
	}
</style>
