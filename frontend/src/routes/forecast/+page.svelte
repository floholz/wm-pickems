<script lang="ts">
	import { forecastStore as fs, koKey, type KOMatch } from '$lib/forecast.svelte';
	import Flag from '$lib/components/Flag.svelte';
	import { ChevronUp, ChevronDown, Lock, Check, Trophy } from '@lucide/svelte';

	let section = $state<'groups' | 'thirds' | 'bracket'>('groups');
	let busy = $state(false);
	let saved = $state(false);
	let err = $state('');

	$effect(() => {
		if (!fs.loaded) fs.load().catch((e) => (err = e?.message ?? 'load failed'));
	});

	const stages = ['R32', 'R16', 'QF', 'SF', '3RD', 'FINAL'];
	const stageName: Record<string, string> = {
		R32: 'Round of 32',
		R16: 'Round of 16',
		QF: 'Quarter-finals',
		SF: 'Semi-finals',
		'3RD': 'Third place',
		FINAL: 'Final'
	};
	let byStage = $derived(
		stages.map((s) => ({
			stage: s,
			matches: fs.knockout.filter((m) => m.stage === s)
		}))
	);

	let finalMatch = $derived(fs.knockout.find((m) => m.stage === 'FINAL'));
	let champion = $derived(
		finalMatch ? fs.bracket[koKey(finalMatch)] : ''
	);

	function tname(id: string) {
		return fs.team(id)?.name ?? '';
	}

	const lockDate = $derived(
		fs.tournamentStart
			? new Date(fs.tournamentStart).toLocaleString(undefined, {
					day: 'numeric',
					month: 'short',
					hour: '2-digit',
					minute: '2-digit'
				})
			: ''
	);

	async function save() {
		err = '';
		saved = false;
		busy = true;
		try {
			await fs.save();
			saved = true;
		} catch (e: unknown) {
			err = (e as { message?: string })?.message ?? 'Could not save.';
		} finally {
			busy = false;
		}
	}

	function sideLabel(m: KOMatch, side: 'home' | 'away') {
		const [h, a] = fs.sides(m);
		const id = side === 'home' ? h : a;
		if (id) return { id, name: tname(id), team: fs.team(id) };
		return {
			id: '',
			name: side === 'home' ? m.homeLabel : m.awayLabel,
			team: undefined
		};
	}
</script>

<p class="kicker">The big call</p>
<h1>Forecast</h1>
<p class="muted">
	Your one-time tournament call. {#if fs.locked}<b>Locked.</b>{:else}Locks at
		kickoff{lockDate ? ` · ${lockDate}` : ''}.{/if}
</p>

{#if err}<p class="error">{err}</p>{/if}

{#if !fs.loaded}
	<p class="muted">Loading…</p>
{:else}
	{#if fs.locked}
		<div class="card lockbar"><Lock size={16} /> The tournament has started — your Forecast is final.</div>
	{/if}

	<div class="seg">
		<button class:on={section === 'groups'} onclick={() => (section = 'groups')}>Groups</button>
		<button class:on={section === 'thirds'} onclick={() => (section = 'thirds')}>Best thirds</button>
		<button class:on={section === 'bracket'} onclick={() => (section = 'bracket')}>Bracket</button>
	</div>

	{#if section === 'groups'}
		<p class="muted small">Order each group 1st → 4th. Top 2 advance; 3rd may qualify as a best third.</p>
		{#each fs.groups as g (g.letter)}
			<section class="card grp">
				<h3>Group {g.letter}</h3>
				{#each fs.groupOrder[g.letter] as id, i (id)}
					<div class="trow">
						<span class="pos">{i + 1}</span>
						<Flag iso2={fs.team(id)?.iso2 ?? ''} code={fs.team(id)?.fifaCode ?? ''} />
						<span class="nm">{tname(id)}</span>
						<span class="tag">
							{#if i < 2}<span class="pill ok">advances</span>
							{:else if i === 2}<span class="pill">3rd</span>{/if}
						</span>
						{#if !fs.locked}
							<span class="ord">
								<button aria-label="up" disabled={i === 0} onclick={() => fs.move(g.letter, i, -1)}><ChevronUp size={16} /></button>
								<button aria-label="down" disabled={i === 3} onclick={() => fs.move(g.letter, i, 1)}><ChevronDown size={16} /></button>
							</span>
						{/if}
					</div>
				{/each}
			</section>
		{/each}
	{:else if section === 'thirds'}
		<p class="muted small">Pick which 3rd-placed team fills each Round-of-32 slot (from your group order).</p>
		{#each fs.thirdSlots as slot (slot.matchNum)}
			<section class="card">
				<div class="row">
					<b>Slot · groups {slot.allowed.join('/')}</b>
				</div>
				<select
					class="input"
					disabled={fs.locked}
					value={fs.thirds[String(slot.matchNum)] ?? ''}
					onchange={(e) =>
						(fs.thirds[String(slot.matchNum)] = (
							e.target as HTMLSelectElement
						).value)}
				>
					<option value="">— choose —</option>
					{#each fs.eligibleThirds(slot) as id (id)}
						<option value={id}>{tname(id)}</option>
					{/each}
				</select>
			</section>
		{/each}
	{:else}
		{#if champion}
			<div class="card champ">
				<Trophy size={20} />
				<span>Predicted champion: <b>{tname(champion)}</b></span>
			</div>
		{/if}
		{#each byStage as col (col.stage)}
			<h3 class="rname">{stageName[col.stage]}</h3>
			{#each col.matches as m (koKey(m))}
				{@const H = sideLabel(m, 'home')}
				{@const A = sideLabel(m, 'away')}
				{@const w = fs.bracket[koKey(m)]}
				<div class="bm card">
					<button
						class="bteam"
						class:win={w && w === H.id}
						disabled={fs.locked || !H.id}
						onclick={() => fs.pick(m, H.id)}
					>
						{#if H.team}<Flag iso2={H.team.iso2} code={H.team.fifaCode} />{/if}
						<span class="bn" class:ph={!H.id}>{H.name}</span>
					</button>
					<span class="vs">vs</span>
					<button
						class="bteam"
						class:win={w && w === A.id}
						disabled={fs.locked || !A.id}
						onclick={() => fs.pick(m, A.id)}
					>
						{#if A.team}<Flag iso2={A.team.iso2} code={A.team.fifaCode} />{/if}
						<span class="bn" class:ph={!A.id}>{A.name}</span>
					</button>
				</div>
			{/each}
		{/each}
	{/if}

	{#if !fs.locked}
		<div class="savebar">
			{#if err}<span class="error">{err}</span>{/if}
			<button class="btn" onclick={save} disabled={busy}>
				{#if saved}<Check size={16} /> Saved{:else}{busy
						? 'Saving…'
						: 'Save Forecast'}{/if}
			</button>
		</div>
	{/if}
{/if}

<style>
	h1 {
		margin: 0.25rem 0 0.2rem;
	}
	.small {
		font-size: 0.85rem;
	}
	.lockbar {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		color: var(--warning);
	}
	.seg {
		display: flex;
		gap: 0.4rem;
		margin: 1rem 0;
		position: sticky;
		top: var(--topbar-h);
		z-index: 10;
	}
	.seg button {
		flex: 1;
		padding: 0.5rem;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--muted);
		font-weight: 600;
		font-size: 0.85rem;
	}
	.seg button.on {
		background: var(--accent);
		color: var(--accent-fg);
		border-color: var(--accent);
	}
	.grp h3 {
		margin: 0 0 0.6rem;
	}
	.trow {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.45rem 0;
		border-top: 1px solid var(--border);
	}
	.trow:nth-child(2) {
		border-top: none;
	}
	.pos {
		width: 1.2rem;
		text-align: center;
		font-weight: 800;
		color: var(--muted);
	}
	.nm {
		flex: 1;
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.pill.ok {
		color: var(--success);
		border-color: var(--success);
	}
	.ord button {
		background: var(--surface-2);
		border: 1px solid var(--border);
		color: var(--accent);
		border-radius: 7px;
		width: 30px;
		height: 26px;
		margin-left: 2px;
	}
	.ord button:disabled {
		color: var(--muted);
		opacity: 0.5;
	}
	.rname {
		margin: 1.2rem 0 0.5rem;
		color: var(--muted);
		font-size: 0.95rem;
	}
	.bm {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 0.7rem;
	}
	.bm + .bm {
		margin-top: 0.5rem;
	}
	.bteam {
		flex: 1;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.55rem 0.6rem;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--text);
		min-width: 0;
	}
	.bteam:disabled {
		opacity: 0.7;
	}
	.bteam.win {
		background: var(--accent);
		border-color: var(--accent);
		color: var(--accent-fg);
	}
	.bn {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-weight: 600;
		font-size: 0.9rem;
	}
	.bn.ph {
		color: var(--muted);
		font-weight: 500;
	}
	.vs {
		color: var(--muted);
		font-size: 0.8rem;
	}
	.champ {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		color: var(--accent-2);
	}
	.savebar {
		position: sticky;
		bottom: calc(var(--nav-h) + 0.5rem);
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-top: 1.5rem;
	}
	.savebar .btn {
		width: auto;
		flex: 1;
	}
</style>
