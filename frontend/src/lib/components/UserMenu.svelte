<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { serverClock } from '$lib/serverclock.svelte';
	import Avatar from './Avatar.svelte';
	import {
		LogOut,
		ChevronDown,
		FlaskConical,
		Settings,
		Shield,
		Crown,
		Megaphone
	} from '@lucide/svelte';

	let {
		align = 'right' as 'right' | 'left',
		up = false,
		showName = false
	}: { align?: 'right' | 'left'; up?: boolean; showName?: boolean } =
		$props();
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
		{#if showName}<span class="tname">{auth.user?.name}</span>{/if}
		<ChevronDown size={16} class="chev {open ? 'up' : ''}" />
	</button>

	{#if open}
		<div
			class="panel"
			class:left={align === 'left'}
			class:up
			role="menu"
		>
			<a class="who" href="/settings" onclick={() => (open = false)}>
				<Avatar name={auth.user?.name ?? '?'} src={auth.user?.avatarUrl} size={40} />
				<div class="meta">
					<div class="name">{auth.user?.name}</div>
					<div class="email">{auth.user?.email}</div>
				</div>
			</a>
			<a class="item" href="/settings" onclick={() => (open = false)}>
				<Settings size={17} /> Settings
			</a>
			{#if serverClock.dev}
				<a class="item" href="/dev" onclick={() => (open = false)}>
					<FlaskConical size={17} /> Dev tools
				</a>
			{/if}
			{#if auth.isAdmin}
				<a class="item" href="/admin" onclick={() => (open = false)}>
					<Megaphone size={17} /> Announcements
				</a>
			{/if}
			{#if auth.isOwner}
				<a class="item" href="/owner" onclick={() => (open = false)}>
					<Crown size={17} /> Owner stats
				</a>
			{/if}
			{#if auth.isAdmin}
				<a
					class="item"
					href="/_/"
					target="_blank"
					rel="noopener"
					onclick={() => (open = false)}
				>
					<Shield size={17} /> PB Dashboard
				</a>
			{/if}
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
		gap: 0.5rem;
		width: 100%;
		background: none;
		border: none;
		padding: 0;
		color: var(--muted);
	}
	.tname {
		flex: 1;
		min-width: 0;
		text-align: left;
		font-weight: 700;
		color: var(--text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
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
		color: var(--text);
		border-radius: var(--radius-sm) var(--radius-sm) 0 0;
	}
	.who:hover {
		background: var(--surface);
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
