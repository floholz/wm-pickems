<script lang="ts">
	import { tipsStore, type Match } from '$lib/tips.svelte';
	import TipCard from '$lib/components/TipCard.svelte';
	import GroupStandings from '$lib/components/GroupStandings.svelte';
	import { collapseOnScroll } from '$lib/actions';
	import { serverClock } from '$lib/serverclock.svelte';
	import { LocateFixed } from '@lucide/svelte';
	import { tick } from 'svelte';

	let tab = $state<'all' | 'group' | 'ko'>('all');

	// Accordion: only one match's tip inputs are open at a time.
	let openId = $state('');

	$effect(() => {
		if (!tipsStore.loaded) tipsStore.load().catch(() => {});
	});

	let filtered = $derived(
		tipsStore.matches.filter((m) => {
			if (tab === 'group') return m.stage === 'group';
			if (tab === 'ko') return m.stage !== 'group';
			return true;
		})
	);

	// "Now" = the next match not yet kicked off (or the last one if the
	// tournament is over) within the current filter.
	let nowId = $derived.by(() => {
		const now = serverClock.now();
		const next = filtered.find(
			(m) => new Date(m.kickoff).getTime() >= now
		);
		return (next ?? filtered[filtered.length - 1])?.id ?? '';
	});

	function goNow() {
		// Scroll to the day-header of the day holding the "now" match —
		// nicer context, and days hold only a handful of games.
		document
			.getElementById(`day-${nowDayIndex}`)
			?.scrollIntoView({ behavior: 'smooth', block: 'start' });
	}

	// Groups tab: by group letter (A..L). Knockout tab: by stage (R32→FINAL).
	// All tab: by calendar day.
	const stageOrder = ['R32', 'R16', 'QF', 'SF', '3RD', 'FINAL'];
	const stageLabel: Record<string, string> = {
		R32: 'Round of 32',
		R16: 'Round of 16',
		QF: 'Quarter-finals',
		SF: 'Semi-finals',
		'3RD': 'Third place',
		FINAL: 'Final'
	};
	let days = $derived.by(() => {
		const byKickoff = (a: Match, b: Match) =>
			new Date(a.kickoff).getTime() - new Date(b.kickoff).getTime();
		if (tab === 'group') {
			const byGroup: Record<string, Match[]> = {};
			for (const m of filtered) (byGroup[m.groupLetter] ||= []).push(m);
			return Object.keys(byGroup)
				.sort()
				.map(
					(l) =>
						[`Group ${l}`, byGroup[l].sort(byKickoff)] as [string, Match[]]
				);
		}
		if (tab === 'ko') {
			const byStage: Record<string, Match[]> = {};
			for (const m of filtered) (byStage[m.stage] ||= []).push(m);
			return stageOrder
				.filter((s) => byStage[s])
				.map(
					(s) =>
						[stageLabel[s] ?? s, byStage[s].sort(byKickoff)] as [
							string,
							Match[]
						]
				);
		}
		return Object.entries(
			filtered.reduce<Record<string, Match[]>>((acc, m) => {
				const d = new Date(m.kickoff).toLocaleDateString(undefined, {
					weekday: 'long',
					day: 'numeric',
					month: 'long'
				});
				(acc[d] ||= []).push(m);
				return acc;
			}, {})
		);
	});

	let nowDayIndex = $derived(
		days.findIndex(([, ms]) => ms.some((m) => m.id === nowId))
	);

	// On first load, instantly jump to the current point in the tournament.
	let didAutoScroll = false;
	$effect(() => {
		if (didAutoScroll || !tipsStore.loaded) return;
		const idx = nowDayIndex;
		if (idx < 0) return;
		didAutoScroll = true;
		// First matchday: stay at the very top (full header). Otherwise jump
		// to that day's header.
		if (idx === 0) return;
		tick().then(() =>
			document
				.getElementById(`day-${idx}`)
				?.scrollIntoView({ block: 'start' })
		);
	});
</script>

<div class="stickyhead" use:collapseOnScroll>
	<p class="kicker">Match predictions</p>
	<div class="sh-expand">
		<div class="sh-inner">
			<h1>Tips</h1>
			<p class="muted desc">Predict every match. Editable until kickoff.</p>
		</div>
	</div>
	<div class="tabs">
		<button class:active={tab === 'all'} onclick={() => (tab = 'all')}>All</button>
		<button class:active={tab === 'group'} onclick={() => (tab = 'group')}
			>Groups</button
		>
		<button class:active={tab === 'ko'} onclick={() => (tab = 'ko')}
			>Knockout</button
		>
	</div>
</div>

{#if !tipsStore.loaded}
	<p class="muted">Loading fixtures…</p>
{:else if filtered.length === 0}
	<p class="muted">Nothing here.</p>
{:else}
	{#each days as [day, ms], i (day)}
		<h3 class="day" id={`day-${i}`}>{day}</h3>
		{#each ms as m (m.id)}
			<div class="match">
				<TipCard
					match={m}
					open={openId === m.id}
					onToggle={() => (openId = openId === m.id ? '' : m.id)}
				/>
			</div>
		{/each}
		{#if tab === 'group'}
			<GroupStandings matches={ms} />
		{/if}
	{/each}
	<div class="fabpad"></div>
{/if}

{#if tipsStore.loaded && nowId}
	<button class="fab" onclick={goNow} aria-label="Scroll to the next match">
		<LocateFixed size={18} /> Now
	</button>
{/if}

<style>
	.stickyhead {
		position: sticky;
		top: var(--topbar-h);
		z-index: 20;
		margin: 0 -1rem;
		padding: 0.6rem 1rem 0.75rem;
		background: color-mix(in srgb, var(--bg) 86%, transparent);
		backdrop-filter: blur(12px) saturate(1.3);
		border-bottom: 1px solid var(--border);
	}
	.stickyhead h1 {
		margin: 0.1rem 0 0;
	}
	.stickyhead .desc {
		margin: 0.3rem 0 0;
		font-size: 0.9rem;
	}
	@media (min-width: 900px) {
		.stickyhead {
			top: 0;
			margin: 0 -2rem;
			padding: 0.75rem 2rem 0.85rem;
		}
	}
	.tabs {
		display: flex;
		gap: 0.4rem;
		margin: 0.75rem 0 0;
		z-index: 10;
	}
	.tabs button {
		flex: 1;
		padding: 0.5rem;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--muted);
		font-weight: 600;
		font-size: 0.85rem;
	}
	.tabs button.active {
		color: var(--accent-fg);
		background: var(--accent);
		border-color: var(--accent);
	}
	.day {
		margin: 1.3rem 0 0.6rem;
		font-size: 0.95rem;
		color: var(--muted);
		/* Land below the fixed top bar + collapsed sticky header. */
		scroll-margin-top: 150px;
	}
	@media (min-width: 900px) {
		.day {
			scroll-margin-top: 96px;
		}
	}
	.match + .match {
		margin-top: 6px;
	}
	.fabpad {
		height: 4rem;
	}
	.fab {
		position: fixed;
		right: 1rem;
		bottom: calc(var(--nav-h) + 1rem);
		z-index: 40;
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.7rem 1rem;
		border: none;
		border-radius: var(--radius-pill);
		background: var(--accent);
		color: var(--accent-fg);
		font:
			800 0.8rem var(--font);
		letter-spacing: 0.06em;
		text-transform: uppercase;
		cursor: pointer;
		box-shadow: var(--shadow-pop);
		transition:
			transform 0.12s ease,
			box-shadow 0.2s ease;
	}
	.fab:hover {
		transform: translateY(-2px);
		box-shadow: var(--glow);
	}
	@media (min-width: 900px) {
		.fab {
			bottom: 1.5rem;
			right: 1.5rem;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.fab {
			transition: none;
		}
	}
</style>
