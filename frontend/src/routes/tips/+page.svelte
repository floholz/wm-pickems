<script lang="ts">
	import { tipsStore, isLocked, type Match } from '$lib/tips.svelte';
	import TipCard from '$lib/components/TipCard.svelte';

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

<p class="kicker">Match predictions</p>
<h1>Tips</h1>
<p class="muted">Predict every match. Editable until kickoff.</p>

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
	<button class:active={tab === 'all'} onclick={() => (tab = 'all')}>All</button>
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
			<TipCard match={m} />
		{/each}
	{/each}
{/if}

<style>
	h1 {
		margin: 0.25rem 0 0.2rem;
	}
	.muted {
		margin: 0 0 1rem;
	}
	.tabs {
		display: flex;
		gap: 0.4rem;
		margin-bottom: 1rem;
		position: sticky;
		top: calc(var(--topbar-h));
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
</style>
