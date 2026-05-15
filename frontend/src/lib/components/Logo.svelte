<script lang="ts">
	let { compact = false }: { compact?: boolean } = $props();
</script>

<a class="logo" href="/" aria-label="WM Tips home">
	<img class="mark" src="/logos/fhun_logo.svg" alt="fhun - logo" />
	{#if !compact}<span class="word">WM&nbsp;Tips</span>{/if}
</a>

<style>
	.logo {
		display: inline-flex;
		align-items: center;
		gap: 0.6rem;
		color: var(--text);
		font-weight: 800;
		letter-spacing: -0.02em;
	}
	.mark {
		height: 30px;
		width: auto;
		display: block;
	}
	.word {
		font-family: var(--font-display);
		font-size: 1.2rem;
		letter-spacing: 0.01em;
		text-transform: uppercase;
	}
</style>
