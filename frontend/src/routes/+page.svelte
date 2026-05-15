<script lang="ts">
	import { pb } from '$lib/pb';

	let health = $state<'checking' | 'ok' | 'down'>('checking');

	$effect(() => {
		pb.health
			.check()
			.then(() => (health = 'ok'))
			.catch(() => (health = 'down'));
	});
</script>

<header>
	<h1>WM Pickems</h1>
	<p class="tagline">Predict the World Cup. Beat your friends.</p>
</header>

<section class="card">
	<p>
		Backend:
		{#if health === 'checking'}
			<span class="muted">checking…</span>
		{:else if health === 'ok'}
			<span class="ok">connected</span>
		{:else}
			<span class="down">unreachable</span>
		{/if}
	</p>
	<p class="muted">
		Scaffold is live. Auth, Leagues, Tips, Forecast and the scoring engine
		land in the next phases.
	</p>
</section>

<style>
	header {
		margin: 2rem 0;
	}
	h1 {
		margin: 0;
		font-size: 2rem;
		letter-spacing: -0.02em;
	}
	.tagline {
		color: var(--muted);
		margin: 0.25rem 0 0;
	}
	.card {
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		padding: 1.25rem;
	}
	.muted {
		color: var(--muted);
	}
	.ok {
		color: var(--accent-2);
	}
	.down {
		color: var(--danger);
	}
</style>
