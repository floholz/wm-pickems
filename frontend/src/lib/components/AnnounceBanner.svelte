<script lang="ts">
	// In-app announcement banners written by an admin in the Admin console.
	// Two distinct kinds, rendered separately:
	//
	//  - persistent: a fixed (sticky) ribbon pinned below the header so it never
	//    scrolls away. The ribbon is the header; expanding grows a body out of it
	//    sharing the same background + slim border (no card switch). Can't be
	//    dismissed — only collapsed/expanded (remembered per-id).
	//
	//  - dismissible (default): a normal in-flow card; the X removes it for good
	//    (per-id in localStorage). A new announcement (new id) always shows.
	import { onMount } from 'svelte';
	import { api, type Announcement } from '$lib/api';
	import {
		Megaphone,
		Info,
		TriangleAlert,
		X,
		ChevronUp,
		ChevronDown
	} from '@lucide/svelte';

	const DISMISS_KEY = 'announce-dismissed-v1';
	const COLLAPSE_KEY = 'announce-collapsed-v1';

	let items = $state<Announcement[]>([]);
	let dismissed = $state<Set<string>>(new Set());
	let collapsed = $state<Set<string>>(new Set());

	function loadSet(key: string): Set<string> {
		try {
			const raw = localStorage.getItem(key);
			if (raw) return new Set(JSON.parse(raw) as string[]);
		} catch {
			/* private mode / bad json — treat as empty */
		}
		return new Set();
	}

	function persist(key: string, set: Set<string>) {
		try {
			localStorage.setItem(key, JSON.stringify([...set]));
		} catch {
			/* ignore */
		}
	}

	function dismiss(id: string) {
		dismissed = new Set(dismissed).add(id);
		persist(DISMISS_KEY, dismissed);
	}

	function toggleCollapse(id: string) {
		const next = new Set(collapsed);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		collapsed = next;
		persist(COLLAPSE_KEY, collapsed);
	}

	// Drop remembered ids that are no longer live so the sets don't grow forever.
	function prune(key: string, set: Set<string>, live: Set<string>): Set<string> {
		if (![...set].some((id) => !live.has(id))) return set;
		const next = new Set([...set].filter((id) => live.has(id)));
		persist(key, next);
		return next;
	}

	onMount(async () => {
		dismissed = loadSet(DISMISS_KEY);
		collapsed = loadSet(COLLAPSE_KEY);
		try {
			const res = await api.activeAnnouncements();
			items = res.announcements;
			const liveIds = new Set(items.map((a) => a.id));
			dismissed = prune(DISMISS_KEY, dismissed, liveIds);
			collapsed = prune(COLLAPSE_KEY, collapsed, liveIds);
		} catch {
			/* not signed in / offline — show nothing */
		}
	});

	let pinned = $derived(items.filter((a) => a.persistent));
	let cards = $derived(items.filter((a) => !a.persistent && !dismissed.has(a.id)));

	const icon = { info: Info, success: Megaphone, warn: TriangleAlert };
</script>

<!-- Persistent: fixed ribbon(s) that stay pinned below the header. -->
{#if pinned.length}
	<div class="pinned">
		{#each pinned as a (a.id)}
			{@const Icon = icon[a.level] ?? Megaphone}
			{@const open = !collapsed.has(a.id)}
			<div class="pa {a.level}" class:open>
				<button
					class="pahead"
					aria-expanded={open}
					title={open ? 'Collapse' : 'Expand'}
					onclick={() => toggleCollapse(a.id)}
				>
					<Icon size={16} />
					<span class="patitle">{a.title}</span>
					{#if open}<ChevronUp size={16} class="pachev" />{:else}<ChevronDown
							size={16}
							class="pachev"
						/>{/if}
				</button>
				{#if open}
					<div class="pabody">{a.body}</div>
				{/if}
			</div>
		{/each}
	</div>
{/if}

<!-- Dismissible: in-flow cards. -->
{#if cards.length}
	<div class="stack">
		{#each cards as a (a.id)}
			{@const Icon = icon[a.level] ?? Megaphone}
			<div class="banner {a.level}" role="status">
				<span class="ico"><Icon size={18} /></span>
				<span class="text">
					<strong class="t">{a.title}</strong>
					<span class="b">{a.body}</span>
				</span>
				<button class="x" aria-label="Dismiss" onclick={() => dismiss(a.id)}>
					<X size={16} />
				</button>
			</div>
		{/each}
	</div>
{/if}

<style>
	/* ============ persistent: sticky ribbon + grown-out body ============ */
	.pinned {
		position: sticky;
		top: var(--topbar-h);
		z-index: 20;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		margin-bottom: 1rem;
	}
	@media (min-width: 900px) {
		.pinned {
			top: 1rem;
		}
	}
	/* One unit: the ribbon fill spans header + body; overflow-clip so the body
	   shares the rounded corners and the fill never bleeds past the border. */
	.pa {
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		overflow: hidden;
		background: var(--surface-2);
		color: var(--text);
	}
	.pahead {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		padding: 0.55rem 0.85rem;
		background: transparent;
		border: none;
		color: inherit;
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
	.patitle {
		flex: 1;
		min-width: 0;
		font-weight: 700;
		font-size: 0.9rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	:global(.pa .pachev) {
		flex-shrink: 0;
		opacity: 0.75;
	}
	/* Body grows out of the ribbon: same background, separated by a slim rule. */
	.pabody {
		padding: 0.6rem 0.85rem 0.7rem;
		border-top: 1px solid var(--border);
		font-size: 0.88rem;
		line-height: 1.55;
		white-space: pre-line;
	}

	/* Info — calm: subtle surface, accent left rule, muted body. */
	.pa.info {
		border-left: 3px solid color-mix(in srgb, var(--muted) 45%, var(--border));
	}
	.pa.info .pabody {
		color: var(--muted);
	}
	/* Highlight — the landing lock-countdown look: lime gradient, dark ink. */
	.pa.success {
		background: linear-gradient(90deg, var(--accent), var(--accent-2));
		color: var(--accent-ink);
		border-color: transparent;
	}
	.pa.success .pabody {
		border-color: rgba(8, 17, 10, 0.18);
	}
	/* Warning — solid amber, dark ink. */
	.pa.warn {
		background: var(--warning);
		color: #20160a;
		border-color: transparent;
	}
	.pa.warn .pabody {
		border-color: rgba(32, 22, 10, 0.22);
	}

	/* On mobile the pinned ribbon spans edge-to-edge — break out of the
	   app-shell's 1rem gutter (safe: the shell is overflow-x: clip), keep 1rem
	   inner padding so text still lines up with page content. */
	@media (max-width: 899px) {
		.pinned {
			margin-inline: -1rem;
		}
		.pa {
			border-radius: 0;
			border-left: none;
			border-right: none;
		}
		.pahead,
		.pabody {
			padding-inline: 1rem;
		}
	}

	/* ============ dismissible cards (unchanged) ============ */
	.stack {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		margin-bottom: 1rem;
	}
	.banner {
		display: grid;
		grid-template-columns: auto 1fr auto;
		align-items: start;
		gap: 0.7rem;
		padding: 0.8rem 0.9rem;
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
	}
	.banner.info {
		border-left: 3px solid color-mix(in srgb, var(--muted) 45%, var(--border));
	}
	.banner.info .ico {
		background: var(--surface-2);
		color: var(--muted);
	}
	.banner.success {
		background: color-mix(in srgb, var(--accent) 13%, var(--surface));
		border-color: color-mix(in srgb, var(--accent) 45%, var(--border));
		border-left: 3px solid var(--accent);
		box-shadow: 0 12px 32px -18px color-mix(in srgb, var(--accent) 70%, transparent);
	}
	.banner.success .ico {
		background: var(--accent);
		color: var(--accent-fg);
	}
	.banner.success .t {
		color: var(--accent-2);
	}
	.banner.warn {
		background: color-mix(in srgb, var(--warning) 12%, var(--surface));
		border-color: color-mix(in srgb, var(--warning) 45%, var(--border));
		border-left: 3px solid var(--warning);
	}
	.banner.warn .ico {
		background: var(--warning);
		color: #20160a;
	}
	.ico {
		display: inline-grid;
		place-items: center;
		width: 28px;
		height: 28px;
		border-radius: var(--radius-pill);
		margin-top: 0.05rem;
	}
	.text {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		min-width: 0;
		font-size: 0.9rem;
		line-height: 1.5;
	}
	.t {
		font-weight: 700;
		color: var(--text);
	}
	.b {
		color: var(--muted);
		white-space: pre-line;
	}
	.x {
		display: inline-grid;
		place-items: center;
		width: 30px;
		height: 30px;
		flex-shrink: 0;
		border: none;
		background: transparent;
		color: var(--muted);
		border-radius: var(--radius-pill);
		cursor: pointer;
	}
	.x:hover {
		color: var(--text);
		background: var(--surface-2);
	}
</style>
