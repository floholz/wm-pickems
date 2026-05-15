<script lang="ts">
	import { api, type LeagueSummary } from '$lib/api';
	import { goto } from '$app/navigation';
	import { Users } from '@lucide/svelte';

	let leagues = $state<LeagueSummary[]>([]);
	let loaded = $state(false);
	let newName = $state('');
	let joinCode = $state('');
	let error = $state('');
	let busy = $state(false);

	async function load() {
		try {
			leagues = (await api.myLeagues()).leagues;
		} catch {
			/* ignore */
		} finally {
			loaded = true;
		}
	}
	$effect(() => {
		load();
	});

	async function create(e: Event) {
		e.preventDefault();
		error = '';
		busy = true;
		try {
			const r = await api.createLeague(newName);
			newName = '';
			goto(`/leagues/${r.id}`);
		} catch {
			error = 'Could not create league.';
		} finally {
			busy = false;
		}
	}

	async function join(e: Event) {
		e.preventDefault();
		error = '';
		busy = true;
		try {
			const r = await api.joinLeague(joinCode);
			joinCode = '';
			goto(`/leagues/${r.id}`);
		} catch {
			error = 'Invalid invite code.';
		} finally {
			busy = false;
		}
	}
</script>

<p class="kicker">Play your friends</p>
<h1>Leagues</h1>
<p class="muted">Private competitions — your predictions vs. your friends'.</p>

<section class="card">
	<h3>Your leagues</h3>
	{#if !loaded}
		<p class="muted">Loading…</p>
	{:else if leagues.length === 0}
		<p class="muted">None yet — create one or join with a code.</p>
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

<section class="card">
	<h3>Create a league</h3>
	<form onsubmit={create}>
		<div class="field">
			<input class="input" placeholder="League name" bind:value={newName} required />
		</div>
		<button class="btn" disabled={busy || !newName.trim()}>Create</button>
	</form>
</section>

<section class="card">
	<h3>Join a league</h3>
	<form onsubmit={join}>
		<div class="field">
			<input
				class="input code"
				placeholder="INVITE CODE"
				bind:value={joinCode}
				required
			/>
		</div>
		<button class="btn secondary" disabled={busy || !joinCode.trim()}>Join</button>
	</form>
</section>

{#if error}<p class="error">{error}</p>{/if}

<style>
	h1 {
		margin: 1rem 0 0.2rem;
	}
	.muted {
		margin: 0 0 1rem;
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
	.code {
		text-transform: uppercase;
		letter-spacing: 0.2em;
		font-weight: 700;
	}
</style>
