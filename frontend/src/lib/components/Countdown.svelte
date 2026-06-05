<script lang="ts">
	import { countdown } from '$lib/countdown.svelte';
	import { Lock } from '@lucide/svelte';

	let { variant = 'hero' }: { variant?: 'hero' | 'cta' | 'bar' } = $props();
	const pad = (n: number) => String(n).padStart(2, '0');
</script>

{#if countdown.ready && !countdown.locked && countdown.kickoff !== null}
	{@const p = countdown.parts}
	{#if variant === 'bar'}
		<div class="cd-bar">
			<Lock size={13} />
			<span class="lbl">Forecast &amp; first match lock in</span>
			<b class="digits">{p.days}d {pad(p.hours)}h {pad(p.mins)}m {pad(p.secs)}s</b>
		</div>
	{:else}
		<div class="cd cd-{variant}">
			<span class="kick"><Lock size={11} /> Locks in</span>
			<div class="units digits">
				<span class="u"><b>{pad(p.days)}</b><i>days</i></span>
				<span class="sep">:</span>
				<span class="u"><b>{pad(p.hours)}</b><i>hrs</i></span>
				<span class="sep">:</span>
				<span class="u"><b>{pad(p.mins)}</b><i>min</i></span>
				<span class="sep">:</span>
				<span class="u"><b>{pad(p.secs)}</b><i>sec</i></span>
			</div>
		</div>
	{/if}
{/if}

<style>
	/* ---------- block variants (hero + cta) ---------- */
	.cd {
		display: inline-flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 0.85rem 1.1rem;
		border: 1px solid var(--border);
		border-radius: var(--radius);
		background: var(--surface-2);
	}
	.cd-cta {
		align-items: center;
	}
	.kick {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		font-size: 0.66rem;
		font-weight: 700;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--accent);
	}
	.units {
		display: flex;
		align-items: baseline;
		gap: 0.55rem;
	}
	.u {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.15rem;
		min-width: 2ch;
	}
	.u b {
		font-size: clamp(1.5rem, 5vw, 2rem);
		font-weight: 700;
		line-height: 1;
		color: var(--text);
		font-variant-numeric: tabular-nums;
	}
	.u i {
		font-style: normal;
		font-size: 0.58rem;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--muted);
	}
	.sep {
		font-size: clamp(1.3rem, 4.5vw, 1.7rem);
		font-weight: 700;
		line-height: 1;
		color: var(--border);
		transform: translateY(-0.35rem);
	}

	/* ---------- sticky bar variant ---------- */
	.cd-bar {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.55rem;
		padding: 0.5rem 1rem;
		background: linear-gradient(90deg, var(--accent) 0%, var(--accent-2) 100%);
		color: var(--accent-ink);
		font-size: 0.85rem;
		font-weight: 700;
	}
	.cd-bar .lbl {
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}
	.cd-bar b {
		font-variant-numeric: tabular-nums;
		letter-spacing: 0.02em;
	}
	@media (max-width: 520px) {
		.cd-bar .lbl {
			display: none;
		}
	}
</style>
