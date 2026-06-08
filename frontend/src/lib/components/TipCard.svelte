<script lang="ts">
	import {
		tipsStore,
		isLocked,
		teamsResolved,
		type Match,
		type FriendTip,
		type PerfectScorers
	} from '$lib/tips.svelte';
	import Flag from './Flag.svelte';
	import Scoreline from './Scoreline.svelte';
	import Stepper from './Stepper.svelte';
	import { Lock, ChevronDown, Check, Users, Target } from '@lucide/svelte';

	// Controlled by the parent so only one card is open at a time (accordion).
	let {
		match,
		open = false,
		onToggle
	}: { match: Match; open?: boolean; onToggle?: () => void } = $props();

	let locked = $derived(isLocked(match));
	let resolved = $derived(teamsResolved(match));
	let home = $derived(tipsStore.team(match.homeTeam));
	let away = $derived(tipsStore.team(match.awayTeam));
	let existing = $derived(tipsStore.tips[match.id]);
	let isKO = $derived(match.stage !== 'group');
	let played = $derived(match.status === 'finished' || !!match.finalizedAt);
	let live = $derived(match.status === 'live');
	let pts = $derived(tipsStore.scores[match.id]);

	// Editable working copy.
	let ftH = $state(0);
	let ftA = $state(0);
	let etH = $state(0);
	let etA = $state(0);
	let pen = $state(''); // penalty winner team id
	let busy = $state(false);
	let msg = $state('');
	let savedOk = $state(false);

	// Seed the editor from the saved tip whenever it changes.
	$effect(() => {
		const t = tipsStore.tips[match.id];
		ftH = t?.ftHome ?? 0;
		ftA = t?.ftAway ?? 0;
		etH = t?.etHome ?? 0;
		etA = t?.etAway ?? 0;
		pen = t?.penWinner ?? '';
	});

	let ftTie = $derived(isKO && ftH === ftA);
	let etTie = $derived(ftTie && etH === etA);

	// Keep ET >= FT (cumulative) as the user edits FT.
	$effect(() => {
		if (etH < ftH) etH = ftH;
		if (etA < ftA) etA = ftA;
	});

	let advancerId = $derived(
		!isKO
			? ''
			: ftH !== ftA
				? ftH > ftA
					? match.homeTeam
					: match.awayTeam
				: etH !== etA
					? etH > etA
						? match.homeTeam
						: match.awayTeam
					: pen
	);
	let advancerName = $derived(
		advancerId ? (tipsStore.team(advancerId)?.name ?? '—') : ''
	);

	const kickoff = $derived(
		new Date(match.kickoff).toLocaleString(undefined, {
			weekday: 'short',
			day: 'numeric',
			month: 'short',
			hour: '2-digit',
			minute: '2-digit'
		})
	);

	async function save() {
		msg = '';
		savedOk = false;
		busy = true;
		try {
			await tipsStore.save({
				id: existing?.id,
				match: match.id,
				ftHome: ftH,
				ftAway: ftA,
				etHome: etH,
				etAway: etA,
				penWinner: pen,
				advancer: ''
			});
			savedOk = true;
		} catch (e: unknown) {
			msg =
				(e as { message?: string })?.message ??
				'Could not save this tip.';
		} finally {
			busy = false;
		}
	}

	// Friends' picks (only available after kickoff) — toggles open/closed.
	let friends = $state<FriendTip[] | null>(null);
	let perfect = $state<PerfectScorers | null>(null);
	let friendsBusy = $state(false);
	async function toggleFriends() {
		if (friends !== null) {
			friends = null;
			perfect = null;
			return;
		}
		friendsBusy = true;
		try {
			const r = await tipsStore.friends(match.id);
			friends = r.tips;
			perfect = r.perfect;
		} catch {
			friends = [];
			perfect = null;
		} finally {
			friendsBusy = false;
		}
	}

	function label(side: 'home' | 'away') {
		const t = side === 'home' ? home : away;
		if (t) return { name: t.name, iso2: t.iso2, code: t.fifaCode };
		const raw = side === 'home' ? match.homeLabel : match.awayLabel;
		return { name: raw, iso2: '', code: raw };
	}
	let H = $derived(label('home'));
	let A = $derived(label('away'));

	// Map an advancing team id to its side, for the Scoreline winner arrow.
	function winnerSide(advancer: string): '' | 'home' | 'away' {
		if (advancer && advancer === match.homeTeam) return 'home';
		if (advancer && advancer === match.awayTeam) return 'away';
		return '';
	}
</script>

<div class="tc card" class:locked>
	<button
		class="head"
		onclick={() => onToggle?.()}
		aria-expanded={open}
	>
		<div class="teams">
			<span class="t">
				<Flag iso2={H.iso2} code={H.code} /> <span class="tn">{H.name}</span>
			</span>
			<span class="score digits">
				{#if played || live}
					<b
						><Scoreline
							home={match.ftHome}
							away={match.ftAway}
							etHome={match.etHome}
							etAway={match.etAway}
							et={isKO && played && match.ftHome === match.ftAway}
							winner={winnerSide(match.advancer)}
						/></b
					>
				{:else if existing}
					<span class="pred"
						><Scoreline
							home={existing.ftHome}
							away={existing.ftAway}
							etHome={existing.etHome}
							etAway={existing.etAway}
							et={isKO && existing.ftHome === existing.ftAway}
							winner={winnerSide(existing.advancer)}
						/></span
					>
				{:else}
					<span class="muted">–:–</span>
				{/if}
			</span>
			<span class="t right">
				<span class="tn">{A.name}</span> <Flag iso2={A.iso2} code={A.code} />
			</span>
		</div>
		<div class="meta">
			<span class="muted"
				>{match.stage === 'group'
					? `Group ${match.groupLetter} · ${match.roundLabel}`
					: match.roundLabel} · {kickoff}</span
			>
			<span class="spacer"></span>
			{#if played}
				<span class="pill done">
					FT
					{#if pts !== undefined}
						<b class="ptv" class:ok={pts > 0}
							>{pts > 0 ? '+' : ''}{pts}&thinsp;pt</b
						>
					{/if}
				</span>
			{:else if live}
				<span class="pill livep"><span class="dot"></span> Live</span>
			{:else if locked}
				<span class="pill"><Lock size={12} /> locked</span>
			{:else if existing}
				<span class="pill ok"><Check size={12} /> tipped</span>
			{/if}
			<ChevronDown size={16} class="cv {open ? 'up' : ''}" />
		</div>
	</button>

	{#if open}
		<div class="body">
			{#if isKO && !resolved}
				<p class="muted">Opens once the matchup is decided.</p>
			{:else if locked}
				{#if existing}
					<div class="yourtip" class:scored={played}>
						<span class="ylabel">Your tip</span>
						<span class="yscore digits"
							><Scoreline
								home={existing.ftHome}
								away={existing.ftAway}
								etHome={existing.etHome}
								etAway={existing.etAway}
								et={isKO && existing.ftHome === existing.ftAway}
								winner={winnerSide(existing.advancer)}
							/></span
						>
						<span class="spacer"></span>
						{#if played && pts !== undefined}
							<span class="ypts" class:ok={pts > 0}
								>{pts > 0 ? '+' : ''}{pts} pt</span
							>
						{/if}
					</div>
				{:else}
					<p class="muted">No tip — this match was locked.</p>
				{/if}
				<button
					class="btn secondary friendsbtn"
					class:on={friends !== null}
					onclick={toggleFriends}
					disabled={friendsBusy}
				>
					<Users size={16} />
					{friends !== null ? 'Hide friends’ picks' : 'Show friends’ picks'}
				</button>
				{#if friends}
					{#if friends.length === 0}
						<p class="muted small">No friends’ tips for this match.</p>
					{:else}
						<table class="friends">
							<tbody>
								{#each friends as f (f.userId)}
									<tr>
										<td>{f.name}</td>
										<td class="num digits"
											><Scoreline
												home={f.ftHome}
												away={f.ftAway}
												etHome={f.etHome}
												etAway={f.etAway}
												et={isKO && f.ftHome === f.ftAway}
												winner={winnerSide(f.advancer)}
											/></td
										>
									</tr>
								{/each}
							</tbody>
						</table>
					{/if}
					{#if perfect && perfect.count > 0}
						<div class="exact">
							<span class="exhead">
								<Target size={14} /> Perfect tip · {perfect.points}&thinsp;pts
							</span>
							<span class="exnames">
								{perfect.names.join(', ')}{#if perfect.count > perfect.names.length}
									&nbsp;+{perfect.count - perfect.names.length} more{/if}
							</span>
						</div>
					{/if}
				{/if}
			{:else}
				<!-- Editable -->
				<div class="enter">
					<Stepper bind:value={ftH} />
					<span class="sep">:</span>
					<Stepper bind:value={ftA} />
				</div>

				{#if ftTie}
					<div class="phase">After extra time</div>
					<div class="enter">
						<Stepper bind:value={etH} min={ftH} />
						<span class="sep">:</span>
						<Stepper bind:value={etA} min={ftA} />
					</div>
				{/if}

				{#if etTie}
					<div class="phase">Penalty shootout — who advances?</div>
					<div class="pens">
						<button
							class="pen"
							class:sel={pen === match.homeTeam}
							onclick={() => (pen = match.homeTeam)}
						>
							{home?.name}
						</button>
						<button
							class="pen"
							class:sel={pen === match.awayTeam}
							onclick={() => (pen = match.awayTeam)}
						>
							{away?.name}
						</button>
					</div>
				{/if}

				{#if isKO && advancerName}
					<p class="adv muted">Advances: <b>{advancerName}</b></p>
				{/if}

				{#if msg}<p class="error">{msg}</p>{/if}
				<button class="btn" onclick={save} disabled={busy}>
					{#if savedOk}<Check size={16} /> Saved{:else}{busy
							? 'Saving…'
							: 'Save tip'}{/if}
				</button>
			{/if}
		</div>
	{/if}
</div>

<style>
	.tc {
		padding: 0;
		overflow: hidden;
	}
	.head {
		width: 100%;
		background: none;
		border: none;
		color: var(--text);
		text-align: left;
		padding: 0.85rem 1rem;
		display: block;
		cursor: pointer;
	}
	.teams {
		display: grid;
		grid-template-columns: 1fr auto 1fr;
		align-items: center;
		gap: 0.5rem;
	}
	.t {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		min-width: 0;
	}
	.t.right {
		justify-content: flex-end;
	}
	.tn {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-weight: 600;
	}
	.score b {
		font-size: 1.1rem;
	}
	.score {
		padding: 0 0.4rem;
	}
	.meta {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.5rem;
		font-size: 0.8rem;
	}
	:global(.tc .cv) {
		transition: transform 0.15s ease;
		color: var(--muted);
	}
	:global(.tc .cv.up) {
		transform: rotate(180deg);
	}
	.pill.ok {
		color: var(--success);
		border-color: var(--success);
	}
	.body {
		padding: 0.25rem 1rem 1rem;
		border-top: 1px solid var(--border);
	}
	.enter {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.6rem;
		margin: 0.8rem 0;
	}
	.sep {
		font-weight: 800;
		opacity: 0.5;
	}
	.phase {
		text-align: center;
		font-size: 0.8rem;
		color: var(--muted);
		margin-top: 0.6rem;
	}
	.pens {
		display: flex;
		gap: 0.6rem;
		margin: 0.6rem 0;
	}
	.pen {
		flex: 1;
		padding: 0.7rem;
		border-radius: var(--radius-sm);
		border: 1px solid var(--border);
		background: var(--surface-2);
		color: var(--text);
		font-weight: 600;
	}
	.pen.sel {
		background: var(--accent);
		color: var(--accent-fg);
		border-color: var(--accent);
	}
	.adv {
		text-align: center;
		margin: 0.5rem 0;
	}
	.pill.done {
		gap: 0.35rem;
		color: var(--muted);
	}
	.pill.done .ptv {
		font-family: var(--font-mono);
		font-weight: 700;
		color: var(--muted);
	}
	.pill.done .ptv.ok {
		color: var(--accent);
	}
	.pill.livep {
		color: var(--bg);
		background: var(--live);
		border-color: var(--live);
	}
	.pill.livep .dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--bg);
		animation: pulse 1.1s ease-in-out infinite;
	}
	@keyframes pulse {
		50% {
			opacity: 0.25;
		}
	}
	.score .pred {
		color: var(--muted);
		font-size: 0.95rem;
	}
	.yourtip {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.7rem 0.85rem;
		margin: 0.2rem 0 0.85rem;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-left: 3px solid var(--accent);
		border-radius: var(--radius-sm);
	}
	.yourtip.scored {
		border-left-color: var(--gold);
	}
	.ylabel {
		font-size: 0.7rem;
		font-weight: 700;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--muted);
	}
	.yscore {
		font-size: 1.25rem;
		font-weight: 800;
	}
	.ypts {
		font-family: var(--font-mono);
		font-weight: 700;
		font-size: 0.85rem;
		padding: 0.15rem 0.5rem;
		border-radius: var(--radius-pill);
		border: 1px solid var(--border);
		color: var(--muted);
	}
	.ypts.ok {
		color: var(--accent-fg);
		background: var(--accent);
		border-color: var(--accent);
	}
	.friendsbtn.on {
		border-color: var(--accent);
		color: var(--accent);
	}
	.friends {
		width: 100%;
		border-collapse: collapse;
		margin-top: 0.6rem;
	}
	.friends td {
		padding: 0.4rem 0.3rem;
		border-bottom: 1px solid var(--border);
	}
	.num {
		font-weight: 700;
	}
	.small {
		font-size: 0.85rem;
	}
	.exact {
		margin-top: 0.7rem;
		padding: 0.55rem 0.7rem;
		border: 1px solid var(--accent);
		border-radius: 0.5rem;
		background: var(--accent);
		color: var(--accent-fg);
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}
	.exhead {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		font-size: 0.85rem;
		font-weight: 600;
	}
	.exnames {
		font-size: 0.9rem;
		line-height: 1.4;
	}
</style>
