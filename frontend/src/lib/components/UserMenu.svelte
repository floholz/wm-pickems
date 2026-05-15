<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import Avatar from './Avatar.svelte';
	import { LogOut, ChevronDown } from '@lucide/svelte';

	let {
		align = 'right' as 'right' | 'left',
		up = false
	}: { align?: 'right' | 'left'; up?: boolean } = $props();
	let open = $state(false);
	let root: HTMLElement;

	function onDocClick(e: MouseEvent) {
		if (root && !root.contains(e.target as Node)) open = false;
	}
	$effect(() => {
		document.addEventListener('click', onDocClick);
		return () => document.removeEventListener('click', onDocClick);
	});
</script>

<div class="um" bind:this={root}>
	<button
		class="trigger"
		onclick={() => (open = !open)}
		aria-haspopup="menu"
		aria-expanded={open}
	>
		<Avatar name={auth.user?.name ?? '?'} src={auth.user?.avatarUrl} size={36} />
		<ChevronDown size={16} class="chev {open ? 'up' : ''}" />
	</button>

	{#if open}
		<div
			class="panel"
			class:left={align === 'left'}
			class:up
			role="menu"
		>
			<div class="who">
				<Avatar name={auth.user?.name ?? '?'} src={auth.user?.avatarUrl} size={40} />
				<div class="meta">
					<div class="name">{auth.user?.name}</div>
					<div class="email">{auth.user?.email}</div>
				</div>
			</div>
			<!-- "Connect Google" goes here once OAuth is wired. -->
			<button class="item" onclick={() => auth.logout()}>
				<LogOut size={17} /> Log out
			</button>
		</div>
	{/if}
</div>

<style>
	.um {
		position: relative;
	}
	.trigger {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		background: none;
		border: none;
		padding: 0;
		color: var(--muted);
	}
	:global(.um .chev) {
		transition: transform 0.15s ease;
	}
	:global(.um .chev.up) {
		transform: rotate(180deg);
	}
	.panel {
		position: absolute;
		top: calc(100% + 0.5rem);
		right: 0;
		min-width: 220px;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		padding: 0.5rem;
		box-shadow: var(--shadow-pop);
		z-index: 60;
	}
	.panel.left {
		right: auto;
		left: 0;
	}
	.panel.up {
		top: auto;
		bottom: calc(100% + 0.5rem);
	}
	.who {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.5rem 0.5rem 0.7rem;
		border-bottom: 1px solid var(--border);
		margin-bottom: 0.4rem;
	}
	.name {
		font-weight: 700;
	}
	.email {
		font-size: 0.8rem;
		color: var(--muted);
	}
	.item {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		width: 100%;
		padding: 0.6rem 0.55rem;
		background: none;
		border: none;
		border-radius: var(--radius-sm);
		color: var(--text);
		font: inherit;
		text-align: left;
	}
	.item:hover {
		background: var(--surface);
	}
</style>
