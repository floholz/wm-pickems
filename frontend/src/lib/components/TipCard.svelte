<script lang="ts">
	import {
		tipsStore,
		isLocked,
		teamsResolved,
		type Match,
		type FriendTip
	} from '$lib/tips.svelte';
	import Flag from './Flag.svelte';
	import Stepper from './Stepper.svelte';
	import { Lock, ChevronDown, Check, Users } from '@lucide/svelte';

	let { match }: { match: Match } = $props();

	let locked = $derived(isLocked(match));
	let resolved = $derived(teamsResolved(match));
	let home = $derived(tipsStore.team(match.homeTeam));
	let away = $derived(tipsStore.team(match.awayTeam));
	let existing = $derived(tipsStore.tips[match.id]);
	let isKO = $derived(match.stage !== 'group');

	let open = $state(false);

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

	// Friends' picks (only available after kickoff).
	let friends = $state<FriendTip[] | null>(null);
	let friendsBusy = $state(false);
	async function loadFriends() {
		friendsBusy = true;
		try {
			friends = await tipsStore.friends(match.id);
		} catch {
			friends = [];
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
</script>

<div class="tc card" class:locked>
	<button
		class="head"
		onclick={() => (open = !open)}
		aria-expanded={open}
	>
		<div class="teams">
			<span class="t">
				<Flag iso2={H.iso2} code={H.code} /> <span class="tn">{H.name}</span>
			</span>
			<span class="score digits">
				{#if existing}
					<b>{existing.ftHome}</b><span class="cln">:</span><b>{existing.ftAway}</b>
				{:else}
					<span class="muted">–:–</span>
				{/if}
			</span>
			<span class="t right">
				<span class="tn">{A.name}</span> <Flag iso2={A.iso2} code={A.code} />
			</span>
		</div>
		<div class="meta">
			<span class="muted">{match.roundLabel} · {kickoff}</span>
			<span class="spacer"></span>
			{#if locked}
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
					<div class="ro">
						Your tip: <b>{existing.ftHome}:{existing.ftAway}</b>
						{#if isKO && existing.advancer}
							· advances:
							<b>{tipsStore.team(existing.advancer)?.name ?? '—'}</b>
						{/if}
					</div>
				{:else}
					<p class="muted">No tip — this match is locked.</p>
				{/if}
				<button
					class="btn secondary"
					onclick={loadFriends}
					disabled={friendsBusy}
				>
					<Users size={16} />
					{friends ? 'Friends’ picks' : 'Show friends’ picks'}
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
										<td class="num">{f.ftHome}:{f.ftAway}</td>
										<td class="muted">
											{#if f.advancer}
												→ {tipsStore.team(f.advancer)?.name ?? ''}
											{/if}
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
					{/if}
				{/if}
			{:else}
				<!-- Editable -->
				<div class="enter">
					<span class="el">{H.name}</span>
					<Stepper bind:value={ftH} />
					<span class="sep">:</span>
					<Stepper bind:value={ftA} />
					<span class="el right">{A.name}</span>
				</div>

				{#if ftTie}
					<div class="phase">After extra time</div>
					<div class="enter">
						<span class="el">{H.name}</span>
						<Stepper bind:value={etH} min={ftH} />
						<span class="sep">:</span>
						<Stepper bind:value={etA} min={ftA} />
						<span class="el right">{A.name}</span>
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
	.el {
		flex: 1;
		font-weight: 600;
		font-size: 0.9rem;
	}
	.el.right {
		text-align: right;
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
	.ro {
		margin: 0.5rem 0 0.8rem;
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
</style>
