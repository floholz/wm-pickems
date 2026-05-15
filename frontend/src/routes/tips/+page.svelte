<script lang="ts">
	import { tipsStore, isLocked, type Match } from '$lib/tips.svelte';
	import TipCard from '$lib/components/TipCard.svelte';
	import { collapseOnScroll } from '$lib/actions';
	import { LocateFixed } from '@lucide/svelte';

	let tab = $state<'upcoming' | 'group' | 'ko' | 'all'>('upcoming');

	$effect(() => {
		if (!tipsStore.loaded) tipsStore.load().catch(() => {});
	});

	let filtered = $derived(
		tipsStore.matches.filter((m) => {
			if (tab === 'upcoming') return !isLocked(m);
			if (tab === 'group') return m.stage === 'group';
			if (tab === 'ko') return m.stage !== 'group';
			return true;
		})
	);

	// "Now" = the next match not yet kicked off (or the last one if the
	// tournament is over) within the current filter.
	let nowId = $derived.by(() => {
		const now = Date.now();
		const next = filtered.find(
			(m) => new Date(m.kickoff).getTime() >= now
		);
		return (next ?? filtered[filtered.length - 1])?.id ?? '';
	});

	function goNow() {
		document
			.getElementById(`m-${nowId}`)
			?.scrollIntoView({ behavior: 'smooth', block: 'start' });
	}

	// Group by calendar day for readable scanning.
	let days = $derived(
		Object.entries(
			filtered.reduce<Record<string, Match[]>>((acc, m) => {
				const d = new Date(m.kickoff).toLocaleDateString(undefined, {
					weekday: 'long',
					day: 'numeric',
					month: 'long'
				});
				(acc[d] ||= []).push(m);
				return acc;
			}, {})
		)
	);
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
		<button class:active={tab === 'upcoming'} onclick={() => (tab = 'upcoming')}
			>Upcoming</button
		>
		<button class:active={tab === 'group'} onclick={() => (tab = 'group')}
			>Groups</button
		>
		<button class:active={tab === 'ko'} onclick={() => (tab = 'ko')}
			>Knockout</button
		>
		<button class:active={tab === 'all'} onclick={() => (tab = 'all')}>All</button
		>
	</div>
</div>

{#if !tipsStore.loaded}
	<p class="muted">Loading fixtures…</p>
{:else if filtered.length === 0}
	<p class="muted">
		{tab === 'upcoming'
			? 'No upcoming matches open for tipping.'
			: 'Nothing here.'}
	</p>
{:else}
	{#each days as [day, ms] (day)}
		<h3 class="day">{day}</h3>
		{#each ms as m (m.id)}
			<div id={`m-${m.id}`} class="anchor"><TipCard match={m} /></div>
		{/each}
	{/each}
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
	}
	/* Land below the fixed top bar + collapsed sticky header. */
	.anchor {
		scroll-margin-top: 165px;
	}
	@media (min-width: 900px) {
		.anchor {
			scroll-margin-top: 110px;
		}
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
