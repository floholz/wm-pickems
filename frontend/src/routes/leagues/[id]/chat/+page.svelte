<script lang="ts">
	import { tick } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { pb } from '$lib/pb';
	import { auth } from '$lib/auth.svelte';
	import { api, type ChatMessage, type ChatMember } from '$lib/api';
	import Avatar from '$lib/components/Avatar.svelte';
	import { ArrowLeft, SendHorizontal, Trash2 } from '@lucide/svelte';

	let id = $derived($page.params.id ?? '');

	let leagueName = $state('');
	let owner = $state(false); // league owner → may delete any message
	let ready = $state(false);
	let error = $state('');
	let messages = $state<ChatMessage[]>([]);
	let members = $state<Record<string, ChatMember>>({});
	let hasMore = $state(false);
	let loadingOlder = $state(false);
	let text = $state('');
	let sending = $state(false);

	let listEl = $state<HTMLDivElement | null>(null);
	let taEl = $state<HTMLTextAreaElement | null>(null);
	let unsub: (() => void) | null = null;

	// Grow the composer with its content up to the CSS max-height (~5 lines),
	// after which it scrolls. Reset to min when cleared.
	function autosize() {
		const el = taEl;
		if (!el) return;
		el.style.height = 'auto';
		el.style.height = `${el.scrollHeight}px`;
	}

	const me = $derived(auth.user?.id ?? '');

	function avatarUrl(userId: string, avatar?: string): string | null {
		return avatar
			? pb.files.getURL({ id: userId, collectionName: 'users' }, avatar)
			: null;
	}

	async function loadMembers() {
		try {
			const res = await api.chatMembers(id);
			const map: Record<string, ChatMember> = {};
			for (const m of res.members) map[m.userId] = m;
			members = map;
		} catch {
			/* keep what we have */
		}
	}

	async function scrollToBottom() {
		await tick();
		if (listEl) listEl.scrollTop = listEl.scrollHeight;
	}

	async function init() {
		// Verify the league: must be a private one we belong to.
		try {
			const mine = (await api.myLeagues()).leagues;
			const lg = mine.find((l) => l.id === id);
			if (!lg || lg.inviteCode === 'GLOBAL') {
				goto(`/leagues/${id}`);
				return;
			}
			leagueName = lg.name;
			owner = lg.role === 'owner';
		} catch {
			error = 'Could not open this chat.';
			ready = true;
			return;
		}

		await loadMembers();
		try {
			const res = await api.chatHistory(id);
			messages = res.messages.slice().reverse(); // endpoint is newest-first
			hasMore = res.hasMore;
		} catch {
			error = 'Could not load messages.';
		}
		ready = true;
		scrollToBottom();
		api.chatMarkRead(id).catch(() => {});

		// Live updates. The subscription is authorised by the collection's
		// member-only view rule.
		unsub = await pb
			.collection('league_messages')
			.subscribe(
				'*',
				(e) => {
					const r = e.record as unknown as {
						id: string;
						user: string;
						text: string;
						created: string;
						deleted?: boolean;
					};
					if (e.action === 'create') {
						if (messages.some((m) => m.id === r.id)) return; // echo of our own post
						if (!members[r.user]) loadMembers(); // unknown sender → refresh directory
						messages = [...messages, { id: r.id, user: r.user, text: r.text, created: r.created }];
						scrollToBottom();
						if (r.user !== me) api.chatMarkRead(id).catch(() => {});
					} else if (e.action === 'update') {
						// Soft-delete: text is cleared in the payload; keep any `original`
						// an admin already has (realtime never carries it).
						messages = messages.map((m) =>
							m.id === r.id ? { ...m, text: r.text ?? '', deleted: r.deleted ?? m.deleted } : m
						);
					} else if (e.action === 'delete') {
						messages = messages.filter((m) => m.id !== r.id);
					}
				},
				{ filter: `league="${id}"` }
			)
			.catch(() => null);
	}

	$effect(() => {
		if (id && !ready) init();
		return () => {
			unsub?.();
			unsub = null;
		};
	});

	async function send() {
		const body = text.trim();
		if (!body || sending) return;
		sending = true;
		try {
			const msg = await api.chatPost(id, body);
			text = '';
			await tick();
			autosize(); // collapse back to one line
			if (!messages.some((m) => m.id === msg.id)) messages = [...messages, msg];
			scrollToBottom();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not send.';
		} finally {
			sending = false;
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			send();
		}
	}

	async function loadOlder() {
		if (loadingOlder || !messages.length) return;
		loadingOlder = true;
		const oldest = messages[0].created;
		const prevH = listEl?.scrollHeight ?? 0;
		try {
			const res = await api.chatHistory(id, oldest);
			messages = [...res.messages.slice().reverse(), ...messages];
			hasMore = res.hasMore;
			// keep the viewport anchored where the user was
			await tick();
			if (listEl) listEl.scrollTop = listEl.scrollHeight - prevH;
		} catch {
			/* ignore */
		} finally {
			loadingOlder = false;
		}
	}

	async function remove(m: ChatMessage) {
		try {
			const res = await api.chatDelete(id, m.id);
			// Soft-delete: replace with the cleared/annotated version in place.
			messages = messages.map((x) => (x.id === m.id ? { ...x, ...res } : x));
		} catch {
			/* ignore */
		}
	}

	// Keep the composer placeholder to one line: hard-cap a long league name to n
	// chars, preferring a word boundary only when one lands far enough in (so a
	// single very long word still gets cut). "Message <name>…" then never wraps.
	function clipName(s: string, n = 20): string {
		if (s.length <= n) return s;
		const cut = s.slice(0, n).trimEnd();
		const sp = cut.lastIndexOf(' ');
		return sp >= n / 2 ? cut.slice(0, sp) : cut;
	}

	function fmtTime(iso: string): string {
		const d = new Date(iso.replace(' ', 'T'));
		return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
	}
	function fmtDay(iso: string): string {
		return new Date(iso.replace(' ', 'T')).toLocaleDateString(undefined, {
			weekday: 'short',
			month: 'short',
			day: 'numeric'
		});
	}
	function dayKey(iso: string): string {
		return new Date(iso.replace(' ', 'T')).toDateString();
	}
</script>

<div class="chat">
	<header class="chead">
		<a class="back" href={`/leagues/${id}`} aria-label="Back to league">
			<ArrowLeft size={18} />
		</a>
		<div class="ctitle">
			<span class="ckicker">League chat</span>
			<h1>{leagueName || '…'}</h1>
		</div>
	</header>

	{#if error && !messages.length}
		<p class="err pad">{error}</p>
	{:else if !ready}
		<p class="muted pad">Loading…</p>
	{:else}
		<div class="messages" bind:this={listEl}>
			<div class="msgs-inner">
			{#if hasMore}
				<button class="older" onclick={loadOlder} disabled={loadingOlder}>
					{loadingOlder ? 'Loading…' : 'Load older messages'}
				</button>
			{/if}
			{#if messages.length === 0}
				<p class="empty muted">No messages yet — say hi 👋</p>
			{/if}
			{#each messages as m, i (m.id)}
				{@const mem = members[m.user]}
				{@const mine = m.user === me}
				{@const prev = messages[i - 1]}
				{@const newDay = !prev || dayKey(prev.created) !== dayKey(m.created)}
				{@const grouped = !newDay && prev && prev.user === m.user}
				{#if newDay}
					<div class="daysep"><span>{fmtDay(m.created)}</span></div>
				{/if}
				<div class="row" class:mine class:grouped>
					{#if !mine}
						<div class="ava">
							{#if !grouped}
								<Avatar name={mem?.name ?? '?'} src={avatarUrl(m.user, mem?.avatar)} size={28} />
							{/if}
						</div>
					{/if}
					<div class="bubblewrap">
						{#if !grouped && !mine}
							<span class="who">{mem?.name ?? 'Member'}</span>
						{/if}
						<div class="bubble" class:deleted={m.deleted}>
							{#if m.deleted}
								{#if m.original}
									<span class="msgtext"><span class="modtag">deleted</span> {m.original}</span>
								{:else}
									<span class="msgtext gone">message deleted</span>
								{/if}
							{:else}
								<span class="msgtext">{m.text}</span>
							{/if}
							{#if (mine || owner) && !m.deleted}
								<button class="del" title="Delete" aria-label="Delete" onclick={() => remove(m)}>
									<Trash2 size={13} />
								</button>
							{/if}
						</div>
						<span class="time">{fmtTime(m.created)}</span>
					</div>
				</div>
			{/each}
			</div>
		</div>

		<form class="composer" onsubmit={(e) => (e.preventDefault(), send())}>
			<textarea
				bind:this={taEl}
				bind:value={text}
				oninput={autosize}
				onkeydown={onKeydown}
				placeholder="Message {clipName(leagueName)}…"
				rows="1"
				maxlength="2000"
			></textarea>
			<button class="sendbtn" disabled={!text.trim() || sending} aria-label="Send">
				<SendHorizontal size={18} />
			</button>
		</form>
		{#if error}<p class="err sendErr">{error}</p>{/if}
	{/if}
</div>

<style>
	.chat {
		display: flex;
		flex-direction: column;
		min-height: 0;
		/* Mobile: pin between the fixed top bar and tab bar so only the messages
		   list scrolls — never the page (which would drag the composer along).
		   With interactive-widget=resizes-content the layout shrinks when the
		   keyboard opens, keeping the composer above it. */
		position: fixed;
		inset: var(--topbar-h) 0 var(--nav-h) 0;
		padding: 0.75rem 1rem;
	}
	@media (min-width: 900px) {
		.chat {
			position: static;
			inset: auto;
			padding: 0;
			height: calc(100dvh - 4rem);
		}
	}
	.chead {
		display: flex;
		align-items: center;
		gap: 0.7rem;
		padding-bottom: 0.8rem;
		border-bottom: 1px solid var(--border);
		flex: none;
	}
	.back {
		display: inline-grid;
		place-items: center;
		width: 36px;
		height: 36px;
		border-radius: var(--radius-sm);
		background: var(--surface-2);
		border: 1px solid var(--border);
		color: var(--text);
		flex: none;
	}
	.ctitle {
		min-width: 0;
	}
	.ckicker {
		font-size: 0.72rem;
		font-weight: 800;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--accent);
	}
	.chead h1 {
		margin: 0;
		font-size: 1.3rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.pad {
		padding: 1rem 0;
	}

	.messages {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		padding: 0.9rem 0.1rem 0.6rem;
	}
	/* Anchor messages to the bottom: when they don't fill the area they sit just
	   above the composer (so they stay readable when the mobile keyboard pushes
	   the view up); when they overflow, the auto margin collapses and it scrolls. */
	.msgs-inner {
		margin-top: auto;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.empty {
		text-align: center;
		padding: 0.5rem 0;
	}
	.older {
		align-self: center;
		margin-bottom: 0.6rem;
		padding: 0.3rem 0.8rem;
		font-size: 0.8rem;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: var(--radius-pill);
		color: var(--muted);
		cursor: pointer;
	}
	.daysep {
		display: flex;
		align-items: center;
		justify-content: center;
		margin: 0.6rem 0 0.4rem;
	}
	.daysep span {
		font-size: 0.72rem;
		color: var(--muted);
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: var(--radius-pill);
		padding: 0.1rem 0.6rem;
	}
	.row {
		display: flex;
		align-items: flex-end;
		gap: 0.5rem;
		max-width: 85%;
	}
	.row.grouped {
		margin-top: -0.1rem;
	}
	.row.mine {
		align-self: flex-end;
		flex-direction: row-reverse;
	}
	.ava {
		width: 28px;
		flex: none;
	}
	.bubblewrap {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}
	.row.mine .bubblewrap {
		align-items: flex-end;
	}
	.who {
		font-size: 0.75rem;
		font-weight: 700;
		color: var(--muted);
		margin: 0 0 0.15rem 0.1rem;
	}
	.bubble {
		position: relative;
		display: flex;
		align-items: flex-start;
		gap: 0.35rem;
		padding: 0.45rem 0.7rem;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: 14px;
		border-bottom-left-radius: 4px;
		font-size: 0.92rem;
		line-height: 1.45;
	}
	.row.mine .bubble {
		background: color-mix(in srgb, var(--accent) 16%, var(--surface-2));
		border-color: color-mix(in srgb, var(--accent) 35%, var(--border));
		border-bottom-left-radius: 14px;
		border-bottom-right-radius: 4px;
	}
	.msgtext {
		white-space: pre-wrap;
		word-break: break-word;
		min-width: 0;
	}
	.bubble.deleted {
		background: var(--surface);
		border-style: dashed;
	}
	.row.mine .bubble.deleted {
		background: var(--surface);
		border-color: var(--border);
	}
	.gone {
		color: var(--muted);
		font-style: italic;
	}
	/* Admin-only view of a deleted message's original text. */
	.modtag {
		font-size: 0.6rem;
		font-weight: 800;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--danger);
		border: 1px solid color-mix(in srgb, var(--danger) 45%, var(--border));
		border-radius: var(--radius-pill);
		padding: 0 0.35rem;
		margin-right: 0.35rem;
		vertical-align: 1px;
	}
	.del {
		display: none;
		flex: none;
		margin-top: 0.05rem;
		padding: 0;
		background: none;
		border: none;
		color: var(--muted);
		cursor: pointer;
	}
	.bubble:hover .del {
		display: inline-flex;
	}
	.del:hover {
		color: var(--danger);
	}
	.time {
		font-size: 0.68rem;
		color: var(--muted);
		margin: 0.15rem 0.2rem 0;
	}

	.composer {
		display: flex;
		align-items: flex-end;
		gap: 0.5rem;
		padding-top: 0.7rem;
		border-top: 1px solid var(--border);
		flex: none;
	}
	.composer textarea {
		flex: 1;
		resize: none;
		min-height: 2.6rem;
		max-height: 7.6rem; /* ~5 lines, then scroll */
		overflow-y: auto;
		padding: 0.6rem 0.8rem;
		line-height: 1.4;
		font: inherit;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		color: var(--text);
	}
	.composer textarea:focus {
		outline: none;
		border-color: var(--accent);
	}
	.sendbtn {
		display: inline-grid;
		place-items: center;
		width: 44px;
		height: 44px;
		flex: none;
		border: none;
		border-radius: var(--radius);
		background: var(--accent);
		color: var(--accent-fg);
		cursor: pointer;
	}
	.sendbtn:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.err {
		color: var(--danger);
	}
	.sendErr {
		font-size: 0.82rem;
		margin: 0.4rem 0 0;
	}
</style>
