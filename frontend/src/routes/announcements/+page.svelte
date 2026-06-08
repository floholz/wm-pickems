<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth } from '$lib/auth.svelte';
	import {
		api,
		type Announcement,
		type AnnounceLevel
	} from '$lib/api';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import {
		Megaphone,
		Info,
		TriangleAlert,
		Send,
		Pencil,
		Trash2,
		Check,
		Plus,
		Zap,
		Pin
	} from '@lucide/svelte';

	// Gate: anyone who isn't an owner/admin goes home. auth hydrates
	// synchronously from the stored token, so isAdmin is accurate on mount.
	$effect(() => {
		if (!auth.isAdmin) goto('/');
	});

	let items = $state<Announcement[]>([]);
	let loaded = $state(false);
	let error = $state('');

	// Form state — doubles as create and edit.
	let editId = $state<string | null>(null);
	let title = $state('');
	let body = $state('');
	let level = $state<AnnounceLevel>('info');
	let active = $state(true);
	let highPriority = $state(false);
	let persistent = $state(false);
	let saving = $state(false);
	let formError = $state('');

	// Confirm dialogs.
	let confirmDelete = $state<Announcement | null>(null);
	let confirmSend = $state<Announcement | null>(null);
	let dialogBusy = $state(false);

	// Per-row inline status (e.g. send result).
	let rowMsg = $state<Record<string, string>>({});

	const levelMeta: Record<AnnounceLevel, { label: string; Icon: typeof Info }> =
		{
			info: { label: 'Info', Icon: Info },
			success: { label: 'Highlight', Icon: Megaphone },
			warn: { label: 'Warning', Icon: TriangleAlert }
		};

	async function load() {
		try {
			const res = await api.allAnnouncements();
			items = res.announcements;
		} catch {
			error = 'Could not load announcements.';
		} finally {
			loaded = true;
		}
	}

	$effect(() => {
		if (auth.isAdmin && !loaded) load();
	});

	function resetForm() {
		editId = null;
		title = '';
		body = '';
		level = 'info';
		active = true;
		highPriority = false;
		persistent = false;
		formError = '';
	}

	function startEdit(a: Announcement) {
		editId = a.id;
		title = a.title;
		body = a.body;
		level = a.level;
		active = a.active;
		highPriority = a.highPriority;
		persistent = a.persistent;
		formError = '';
		if (typeof window !== 'undefined') window.scrollTo({ top: 0, behavior: 'smooth' });
	}

	async function save() {
		formError = '';
		if (!title.trim() || !body.trim()) {
			formError = 'Title and body are required.';
			return;
		}
		saving = true;
		try {
			const payload = {
				title: title.trim(),
				body: body.trim(),
				level,
				active,
				highPriority,
				persistent
			};
			if (editId) {
				const updated = await api.updateAnnouncement(editId, payload);
				items = items.map((a) => (a.id === editId ? updated : a));
			} else {
				const created = await api.createAnnouncement(payload);
				items = [created, ...items];
			}
			resetForm();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Save failed.';
		} finally {
			saving = false;
		}
	}

	async function toggleActive(a: Announcement) {
		try {
			const updated = await api.updateAnnouncement(a.id, { active: !a.active });
			items = items.map((x) => (x.id === a.id ? updated : x));
		} catch {
			rowMsg = { ...rowMsg, [a.id]: 'Could not update.' };
		}
	}

	async function doDelete() {
		if (!confirmDelete) return;
		dialogBusy = true;
		try {
			await api.deleteAnnouncement(confirmDelete.id);
			items = items.filter((a) => a.id !== confirmDelete!.id);
			if (editId === confirmDelete.id) resetForm();
			confirmDelete = null;
		} catch {
			/* leave dialog open on failure */
		} finally {
			dialogBusy = false;
		}
	}

	async function doSend() {
		if (!confirmSend) return;
		dialogBusy = true;
		const id = confirmSend.id;
		try {
			const res = await api.sendAnnouncement(id);
			items = items.map((a) => (a.id === id ? res.announcement : a));
			rowMsg = {
				...rowMsg,
				[id]: `Sent — ${res.result.sent} delivered, ${res.result.skipped} skipped.`
			};
			confirmSend = null;
		} catch (e) {
			rowMsg = { ...rowMsg, [id]: e instanceof Error ? e.message : 'Send failed.' };
			confirmSend = null;
		} finally {
			dialogBusy = false;
		}
	}

	function fmtDate(iso: string): string {
		if (!iso) return '';
		const d = new Date(iso);
		return d.toLocaleDateString(undefined, {
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}
</script>

<div class="head">
	<div>
		<p class="kicker">Admin</p>
		<h1>Announcements</h1>
	</div>
</div>

{#if !auth.isAdmin}
	<p class="muted">Restricted.</p>
{:else}
	<!-- Create / edit form -->
	<section class="card form">
		<h2 class="sec">
			{#if editId}<Pencil size={18} /> Edit announcement{:else}<Plus size={18} /> New announcement{/if}
		</h2>
		<label class="fld">
			<span>Title</span>
			<input bind:value={title} maxlength="120" placeholder="What's new?" />
		</label>
		<label class="fld">
			<span>Message</span>
			<textarea bind:value={body} maxlength="2000" rows="3" placeholder="A short message for everyone…"></textarea>
		</label>
		<div class="row">
			<label class="fld grow">
				<span>Style</span>
				<select bind:value={level}>
					<option value="info">Info</option>
					<option value="success">Highlight</option>
					<option value="warn">Warning</option>
				</select>
			</label>
			<label class="chk">
				<input type="checkbox" bind:checked={active} />
				<span>Show banner now</span>
			</label>
		</div>
		<label class="chk hp">
			<input type="checkbox" bind:checked={highPriority} />
			<span><Zap size={14} /> High-priority push</span>
		</label>
		<p class="hp-hint">
			Only affects "Send as notification": delivers at high urgency (reaches
			dozing phones promptly) and keeps the alert on screen until tapped.
		</p>
		<label class="chk hp">
			<input type="checkbox" bind:checked={persistent} />
			<span><Pin size={14} /> Persistent</span>
		</label>
		<p class="hp-hint">
			Can't be dismissed — users can only collapse it to a slim ribbon. Stays
			until you hide or delete it.
		</p>
		{#if formError}<p class="err">{formError}</p>{/if}
		<div class="actions">
			{#if editId}
				<button class="btn ghost" onclick={resetForm} disabled={saving}>Cancel</button>
			{/if}
			<button class="btn" onclick={save} disabled={saving}>
				{saving ? 'Saving…' : editId ? 'Save changes' : 'Create announcement'}
			</button>
		</div>
	</section>

	<!-- List -->
	{#if !loaded}
		<p class="muted">Loading…</p>
	{:else if error}
		<p class="err">{error}</p>
	{:else if items.length === 0}
		<p class="muted empty">No announcements yet. Create one above.</p>
	{:else}
		<ul class="list">
			{#each items as a (a.id)}
				{@const Icon = levelMeta[a.level].Icon}
				<li class="ann {a.level}" class:inactive={!a.active}>
					<div class="ann-top">
						<span class="badge"><Icon size={14} /> {levelMeta[a.level].label}</span>
						{#if a.active}
							<span class="pill on">Live</span>
						{:else}
							<span class="pill off">Hidden</span>
						{/if}
						{#if a.persistent}
							<span class="pill pin" title="Persistent — users can only collapse it">
								<Pin size={12} /> Persistent
							</span>
						{/if}
						{#if a.highPriority}
							<span class="pill hp" title="High-priority push when broadcast">
								<Zap size={12} /> Priority
							</span>
						{/if}
						{#if a.notifiedAt}
							<span class="pill sent" title={`Notified ${fmtDate(a.notifiedAt)}`}>
								<Check size={12} /> Notified
							</span>
						{/if}
						<span class="when">{fmtDate(a.created)}</span>
					</div>
					<strong class="ann-title">{a.title}</strong>
					<p class="ann-body">{a.body}</p>

					{#if rowMsg[a.id]}<p class="rowmsg">{rowMsg[a.id]}</p>{/if}

					<div class="ann-actions">
						<button class="ib" onclick={() => toggleActive(a)} title={a.active ? 'Hide banner' : 'Show banner'}>
							{a.active ? 'Hide' : 'Show'}
						</button>
						<button class="ib" onclick={() => startEdit(a)} title="Edit">
							<Pencil size={15} /> Edit
						</button>
						<button
							class="ib send"
							onclick={() => (confirmSend = a)}
							disabled={!!a.notifiedAt}
							title={a.notifiedAt ? 'Already sent as a notification' : 'Send as email + push to everyone'}
						>
							<Send size={15} /> {a.notifiedAt ? 'Sent' : 'Send'}
						</button>
						<button class="ib danger" onclick={() => (confirmDelete = a)} title="Delete">
							<Trash2 size={15} />
						</button>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
{/if}

<ConfirmDialog
	open={!!confirmDelete}
	title="Delete announcement?"
	message="This removes it for everyone and can't be undone."
	confirmLabel="Delete"
	danger
	busy={dialogBusy}
	onconfirm={doDelete}
	oncancel={() => (confirmDelete = null)}
/>

<ConfirmDialog
	open={!!confirmSend}
	title="Send as notification?"
	message="This emails and pushes this announcement to every eligible user. It can only be sent once."
	confirmLabel="Send to everyone"
	busy={dialogBusy}
	onconfirm={doSend}
	oncancel={() => (confirmSend = null)}
/>

<style>
	.head {
		margin-bottom: 1.1rem;
	}
	.sec {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		margin: 0 0 0.9rem;
		font-size: 0.95rem;
		font-weight: 700;
		color: var(--muted);
	}
	.form {
		margin-bottom: 1.4rem;
	}
	.fld {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		margin-bottom: 0.8rem;
	}
	.fld span {
		font-size: 0.82rem;
		font-weight: 600;
		color: var(--muted);
	}
	.fld input,
	.fld textarea,
	.fld select {
		width: 100%;
	}
	.fld textarea {
		resize: vertical;
	}
	.row {
		display: flex;
		align-items: flex-end;
		gap: 1rem;
		flex-wrap: wrap;
	}
	.grow {
		flex: 1;
		min-width: 150px;
		margin-bottom: 0;
	}
	.chk {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.88rem;
		color: var(--text);
		padding-bottom: 0.55rem;
		cursor: pointer;
	}
	.chk input {
		width: 16px;
		height: 16px;
		accent-color: var(--accent);
	}
	.chk.hp {
		margin-top: 0.9rem;
		padding-bottom: 0;
	}
	.chk.hp span {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
	}
	.hp-hint {
		margin: 0.3rem 0 0 1.6rem;
		font-size: 0.78rem;
		line-height: 1.45;
		color: var(--muted);
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.6rem;
		margin-top: 1rem;
	}
	.actions .btn {
		width: auto;
	}
	.empty {
		text-align: center;
		padding: 2rem 0;
	}
	.list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.8rem;
	}
	.ann {
		/* info (default) is neutral grey; highlight + warn carry colour, matching
		   the in-app banner so the styles read the same in both places. */
		--tone: color-mix(in srgb, var(--muted) 60%, var(--border));
		padding: 0.9rem 1rem;
		background: var(--surface);
		border: 1px solid var(--border);
		border-left: 3px solid var(--tone);
		border-radius: var(--radius-sm);
	}
	.ann.success {
		--tone: var(--accent);
	}
	.ann.warn {
		--tone: var(--warning);
	}
	.ann.inactive {
		opacity: 0.62;
	}
	.ann-top {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
		margin-bottom: 0.45rem;
	}
	.badge {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		font-size: 0.72rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--tone);
	}
	.pill {
		font-size: 0.7rem;
		font-weight: 700;
		padding: 0.12rem 0.5rem;
		border-radius: var(--radius-pill);
		border: 1px solid var(--border);
		color: var(--muted);
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
	}
	.pill.on {
		color: var(--success);
		border-color: color-mix(in srgb, var(--success) 45%, var(--border));
	}
	.pill.sent {
		color: var(--accent);
		border-color: color-mix(in srgb, var(--accent) 45%, var(--border));
	}
	.pill.hp {
		color: var(--warning);
		border-color: color-mix(in srgb, var(--warning) 45%, var(--border));
	}
	.pill.pin {
		color: var(--accent);
		border-color: color-mix(in srgb, var(--accent) 45%, var(--border));
	}
	.when {
		margin-left: auto;
		font-size: 0.75rem;
		color: var(--muted);
	}
	.ann-title {
		display: block;
		font-weight: 700;
		margin-bottom: 0.15rem;
	}
	.ann-body {
		margin: 0;
		font-size: 0.9rem;
		line-height: 1.5;
		color: var(--muted);
		white-space: pre-line;
	}
	.rowmsg {
		margin: 0.5rem 0 0;
		font-size: 0.82rem;
		color: var(--accent);
	}
	.ann-actions {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		flex-wrap: wrap;
		margin-top: 0.8rem;
	}
	.ib {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		padding: 0.35rem 0.6rem;
		font-size: 0.82rem;
		font-weight: 600;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--text);
		cursor: pointer;
	}
	.ib:hover:not(:disabled) {
		border-color: var(--accent);
	}
	.ib:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.ib.send:hover:not(:disabled) {
		color: var(--accent);
	}
	.ib.danger {
		color: var(--danger);
		margin-left: auto;
	}
	.ib.danger:hover {
		border-color: var(--danger);
	}
	.err {
		color: var(--danger);
		font-size: 0.85rem;
	}
</style>
