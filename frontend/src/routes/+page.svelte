<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { api, type LeagueSummary } from '$lib/api';
	import { Telescope, Volleyball, Trophy, Users, ChevronRight } from '@lucide/svelte';
	import Landing from '$lib/components/Landing.svelte';

	let leagues = $state<LeagueSummary[]>([]);
	let loaded = $state(false);

	$effect(() => {
		if (!auth.isAuthed) return;
		api
			.myLeagues()
			.then((r) => (leagues = r.leagues))
			.catch(() => {})
			.finally(() => (loaded = true));
	});

	const moves = [
		{
			href: '/forecast',
			icon: Telescope,
			title: 'Fill in your Forecast',
			sub: 'Full tournament call — before the opening match'
		},
		{
			href: '/tips',
			icon: Volleyball,
			title: 'Tip the upcoming matches',
			sub: 'Score predictions, editable until kickoff'
		},
		{
			href: '/leagues',
			icon: Trophy,
			title: 'Create or join a League',
			sub: 'Play against your friends'
		}
	];
</script>

{#if !auth.isAuthed}
	<Landing />
{:else}
<header>
	<p class="kicker">Matchday HQ</p>
	<h1>Hi,&nbsp;{auth.user?.name}</h1>
	<p class="muted sd">World Cup 2026 · 11 Jun – 19 Jul · 48 nations</p>
</header>

<div class="stagger">
<section class="card">
	<h3>Your next moves</h3>
	<div class="moves">
		{#each moves as m (m.href)}
			{@const Icon = m.icon}
			<a class="move" href={m.href}>
				<span class="mi"><Icon size={20} /></span>
				<span class="mt">
					<span class="title">{m.title}</span>
					<span class="muted sub">{m.sub}</span>
				</span>
				<ChevronRight size={18} class="cr" />
			</a>
		{/each}
	</div>
</section>

<section class="card">
	<div class="row">
		<h3>Your leagues</h3>
		<div class="spacer"></div>
		<a class="pill" href="/leagues">Manage</a>
	</div>
	{#if !loaded}
		<p class="muted">Loading…</p>
	{:else if leagues.length === 0}
		<p class="muted">
			You're not in a league yet. <a href="/leagues">Create or join one →</a>
		</p>
	{:else}
		{#each leagues as l (l.id)}
			<a class="lrow" href={`/leagues/${l.id}`}>
				<span>{l.name}</span>
				{#if l.role === 'owner'}<span class="pill">owner</span>{/if}
				<span class="spacer"></span>
				<span class="cnt"><Users size={15} /> {l.members}</span>
			</a>
		{/each}
	{/if}
</section>
</div>
{/if}

<style>
	header {
		margin: 0.25rem 0 1.25rem;
	}
	h1 {
		margin: 0;
		font-size: 1.6rem;
	}
	header .muted {
		margin: 0.2rem 0 0;
	}
	.moves {
		margin-top: 0.6rem;
	}
	.move {
		display: flex;
		align-items: center;
		gap: 0.85rem;
		padding: 0.75rem 0;
		border-top: 1px solid var(--border);
		color: var(--text);
	}
	.move:first-child {
		border-top: none;
	}
	.mi {
		display: grid;
		place-items: center;
		width: 38px;
		height: 38px;
		border-radius: var(--radius-sm);
		background: var(--surface-2);
		color: var(--accent);
		flex: none;
	}
	.mt {
		display: flex;
		flex-direction: column;
	}
	.title {
		font-weight: 600;
	}
	.sub {
		font-size: 0.82rem;
	}
	:global(.move .cr) {
		margin-left: auto;
		color: var(--muted);
	}
	.lrow {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.7rem 0;
		border-top: 1px solid var(--border);
		color: var(--text);
	}
	.lrow:first-of-type {
		border-top: none;
	}
	.cnt {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		color: var(--muted);
		font-size: 0.9rem;
	}
</style>
