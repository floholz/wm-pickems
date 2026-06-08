<script lang="ts">
	import { page } from '$app/stores';
	import { api, type LeaderboardRow, type BotSummary } from '$lib/api';
	import { auth } from '$lib/auth.svelte';
	import { pb } from '$lib/pb';
	import Avatar from '$lib/components/Avatar.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import {
		Eye,
		EyeOff,
		Copy,
		Share2,
		ChevronDown,
		Telescope,
		Settings,
		Check,
		X,
		Lock,
		RefreshCw,
		UserMinus,
		UserPlus,
		Bot,
		ShieldCheck,
		MessageSquare,
		ChevronRight
	} from '@lucide/svelte';

	interface Cfg {
		match: {
			tendency: number;
			exact: number;
			totalGoals: number;
			goalDiff: number;
		};
		forecast: {
			groupPosition: number;
			perfectGroupBonus: number;
			advance: number;
			round: Record<string, number>;
		};
		tiebreakers: string[];
	}
	let cfg = $state<Cfg | null>(null);

	const tbLabel: Record<string, string> = {
		points: 'Total points',
		exactScores: 'Most exact scores',
		correctWinners: 'Most correct winners',
		goalDiffDeviation: 'Smallest goal-difference error vs. results',
		fewestTips: 'Fewest tips submitted',
		earliestEdit: 'Earliest last edit (submitted first)'
	};
	const roundLabel: Record<string, string> = {
		R32: 'Round of 32',
		R16: 'Round of 16',
		QF: 'Quarter-final',
		SF: 'Semi-final',
		FINAL: 'Final',
		CHAMPION: 'Champion'
	};

	let revealed = $state(false);
	let openRow = $state<string | null>(null);
	// The invite/share card is hidden by default behind the header "Share" toggle.
	let showShare = $state(false);

	let id = $derived($page.params.id ?? '');
	let league = $state<{ id: string; name: string } | null>(null);
	let rows = $state<LeaderboardRow[]>([]);
	let invite = $state('');
	let loaded = $state(false);
	let error = $state('');
	let tab = $state<'total' | 'tipsPoints' | 'forecastPoints'>('total');

	// Chat (private leagues only).
	let canChat = $state(false);
	let chatUnread = $state(0);

	// Owner-only management.
	let isOwner = $state(false);
	let isPrivate = $state(false);
	let editing = $state(false);
	let nameDraft = $state('');
	let confirmRegen = $state(false);
	let mgmtBusy = $state(false);
	let mgmtError = $state('');
	let availableBots = $state<BotSummary[]>([]);
	let botBusy = $state<string | null>(null);

	$effect(() => {
		const lid = id;
		loaded = false;
		cfg = null;
		editing = false;
		confirmRegen = false;
		showShare = false;
		mgmtError = '';
		availableBots = [];
		Promise.all([api.leaderboard(lid), api.myLeagues()])
			.then(([lb, mine]) => {
				league = lb.league;
				rows = lb.rows;
				cfg = (lb.scoring as Cfg | undefined) ?? null;
				const me = mine.leagues.find((l) => l.id === lid);
				invite = me?.inviteCode ?? '';
				isOwner = me?.role === 'owner';
				isPrivate = me?.private ?? false;
				canChat = !!me && me.inviteCode !== 'GLOBAL';
				if (canChat) {
					api
						.chatUnread()
						.then((r) => (chatUnread = r.unread[lid] ?? 0))
						.catch(() => {});
				}
			})
			.catch(() => (error = 'Could not load this league.'))
			.finally(() => (loaded = true));
	});

	// Keep the chat unread badge live: bump it when a new message lands in this
	// league while we're on the league page (the chat itself marks read on open,
	// and re-mounting the page re-syncs the baseline count).
	$effect(() => {
		if (!canChat || !id) return;
		const lid = id;
		let unsub: (() => void) | null = null;
		pb.collection('league_messages')
			.subscribe(
				'*',
				(e) => {
					if (e.action === 'create' && (e.record as { user?: string }).user !== auth.user?.id) {
						chatUnread += 1;
					}
				},
				{ filter: `league="${lid}"` }
			)
			.then((u) => (unsub = u))
			.catch(() => {});
		return () => unsub?.();
	});

	function enterEdit() {
		nameDraft = league?.name ?? '';
		mgmtError = '';
		confirmRegen = false;
		editing = true;
		loadBots();
	}

	async function loadBots() {
		if (!isOwner) return;
		try {
			availableBots = (await api.availableBots(id)).bots;
		} catch {
			availableBots = [];
		}
	}

	async function refreshRows() {
		try {
			rows = (await api.leaderboard(id)).rows;
		} catch {
			/* keep current rows on a transient error */
		}
	}

	async function addBot(b: BotSummary) {
		if (!league) return;
		botBusy = b.userId;
		mgmtError = '';
		try {
			await api.addBot(league.id, b.userId);
			availableBots = availableBots.filter((x) => x.userId !== b.userId);
			await refreshRows();
		} catch {
			mgmtError = `Could not add ${b.name}.`;
		} finally {
			botBusy = null;
		}
	}
	function exitEdit() {
		editing = false;
		confirmRegen = false;
	}
	async function saveName() {
		const name = nameDraft.trim();
		if (!league) return;
		if (!name || name === league.name) {
			exitEdit();
			return;
		}
		mgmtBusy = true;
		mgmtError = '';
		try {
			await api.renameLeague(league.id, name);
			league = { ...league, name };
			exitEdit();
		} catch {
			mgmtError = 'Could not rename the league.';
		} finally {
			mgmtBusy = false;
		}
	}
	async function setPrivacy(next: boolean) {
		if (!league || next === isPrivate) return;
		mgmtBusy = true;
		mgmtError = '';
		try {
			await api.setCodePrivacy(league.id, next);
			isPrivate = next;
		} catch {
			mgmtError = 'Could not update visibility.';
		} finally {
			mgmtBusy = false;
		}
	}
	async function regenerate() {
		if (!league) return;
		mgmtBusy = true;
		mgmtError = '';
		try {
			const r = await api.regenerateCode(league.id);
			invite = r.inviteCode;
			confirmRegen = false;
			revealed = true;
		} catch {
			mgmtError = 'Could not regenerate the code.';
		} finally {
			mgmtBusy = false;
		}
	}
	// Two-step removal: clicking the button opens a confirm dialog; confirming
	// runs the actual call. removeTarget holds the pending member (and drives
	// the dialog's open state + message).
	let removeTarget = $state<{ userId: string; name: string } | null>(null);

	function requestRemove(userId: string, name: string) {
		removeTarget = { userId, name };
	}
	async function confirmRemove() {
		if (!league || !removeTarget) return;
		const { userId } = removeTarget;
		mgmtBusy = true;
		mgmtError = '';
		try {
			await api.removeMember(league.id, userId);
			rows = rows.filter((r) => r.userId !== userId);
			// A removed bot becomes available to add again.
			await loadBots();
		} catch {
			mgmtError = 'Could not remove the member.';
		} finally {
			mgmtBusy = false;
			removeTarget = null;
		}
	}

	let sorted = $derived(
		[...rows].sort((a, b) => b[tab] - a[tab])
	);
	let fcView = $derived(tab === 'forecastPoints');

	// Build the avatar URL the same way auth.svelte does — a users.avatar file
	// resolves to /api/files/users/{id}/{filename} (same origin).
	function avatarUrl(userId: string, avatar?: string | null): string | null {
		return avatar
			? pb.files.getURL({ id: userId, collectionName: 'users' }, avatar)
			: null;
	}

	function copyInvite() {
		navigator.clipboard?.writeText(invite);
	}

	let linkCopied = $state(false);
	let copyTimer: ReturnType<typeof setTimeout>;
	function shareInvite() {
		const url = `${window.location.origin}/join/${invite}`;
		navigator.clipboard?.writeText(url);
		linkCopied = true;
		clearTimeout(copyTimer);
		copyTimer = setTimeout(() => (linkCopied = false), 1800);
	}
</script>

<a href="/leagues" class="muted back">← Leagues</a>

{#if error}
	<p class="error">{error}</p>
{:else if !loaded}
	<p class="muted">Loading…</p>
{:else if league}
	<div class="lhead">
		<div class="ltitle">
			<p class="kicker">League</p>
			{#if editing}
				<input
					class="input nameedit"
					bind:value={nameDraft}
					maxlength="64"
					aria-label="League name"
					onkeydown={(e) => e.key === 'Enter' && saveName()}
				/>
			{:else}
				<h1>{league.name}</h1>
			{/if}
		</div>
		{#if isOwner || (invite && invite !== 'GLOBAL')}
			<div class="lactions">
				{#if editing}
					<button
						class="btn secondary icon"
						onclick={saveName}
						disabled={mgmtBusy}
						aria-label="Save name"><Check size={18} /></button
					>
					<button
						class="btn secondary icon"
						onclick={exitEdit}
						disabled={mgmtBusy}
						aria-label="Done editing"><X size={18} /></button
					>
				{:else}
					{#if invite && invite !== 'GLOBAL'}
						<button
							class="btn secondary sharebtn"
							class:active={showShare}
							aria-pressed={showShare}
							onclick={() => (showShare = !showShare)}
						>
							<Share2 size={16} /> Share
						</button>
					{/if}
					{#if isOwner}
						<button
							class="btn secondary icon"
							onclick={enterEdit}
							aria-label="Manage league"><Settings size={18} /></button
						>
					{/if}
				{/if}
			</div>
		{/if}
	</div>

	{#if canChat}
		<a class="chatcta" href={`/leagues/${id}/chat`}>
			<MessageSquare size={20} class="cc-ico" />
			<span class="cc-label">League Chat</span>
			{#if chatUnread > 0}
				<span class="cc-badge">{chatUnread > 99 ? '99+' : chatUnread}</span>
			{/if}
			<ChevronRight size={18} class="cc-cv" />
		</a>
	{/if}

	{#if mgmtError}<p class="error">{mgmtError}</p>{/if}

	{#if editing}
		<section class="card vis">
			<div class="muted small">Invite code visibility</div>
			<div class="tabs vistabs">
				<button class:active={!isPrivate} onclick={() => setPrivacy(false)} disabled={mgmtBusy}
					>Members</button
				>
				<button class:active={isPrivate} onclick={() => setPrivacy(true)} disabled={mgmtBusy}
					>Private</button
				>
			</div>
			<p class="muted small hint">
				{isPrivate
					? 'Only you can see and share the invite code.'
					: 'Everyone in the league can see and share the invite code.'}
			</p>
		</section>
	{/if}

	{#if invite && invite !== 'GLOBAL' && (showShare || editing)}
		<section class="card invite">
			<div class="irow">
				<div class="ic">
					<div class="muted small">
						Invite code
						{#if isPrivate}<span class="lockpill"><Lock size={11} /> Private</span>{/if}
					</div>
					<div class="code" class:masked={!revealed}>
						{revealed ? invite : '•'.repeat(invite.length || 6)}
					</div>
				</div>
				<div class="spacer"></div>
				<button
					class="btn secondary eye"
					aria-label={revealed ? 'Hide code' : 'Reveal code'}
					onclick={() => (revealed = !revealed)}
				>
					{#if revealed}<EyeOff size={18} />{:else}<Eye size={18} />{/if}
				</button>
				<button class="btn secondary copy" onclick={copyInvite}>
					<Copy size={16} /> Copy
				</button>
			</div>
			<button class="btn share" onclick={shareInvite}>
				<Share2 size={16} />
				{linkCopied ? 'Link copied!' : 'Share invite link'}
			</button>
			{#if editing}
				{#if confirmRegen}
					<p class="muted small hint regwarn">
						This invalidates the current code and any links already shared.
					</p>
					<div class="regrow">
						<button class="btn danger" onclick={regenerate} disabled={mgmtBusy}>
							Regenerate
						</button>
						<button
							class="btn secondary"
							onclick={() => (confirmRegen = false)}
							disabled={mgmtBusy}>Cancel</button
						>
					</div>
				{:else}
					<button class="btn ghost regenbtn" onclick={() => (confirmRegen = true)}>
						<RefreshCw size={16} /> Regenerate code
					</button>
				{/if}
			{/if}
		</section>
	{/if}

	<section class="card">
		<div class="tabs">
			<button class:active={tab === 'total'} onclick={() => (tab = 'total')}>Overall</button>
			<button class:active={tab === 'tipsPoints'} onclick={() => (tab = 'tipsPoints')}>Tips</button>
			<button class:active={tab === 'forecastPoints'} onclick={() => (tab = 'forecastPoints')}>Forecast</button>
		</div>

		<table class="lb">
			<thead>
				<tr>
					<th>#</th>
					<th>Player</th>
					{#if fcView}
						<th class="num ext" title="Correct group positions">Grp</th>
						<th class="num ext" title="Correct advancers (group stage)">Adv</th>
						<th class="num ext" title="Predicted teams that reached the Round of 32">R32</th>
						<th class="num ext" title="…Round of 16">R16</th>
						<th class="num ext" title="…Quarter-finals">QF</th>
						<th class="num ext" title="…Semi-finals">SF</th>
						<th class="num ext" title="…the Final">F</th>
						<th class="num ext" title="Champion predicted correctly">Win</th>
					{:else}
						<th class="num ext" title="Matches predicted">Pred</th>
						<th class="num ext" title="Forecast points">FC</th>
						<th class="num ext" title="Exact scores (tiebreak 1)">Exact</th>
						<th class="num ext" title="Correct winners (tiebreak 2)">Win</th>
						<th class="num ext" title="Goal-diff error (tiebreak 3, lower is better)">GD&Delta;</th>
					{/if}
					<th class="num pts">Pts</th>
				</tr>
			</thead>
			<tbody>
				{#each sorted as r, i (r.userId)}
					{@const f = r.forecast ?? {}}
					<tr
						class:lead={r.userId === auth.user?.id}
						class="main"
						class:open={openRow === r.userId}
						onclick={() =>
							(openRow = openRow === r.userId ? null : r.userId)}
					>
						<td class="rank">{i + 1}</td>
						<td class="player">
							<div class="pwrap">
								<Avatar name={r.name} src={avatarUrl(r.userId, r.avatar)} size={28} />
								<span class="pname">{r.name}</span>
								{#if r.role === 'bot'}
									<span class="rolepill" title="Bot player"><Bot size={11} /> Bot</span>
								{:else if r.role === 'admin'}
									<span class="rolepill admin" title="Admin"><ShieldCheck size={11} /> Admin</span>
								{/if}
								<a
									class="fclink"
									href={`/forecast/${r.userId}`}
									title="View {r.name}'s forecast"
									onclick={(e) => e.stopPropagation()}
								>
									<Telescope size={15} />
								</a>
								{#if editing && r.userId !== auth.user?.id}
									<button
										class="rmbtn"
										title="Remove {r.name}"
										aria-label="Remove {r.name}"
										disabled={mgmtBusy}
										onclick={(e) => {
											e.stopPropagation();
											requestRemove(r.userId, r.name);
										}}
									>
										<UserMinus size={15} />
									</button>
								{/if}
								<ChevronDown size={14} class="rx" />
							</div>
						</td>
						{#if fcView}
							<td class="num ext digits">{f.groups ?? 0}</td>
							<td class="num ext digits">{f.advance ?? 0}</td>
							<td class="num ext digits">{f.R32 ?? 0}</td>
							<td class="num ext digits">{f.R16 ?? 0}</td>
							<td class="num ext digits">{f.QF ?? 0}</td>
							<td class="num ext digits">{f.SF ?? 0}</td>
							<td class="num ext digits">{f.FINAL ?? 0}</td>
							<td class="num ext digits">{f.champion ? '✓' : '–'}</td>
						{:else}
							<td class="num ext digits">{r.predicted}</td>
							<td class="num ext digits">{r.forecastPoints}</td>
							<td class="num ext digits">{r.exactScores}</td>
							<td class="num ext digits">{r.correctWinners}</td>
							<td class="num ext digits">{r.gdDeviation}</td>
						{/if}
						<td class="num pts digits">{r[tab]}</td>
					</tr>
					{#if openRow === r.userId}
						<tr class="detail">
							<td colspan="12">
								{#if fcView}
									<div class="stats">
										<span><i>Correct group positions</i><b>{f.groups ?? 0}</b></span>
										<span><i>Correct advancers</i><b>{f.advance ?? 0}</b></span>
										<span><i>Reached Round of 32</i><b>{f.R32 ?? 0}</b></span>
										<span><i>Reached Round of 16</i><b>{f.R16 ?? 0}</b></span>
										<span><i>Reached Quarter-finals</i><b>{f.QF ?? 0}</b></span>
										<span><i>Reached Semi-finals</i><b>{f.SF ?? 0}</b></span>
										<span><i>Reached the Final</i><b>{f.FINAL ?? 0}</b></span>
										<span><i>Champion correct</i><b>{f.champion ? 'Yes' : 'No'}</b></span>
									</div>
								{:else}
									<div class="stats">
										<span><i>Matches predicted</i><b>{r.predicted}</b></span>
										<span><i>Tip points</i><b>{r.tipsPoints}</b></span>
										<span><i>Forecast points</i><b>{r.forecastPoints}</b></span>
										<span><i>Exact scores</i><b>{r.exactScores}</b></span>
										<span><i>Correct winners</i><b>{r.correctWinners}</b></span>
										<span><i>Goal-diff error</i><b>{r.gdDeviation}</b></span>
									</div>
								{/if}
							</td>
						</tr>
					{/if}
				{/each}

				{#if editing && availableBots.length}
					<tr class="botsep">
						<td colspan="12">Add a bot player</td>
					</tr>
					{#each availableBots as b (b.userId)}
						<tr class="addbot">
							<td class="rank"></td>
							<td class="player" colspan="11">
								<div class="pwrap">
									<Avatar
										name={b.name}
										src={avatarUrl(b.userId, b.avatar)}
										size={28}
									/>
									<span class="pname">{b.name}</span>
									<span class="rolepill" title="Bot player">
										<Bot size={11} />
										{b.botKind || 'Bot'}
									</span>
									<button
										class="addbtn"
										title="Add {b.name} to this league"
										disabled={botBusy === b.userId || mgmtBusy}
										onclick={() => addBot(b)}
									>
										<UserPlus size={15} /> Add
									</button>
								</div>
							</td>
						</tr>
					{/each}
				{/if}
			</tbody>
		</table>
		<p class="muted small note">
			Points update automatically as results come in.
		</p>
	</section>

	{#if cfg}
		<details class="card legend">
			<summary>How points work</summary>

			<h4>Per match (your Tip) — max {cfg.match.tendency +
					cfg.match.exact +
					cfg.match.totalGoals +
					cfg.match.goalDiff} pt</h4>
			<ul class="leg">
				<li>
					<span>Correct result — group: 1 / X / 2; knockout: the team
						that advances</span><b>{cfg.match.tendency} pt</b>
				</li>
				<li><span>Exact score</span><b>+{cfg.match.exact} pt</b></li>
				<li><span>Correct total number of goals</span><b>+{cfg.match.totalGoals} pt</b></li>
				<li><span>Correct goal difference</span><b>+{cfg.match.goalDiff} pt</b></li>
			</ul>
			<p class="muted small">
				Knockout games have no draw — the result point is for the team
				that goes through. If a knockout game is decided in extra time,
				the score points use the after-extra-time score.
			</p>

			<h4>Tournament Forecast</h4>
			<ul class="leg">
				<li><span>Each team in its correct final group position</span><b>{cfg.forecast.groupPosition} pt</b></li>
				<li><span>Whole group ordered perfectly (bonus)</span><b>+{cfg.forecast.perfectGroupBonus} pt</b></li>
				<li>
					<span>Each team you predicted to advance (group top 2, or a
						best-third pick) that actually advances</span
					><b>{cfg.forecast.advance} pt</b>
				</li>
			</ul>
			<p class="muted small">
				Reaching a knockout round (per correctly predicted team):
			</p>
			<ul class="leg">
				{#each Object.entries(roundLabel) as [k, lbl] (k)}
					{#if cfg.forecast.round[k] != null}
						<li><span>{lbl}</span><b>{cfg.forecast.round[k]} pt</b></li>
					{/if}
				{/each}
			</ul>

			<h4>Tiebreakers (in order)</h4>
			<ol class="tiebreak">
				{#each cfg.tiebreakers as t (t)}
					<li>{tbLabel[t] ?? t}</li>
				{/each}
			</ol>
		</details>
	{/if}
{/if}

<ConfirmDialog
	open={removeTarget !== null}
	title="Remove member"
	message={removeTarget
		? `Remove ${removeTarget.name} from this league?`
		: ''}
	confirmLabel="Remove"
	cancelLabel="Cancel"
	danger
	busy={mgmtBusy}
	onconfirm={confirmRemove}
	oncancel={() => (removeTarget = null)}
/>

<style>
	.back {
		display: inline-block;
		margin: 0.5rem 0 0.75rem;
	}
	h1 {
		margin: 0 0 1rem;
	}
	.lhead {
		display: flex;
		align-items: flex-end;
		gap: 0.75rem;
		margin-bottom: 1rem;
	}
	.ltitle {
		flex: 1;
		min-width: 0;
	}
	.lhead .kicker {
		margin: 0;
	}
	.lhead h1 {
		margin: 0.1rem 0 0;
	}
	.nameedit {
		font-size: 1.5rem;
		font-weight: 700;
		margin-top: 0.15rem;
	}
	.lactions {
		display: flex;
		gap: 0.4rem;
		flex: none;
	}
	.icon {
		width: auto;
		padding: 0.6rem;
	}
	/* Prominent full-width League Chat entry, below the header actions. */
	.chatcta {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		width: 100%;
		margin: 0.2rem 0 1rem;
		padding: 0.85rem 1rem;
		background:
			linear-gradient(180deg, rgba(255, 255, 255, 0.025), transparent 40%),
			var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		color: var(--text);
		font-weight: 700;
		font-size: 1.02rem;
		transition:
			border-color 0.15s ease,
			background 0.15s ease;
	}
	.chatcta:hover {
		border-color: color-mix(in srgb, var(--accent) 45%, var(--border));
		background: color-mix(in srgb, var(--accent) 6%, var(--surface));
	}
	:global(.chatcta .cc-ico) {
		color: var(--accent);
		flex: none;
	}
	.cc-label {
		flex: 1;
	}
	.cc-badge {
		flex: none;
		min-width: 1.3rem;
		padding: 0.1rem 0.4rem;
		display: grid;
		place-items: center;
		border-radius: var(--radius-pill);
		background: var(--accent);
		color: var(--accent-fg);
		font-size: 0.78rem;
		font-weight: 800;
		font-variant-numeric: tabular-nums;
	}
	:global(.chatcta .cc-cv) {
		color: var(--muted);
		flex: none;
	}
	.chatcta:hover :global(.cc-cv) {
		color: var(--accent);
	}
	/* Header "Share" toggle: reveals the invite/share card. Filled accent when
	   the card is open so the toggle state is obvious. */
	.sharebtn {
		width: auto;
		padding: 0.6rem 0.85rem;
	}
	.sharebtn.active {
		color: var(--accent-fg);
		background: var(--accent);
		border-color: var(--accent);
	}
	.vis {
		margin-bottom: 1rem;
	}
	.vistabs {
		margin: 0.5rem 0 0;
	}
	.hint {
		margin: 0.5rem 0 0;
	}
	.lockpill {
		display: inline-flex;
		align-items: center;
		gap: 0.2rem;
		margin-left: 0.35rem;
		padding: 0.05rem 0.4rem;
		border: 1px solid var(--border);
		border-radius: 999px;
		font-size: 0.7rem;
		vertical-align: middle;
	}
	.regenbtn {
		width: auto;
		margin-top: 0.85rem;
	}
	.regwarn {
		margin-top: 0.85rem;
	}
	.btn.danger {
		width: auto;
		background: var(--danger);
		color: #fff;
		border-color: transparent;
	}
	.regrow {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.4rem;
	}
	.regrow .btn.secondary {
		width: auto;
	}
	.rmbtn {
		display: inline-grid;
		place-items: center;
		flex: none;
		padding: 0.15rem;
		background: none;
		border: none;
		color: var(--muted);
		cursor: pointer;
	}
	.rmbtn:hover:not(:disabled) {
		color: var(--danger);
	}
	.rmbtn:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.botsep td {
		padding-top: 0.9rem;
		color: var(--muted);
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		border-bottom: none;
	}
	tr.addbot .pname {
		color: var(--muted);
	}
	tr.addbot .rolepill {
		text-transform: capitalize;
	}
	.addbtn {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		margin-left: auto;
		flex: none;
		padding: 0.25rem 0.6rem;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--accent);
		font-weight: 600;
		font-size: 0.8rem;
		cursor: pointer;
	}
	.addbtn:hover:not(:disabled) {
		border-color: color-mix(in srgb, var(--accent) 40%, var(--border));
	}
	.addbtn:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.irow {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.share {
		margin-top: 0.85rem;
	}
	.ic {
		min-width: 0;
	}
	.small {
		font-size: 0.8rem;
	}
	.code {
		font-family: var(--font-mono);
		font-weight: 700;
		letter-spacing: 0.2em;
		font-size: 1.3rem;
	}
	.code.masked {
		color: var(--muted);
		letter-spacing: 0.15em;
	}
	.eye {
		width: auto;
		padding: 0.7rem;
	}
	.copy {
		width: auto;
	}
	.tabs {
		display: flex;
		gap: 0.4rem;
		margin-bottom: 0.75rem;
	}
	.tabs button {
		flex: 1;
		padding: 0.5rem;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--muted);
		font-weight: 600;
	}
	.tabs button.active {
		color: var(--accent-fg);
		background: var(--accent);
		border-color: var(--accent);
	}
	.lb {
		width: 100%;
		border-collapse: collapse;
	}
	.lb th,
	.lb td {
		text-align: left;
		padding: 0.6rem 0.4rem;
		border-bottom: 1px solid var(--border);
	}
	.lb th {
		color: var(--muted);
		font-size: 0.8rem;
		font-weight: 600;
	}
	.num {
		text-align: right;
	}
	.rank {
		width: 2rem;
		color: var(--muted);
		font-family: var(--font-mono);
	}
	tr.lead td {
		background: color-mix(in srgb, var(--accent) 9%, transparent);
	}
	tr.lead .rank {
		color: var(--accent);
		font-weight: 800;
	}
	.lb th.num,
	.lb td.num {
		text-align: right;
	}

	/* Pts is the focus — set it apart from the stat columns. */
	.lb th.pts,
	.lb td.pts {
		padding-left: 1.15rem;
		border-left: 1px solid var(--border);
		font-size: 1.02rem;
	}
	.lb th.pts {
		font-size: 0.8rem;
	}

	/* Extra tiebreaker columns: desktop only. */
	.ext {
		display: none;
	}
	.player {
		width: 100%;
	}
	.pwrap {
		display: flex;
		align-items: center;
		gap: 0.4rem;
	}
	.pname {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.rolepill {
		display: inline-flex;
		align-items: center;
		gap: 0.2rem;
		flex: none;
		padding: 0.05rem 0.4rem;
		border: 1px solid var(--border);
		border-radius: 999px;
		font-size: 0.7rem;
		color: var(--muted);
	}
	.rolepill.admin {
		color: var(--accent);
		border-color: color-mix(in srgb, var(--accent) 40%, var(--border));
	}
	.fclink {
		display: inline-grid;
		place-items: center;
		color: var(--muted);
		flex: none;
	}
	.fclink:hover {
		color: var(--accent);
	}
	:global(.lb .rx) {
		color: var(--muted);
		transition: transform 0.15s ease;
		margin-left: auto;
	}
	tr.main.open :global(.rx) {
		transform: rotate(180deg);
	}
	tr.main {
		cursor: pointer;
	}
	.detail td {
		padding: 0 0.4rem 0.7rem;
	}
	.stats {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.4rem 1rem;
	}
	.stats span {
		display: flex;
		justify-content: space-between;
		gap: 0.6rem;
		padding: 0.35rem 0;
		border-bottom: 1px solid var(--border);
	}
	.stats i {
		color: var(--muted);
		font-style: normal;
		font-size: 0.85rem;
	}
	.stats b {
		font-family: var(--font-mono);
	}

	@media (min-width: 760px) {
		.ext {
			display: table-cell;
		}
		:global(.lb .rx) {
			display: none;
		}
		tr.main {
			cursor: default;
		}
		.detail {
			display: none;
		}
	}
	.note {
		margin: 0.75rem 0 0;
	}
	.legend summary {
		cursor: pointer;
		font-weight: 700;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		font-size: 0.85rem;
		color: var(--accent);
	}
	.legend h4 {
		margin: 1rem 0 0.5rem;
		font-size: 0.95rem;
	}
	.legend .small {
		margin: 0.4rem 0 0;
	}
	ul.leg {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	ul.leg li {
		display: flex;
		align-items: baseline;
		gap: 0.75rem;
		padding: 0.4rem 0;
		border-bottom: 1px solid var(--border);
	}
	ul.leg li span {
		flex: 1;
	}
	ul.leg li b {
		font-family: var(--font-mono);
		color: var(--accent);
		white-space: nowrap;
	}
	ol.tiebreak {
		margin: 0.5rem 0 0;
		padding-left: 1.3rem;
		line-height: 1.8;
	}
	ol.tiebreak li {
		padding-left: 0.3rem;
	}
</style>
