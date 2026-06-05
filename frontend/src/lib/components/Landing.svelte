<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Confetti } from 'svelte-confetti';
	import { auth } from '$lib/auth.svelte';
	import { countdown } from '$lib/countdown.svelte';
	import Countdown from './Countdown.svelte';
	import {
		Telescope,
		Volleyball,
		Trophy,
		Users,
		ArrowRight,
		Gift,
		Ban,
		Code,
		Lock,
		Target,
		Check,
		Sparkles,
		ChevronUp,
		ChevronDown,
		Minus,
		Plus
	} from '@lucide/svelte';

	// The landing is only mounted for signed-out visitors, but keep the primary
	// CTA honest if an authed user ever lands here (e.g. via a stale link).
	let primaryHref = $derived(auth.isAuthed ? '/' : '/register');
	let primaryLabel = $derived(auth.isAuthed ? 'Back to the app' : 'Create free account');

	// The hero headline rolls "friends" through these (drop in from the top).
	// Keep the longest one as the width sizer (.roll-size) below.
	const rollWords = ['friends.', 'colleagues.', 'family.', 'team.', 'coworkers.', 'rivals.'];

	const why = [
		{
			icon: Gift,
			title: 'Free',
			body: 'Every feature, every match. No premium tier, no paywall, no catch.'
		},
		{
			icon: Ban,
			title: 'No ads',
			body: 'Not a single banner or tracker. Your data stays yours — we just keep score.'
		},
		{
			icon: Code,
			title: 'Open source',
			body: 'Built in the open under GPLv3. Read it, host it, fork it yourself.'
		}
	];

	// Per-match scoring — mirrors the live config legend (max 6 / game).
	const tipPoints = [
		{ label: 'Correct result', pts: '3' },
		{ label: 'Exact score', pts: '+1' },
		{ label: 'Total goals', pts: '+1' },
		{ label: 'Goal difference', pts: '+1' }
	];
	// Broadcast lower-third ticker — repeated enough that one copy overflows the
	// widest container, so the two identical copies scroll seamlessly (-50%).
	const ticker = ['Free', 'No ads', 'Open source'];
	const tickerRun = Array.from({ length: 5 }, () => ticker).flat();

	// Forecast group-stage scoring.
	const groups = [
		{ r: 'Position', p: '1' },
		{ r: 'Advancer', p: '+1' },
		{ r: 'Perfect', p: '+2' }
	];
	// Forecast knockout-reach escalation.
	const reach = [
		{ r: 'R32', p: '1' },
		{ r: 'R16', p: '2' },
		{ r: 'QF', p: '3' },
		{ r: 'SF', p: '5' },
		{ r: 'Final', p: '8' },
		{ r: 'Champ', p: '13' }
	];

	// ---- Live in-app demos (mirror the real forecast/tips UI) -------------
	// Forecast group orderer — reorder with the chevrons; top two advance, 3rd
	// goes to the best-third pool. Flags for non-qualifiers live in /flags/more.
	let groupOrder = $state([
		{ name: 'Austria', flag: '/flags/at.svg', code: 'AUT' },
		{ name: 'Rwanda', flag: '/flags/more/rw.svg', code: 'RWA' },
		{ name: 'Bolivia', flag: '/flags/more/bo.svg', code: 'BOL' },
		{ name: 'Italy', flag: '/flags/more/it.svg', code: 'ITA' }
	]);
	function moveTeam(i: number, dir: number) {
		const j = i + dir;
		if (j < 0 || j >= groupOrder.length) return;
		const next = [...groupOrder];
		[next[i], next[j]] = [next[j], next[i]];
		groupOrder = next;
	}

	// Knockout matchup — the group's top two carry over; click one to send it
	// through. Tracked by slot so it stays valid as the group reorders.
	let koPick = $state('first');

	// Match tip — the steppers stage an edit; the header scoreline only updates
	// once you hit Save, mirroring the real editor.
	let tipH = $state(1);
	let tipA = $state(0);
	let savedH = $state(1);
	let savedA = $state(0);
	let savedFlash = $state(false);
	// Easter-egg confetti. Each qualifying save appends a burst anchored to the
	// Save button's on-screen centre — a fixed layer, so it escapes the cards'
	// overflow clip. Bursts stack rather than cancelling each other; a single
	// idle timeout (reset on every save) tears the whole layer down afterwards.
	const BURST_MS = 2000; // ≈ piece duration; layer is torn down this long after the last save
	const MAX_BURSTS = 2; // cap concurrent bursts so spamming Save can't tank the framerate
	let saveBtn: HTMLButtonElement;
	let bursts: { id: number; x: number; y: number }[] = $state([]);
	let burstSeq = 0;
	let clearBursts: ReturnType<typeof setTimeout>;
	function bumpTip(side: 'h' | 'a', d: number) {
		if (side === 'h') tipH = Math.max(0, Math.min(99, tipH + d));
		else tipA = Math.max(0, Math.min(99, tipA + d));
	}
	function saveTip(e?: MouseEvent) {
		savedH = tipH;
		savedA = tipA;
		savedFlash = true;
		setTimeout(() => (savedFlash = false), 1400);
		// Easter egg: Brazil 1 : Germany 7 — the 2014 semi-final. 🇧🇷🇩🇪
		if (tipH === 1 && tipA === 7) {
			// Burst from the actual click point (the .party layer is fixed, so use
			// viewport coords). Keyboard-activated clicks report 0,0 — fall back to
			// the button's centre then.
			let x: number, y: number;
			if (e && (e.clientX || e.clientY)) {
				x = e.clientX;
				y = e.clientY;
			} else {
				const r = saveBtn?.getBoundingClientRect();
				x = r ? r.left + r.width / 2 : 0;
				y = r ? r.top + r.height / 2 : 0;
			}
			bursts = [...bursts, { id: burstSeq++, x, y }].slice(-MAX_BURSTS);
			clearTimeout(clearBursts);
			clearBursts = setTimeout(() => (bursts = []), BURST_MS + 400);
		}
	}

	// Countdown to the lock (first kickoff). The sticky top bar shows once the
	// hero countdown scrolls out of view; everything hides once locked.
	let heroCdEl: HTMLElement | undefined = $state();
	let showBar = $state(false);
	onMount(() => {
		countdown.start();
	});
	onDestroy(() => countdown.stop());
	$effect(() => {
		const el = heroCdEl;
		if (!el) return;
		const io = new IntersectionObserver(([entry]) => (showBar = !entry.isIntersecting), {
			threshold: 0
		});
		io.observe(el);
		return () => io.disconnect();
	});
</script>

<div class="land stagger">
	<!-- ============ HERO ============ -->
	<header class="hero">
		<p class="kicker hero-kick">
			<span>FIFA World Cup 2026</span><span class="dates"
				>11 Jun – 19 Jul <span class="flags">· 🇨🇦 🇲🇽 🇺🇸</span></span
			>
		</p>
		<h1 class="head">
			Predict the <span class="wm"
				>WM<span class="wm-note"
					><span class="wm-arrow" aria-hidden="true"></span><span class="wm-note-text"
						>abbr. <em>„Weltmeisterschaft“</em> — German for World&nbsp;Cup</span
					></span
				></span
			>.<br /><span class="grad"
				>Beat your <span class="roll" aria-hidden="true"
					><span class="roll-size">colleagues.</span><span class="drum"
						>{#each rollWords as w, i (w)}<b class="face" style="--i:{i}"
								><span>{w}</span></b
							>{/each}</span
					></span
				><span class="sr-only">friends.</span></span
			>
		</h1>
		<p class="tldr">
			<span class="tl">TL;DR</span>
			A free prediction game for the World Cup. Call the whole tournament once,
			tip every single match, and climb private leaderboards with your mates.
		</p>

		{#if countdown.ready && !countdown.locked}
			<div class="hero-cd" bind:this={heroCdEl}>
				<Countdown variant="hero" />
			</div>
		{/if}

		<div class="cta">
			<a class="btn big" href={primaryHref}>{primaryLabel} <ArrowRight size={18} /></a>
			{#if !auth.isAuthed}
				<a class="btn secondary big" href="/login">Sign in</a>
			{/if}
		</div>

		<dl class="stats digits">
			<div><dt>48</dt><dd>Nations</dd></div>
			<div><dt>104</dt><dd>Matches</dd></div>
			<div><dt>12</dt><dd>Groups</dd></div>
			<div><dt>1</dt><dd>Winner</dd></div>
		</dl>
	</header>

	<!-- ============ BROADCAST MARQUEE ============ -->
	<div class="marquee" aria-hidden="true">
		<div class="track">
			{#each [0, 1] as copy (copy)}
				<span class="run">
					{#each tickerRun as t, k (k)}
						<b>{t}</b><i>·</i>
					{/each}
				</span>
			{/each}
		</div>
	</div>

	<!-- ============ WHY ============ -->
	<section class="block">
		<p class="kicker">Why this app</p>
		<h2>No money. No ads. No nonsense.</h2>
		<div class="grid3">
			{#each why as w (w.title)}
				{@const Icon = w.icon}
				<div class="card why">
					<span class="ic"><Icon size={22} /></span>
					<div class="why-txt">
						<h3>{w.title}</h3>
						<p class="muted">{w.body}</p>
					</div>
				</div>
			{/each}
		</div>
	</section>

	<!-- ============ TWO MODES ============ -->
	<section class="block">
		<p class="kicker">Two ways to play</p>
		<h2>One big call. <span class="grad">104 small ones.</span></h2>
		<div class="modes">
			<!-- Forecast: copy + live group/knockout pickers -->
			<div class="mode-row card">
				<div class="mode-copy">
					<span class="ic"><Telescope size={22} /></span>
					<h3>Forecast</h3>
					<p class="muted">
						One pre-tournament prediction: full group standings 1–4, the eight
						best-third qualifiers and the entire knockout bracket. Locks at the
						opening kickoff — then scores tick in stage by stage.
					</p>
					<span class="pill ok"><Lock size={13} /> Locks at first kickoff</span>
				</div>
				<div class="mode-demo">
					<div class="gdemo card">
						<p class="glabel">Groups</p>
						{#each groupOrder as t, i (t.name)}
							<div class="grow">
								<span class="gpos digits">{i + 1}</span>
								<img class="gflag" src={t.flag} alt={t.code} />
								<span class="gnm">{t.name}</span>
								<span class="gtag">
									{#if i < 2}<span class="pill ok">advances</span>
									{:else if i === 2}<span class="pill">3rd</span>{/if}
								</span>
								<span class="gord">
									<button
										aria-label="move {t.name} up"
										disabled={i === 0}
										onclick={() => moveTeam(i, -1)}><ChevronUp size={16} /></button
									>
									<button
										aria-label="move {t.name} down"
										disabled={i === 3}
										onclick={() => moveTeam(i, 1)}><ChevronDown size={16} /></button
									>
								</span>
							</div>
						{/each}
					</div>
					<div class="kdemo card">
						<p class="glabel">Knockout</p>
						<div class="kmatch">
							<button
								class="kteam"
								class:win={koPick === 'first'}
								onclick={() => (koPick = 'first')}
							>
								<img class="gflag" src={groupOrder[0].flag} alt={groupOrder[0].code} />
								<span class="kn">{groupOrder[0].name}</span>
							</button>
							<span class="kvs">vs</span>
							<button
								class="kteam"
								class:win={koPick === 'second'}
								onclick={() => (koPick = 'second')}
							>
								<img class="gflag" src={groupOrder[1].flag} alt={groupOrder[1].code} />
								<span class="kn">{groupOrder[1].name}</span>
							</button>
						</div>
					</div>
				</div>
			</div>

			<!-- Tips: copy + live score tip -->
			<div class="mode-row reverse card">
				<div class="mode-copy">
					<span class="ic alt"><Volleyball size={22} /></span>
					<h3>Tips</h3>
					<p class="muted">
						Predict the score of every match, editable right up to kickoff.
						Knockouts go deeper — 90′, extra time, then penalties. Once a game
						starts your tip locks and you can see what everyone else picked.
					</p>
					<span class="pill"><Check size={13} /> Editable until kickoff</span>
				</div>
				<div class="mode-demo">
					<div class="tdemo card">
						<div class="thead">
							<div class="tteams">
								<span class="tt">
									<img class="gflag" src="/flags/br.svg" alt="BRA" />
									<span class="ttn">Brazil</span>
								</span>
								<span class="tsc digits"
									><span class="pred">{savedH}<span class="cln">:</span>{savedA}</span></span
								>
								<span class="tt right">
									<span class="ttn">Germany</span>
									<img class="gflag" src="/flags/de.svg" alt="GER" />
								</span>
							</div>
							<div class="tmeta">
								<span class="muted">Group F · Matchday 1 · Sat, Jun 13, 6:00 PM</span>
								<span class="tspacer"></span>
								<span class="pill ok"><Check size={12} /> tipped</span>
								<ChevronUp size={16} class="tcv" />
							</div>
						</div>
						<div class="tbody">
							<div class="tenter">
								<span class="tstep">
									<button aria-label="Brazil minus" onclick={() => bumpTip('h', -1)}
										><Minus size={16} /></button
									>
									<span class="tval digits">{tipH}</span>
									<button aria-label="Brazil plus" onclick={() => bumpTip('h', 1)}
										><Plus size={16} /></button
									>
								</span>
								<span class="tsep">:</span>
								<span class="tstep">
									<button aria-label="Germany minus" onclick={() => bumpTip('a', -1)}
										><Minus size={16} /></button
									>
									<span class="tval digits">{tipA}</span>
									<button aria-label="Germany plus" onclick={() => bumpTip('a', 1)}
										><Plus size={16} /></button
									>
								</span>
							</div>
							<button class="btn tsave" bind:this={saveBtn} onclick={saveTip}>
								{#if savedFlash}<Check size={16} /> Saved{:else}Save tip{/if}
							</button>
						</div>
					</div>
				</div>
			</div>
		</div>
	</section>

	<!-- ============ LEAGUES ============ -->
	<section class="block">
		<p class="kicker">Bragging rights</p>
		<h2>Play in leagues.</h2>
		<div class="card leagues">
			<div class="lg-copy">
				<p class="muted">
					Spin up a private competition and share a join code with your friends.
					Each league has its own leaderboard with
					<b>Overall</b>, <b>Tips</b> and <b>Forecast</b> tables, full tiebreaker
					stats and a built-in scoring legend. Everyone's auto-entered into a
					global league too — so you're never playing alone.
				</p>
				<div class="lg-tags">
					<span class="pill"><Users size={13} /> Private groups</span>
					<span class="pill"><Trophy size={13} /> Live leaderboards</span>
					<span class="pill"><Sparkles size={13} /> Bot opponents</span>
				</div>
			</div>
			<div class="invite" aria-hidden="true">
				<span class="invite-lbl">Join code</span>
				<span class="invite-code digits">WC-2026</span>
			</div>
		</div>
	</section>

	<!-- ============ POINTS ============ -->
	<section class="block">
		<p class="kicker">How points work</p>
		<h2>Six points a game. <span class="grad">Max.</span></h2>
		<div class="pts-grid">
			<div class="card score tips">
				<div class="score-head">
					<span class="pill">Per match</span>
					<span class="max digits">6<small>max</small></span>
				</div>
				<ul class="score-list">
					{#each tipPoints as t (t.label)}
						<li><span>{t.label}</span><b class="digits">{t.pts}</b></li>
					{/each}
				</ul>
				<p class="muted fine">
					<Target size={13} /> Knockout score points use the after-extra-time result.
				</p>
			</div>
			<div class="card score">
				<div class="score-head">
					<span class="pill ok">Forecast groups</span>
					<span class="muted fine">each correct call</span>
				</div>
				<div class="reach">
					{#each groups as g (g.r)}
						<div class="rstep">
							<span class="rp digits">{g.p}</span>
							<span class="rr">{g.r}</span>
						</div>
					{/each}
				</div>
				<p class="muted fine">
					<Sparkles size={13} /> Each team in its right slot, plus every predicted
					advancer — a whole group nailed earns the bonus.
				</p>
			</div>
			<div class="card score">
				<div class="score-head">
					<span class="pill ok">Forecast reach</span>
					<span class="muted fine">per team that gets there</span>
				</div>
				<div class="reach">
					{#each reach as r (r.r)}
						<div class="rstep">
							<span class="rp digits">{r.p}</span>
							<span class="rr">{r.r}</span>
						</div>
					{/each}
				</div>
				<p class="muted fine">
					<Trophy size={13} /> Climbs each round a predicted team keeps surviving.
				</p>
			</div>
		</div>
	</section>

	<!-- ============ FINAL CTA ============ -->
	<section class="block final">
		<div class="card cta-card">
			<h2>Kickoff is coming.</h2>
			<p class="muted">Make your picks before the rest of the group does.</p>
			<Countdown variant="cta" />
			<div class="cta">
				<a class="btn big" href={primaryHref}>{primaryLabel} <ArrowRight size={18} /></a>
				{#if !auth.isAuthed}
					<a class="btn secondary big" href="/login">I already have an account</a>
				{/if}
			</div>
		</div>
		<p class="foot muted">
			WM Tips · open source under GPLv3 · made for the love of the game, not your data.
		</p>
	</section>
</div>

<!-- Sticky lock countdown — slides in once the hero countdown scrolls away. -->
{#if showBar && countdown.ready && !countdown.locked}
	<div class="cd-stickybar"><Countdown variant="bar" /></div>
{/if}

<!-- Easter egg: saving Brazil 1 : Germany 7 pops confetti + trophies from the button. -->
{#each bursts as b (b.id)}
	<div class="party" aria-hidden="true" style="left:{b.x}px; top:{b.y}px;">
		<Confetti
			x={[-0.75, 0.75]}
			y={[-0.5, 0.5]}
			fallDistance="20px"
			amount={150}
			duration={2000}
			colorArray={['var(--accent)', 'var(--accent-2)', '#ffcf3a', '#ff5a36', '#ffffff']}
			destroyOnComplete
		/>
		<Confetti
			x={[-1.5, 1.5]}
			y={[0.3, 1.2]}
			fallDistance="20px"
			amount={20}
			size={32}
			duration={2000}
			colorArray={['url(/assets/wc_trophy.svg) center / contain no-repeat']}
			destroyOnComplete
		/>
	</div>
{/each}

<style>
	@import url('https://fonts.googleapis.com/css2?family=Caveat:wght@600;700&display=swap');

	.land {
		max-width: 960px;
		margin: 0 auto;
		padding-bottom: 2rem;
	}

	/* ---------- HERO ---------- */
	.hero {
		position: relative;
		padding: clamp(1.5rem, 5vw, 3rem) 0 1.5rem;
	}

	/* The "WM" word + a hand-scrawled note explaining the German abbreviation.
	   The note is anchored to .wm so it tracks the word across breakpoints. */
	.wm {
		position: relative;
		color: var(--accent);
	}
	.wm-note {
		position: absolute;
		left: calc(100% + 6.2rem);
		top: -0.7rem;
		width: 11.5rem;
		pointer-events: none;
		text-transform: none;
		font-family: 'Caveat', cursive;
		font-weight: 600;
		font-size: 1.45rem;
		line-height: 1.02;
		letter-spacing: 0;
		color: var(--muted);
		transform: rotate(-3deg);
	}
	.wm-note em {
		font-style: normal;
		color: var(--text);
	}
	/* Hand-drawn arrow, tinted via mask, sweeping down-left from the note to WM. */
	.wm-arrow {
		position: absolute;
		left: -5.4rem;
		top: 0.7rem;
		width: 5.2rem;
		height: 3.75rem; /* arrow-vector aspect 398:287 */
		background: var(--muted);
		-webkit-mask: url('/assets/arrow-vector.svg') left bottom / contain no-repeat;
		mask: url('/assets/arrow-vector.svg') left bottom / contain no-repeat;
		transform: rotate(4deg);
	}
	.wm-note-text {
		display: block;
	}
	/* Below desktop: no room beside WM, so anchor the note to the hero (.wm goes
	   static) and tuck it in the empty space lower-right, arrow flipped to point up. */
	@media (max-width: 900px) {
		.wm {
			position: static;
		}
		.wm-note {
			left: auto;
			right: -1.6rem;
			top: 2rem;
			width: 9.5rem;
			font-size: 0.8rem;
			transform: rotate(2deg);
		}
		.wm-arrow {
			left: 1.9rem;
			top: 0.1rem;
			width: 2.6rem;
			transform: rotate(341deg);
		}
	}
	.flags {
		white-space: nowrap;
	}
	/* Desktop: one line — a dot separates the title from the dates. */
	.hero-kick .dates::before {
		content: '·';
		margin: 0 0.5em;
	}
	/* Mobile: break into two lines — title, then dates · flags (no separator). */
	@media (max-width: 600px) {
		.hero-kick .dates {
			display: block;
			margin-top: 0.25rem;
		}
		.hero-kick .dates::before {
			display: none;
		}
	}
	.head {
		font-size: clamp(2.6rem, 11vw, 5.5rem);
		line-height: 0.92;
		margin: 0.6rem 0 0;
	}
	.grad {
		background: linear-gradient(100deg, var(--accent) 10%, var(--accent-2) 90%);
		-webkit-background-clip: text;
		background-clip: text;
		color: transparent;
	}

	/* Rolling word: a hexagonal "drum" whose six faces each carry one word and
	   that rotates on the X-axis, flipping to the next word. .roll-size (longest
	   word) fixes the slot width so every face left-aligns under it; the trailing
	   space sits at the line end, so it's invisible. */
	.roll {
		--n: 6; /* number of faces (rollWords.length) */
		--slot: 2.4s; /* time each word rests facing the viewer */
		--theta: 60deg; /* 360 / n */
		--r: 0.866em; /* prism apothem = (faceHeight/2) / tan(theta/2) */
		position: relative;
		display: inline-block;
		height: 1em;
		line-height: 1;
		overflow: hidden;
		perspective: 9em;
		vertical-align: bottom;
		text-align: left;
	}
	.roll-size {
		display: block;
		visibility: hidden;
		height: 1em;
		line-height: 1;
	}
	.drum {
		position: absolute;
		inset: 0;
		transform-style: preserve-3d;
		/* translateZ(-r) keeps the front face on the screen plane (natural size) */
		transform: translateZ(calc(var(--r) * -1));
		animation: drum calc(var(--n) * var(--slot)) infinite;
	}
	.face {
		position: absolute;
		inset: 0;
		height: 1em;
		line-height: 1;
		font-weight: inherit;
		backface-visibility: hidden;
		transform: rotateX(calc(var(--i) * var(--theta))) translateZ(var(--r));
	}
	.face > span {
		display: block;
		white-space: nowrap;
		background: linear-gradient(100deg, var(--accent) 10%, var(--accent-2) 90%);
		-webkit-background-clip: text;
		background-clip: text;
		color: transparent;
	}
	/* Hold on each face, then a quick flip to the next (-60° per step). The drum
	   keeps the constant translateZ(-r) so the front face stays full-size. */
	@keyframes drum {
		0%, 13% { transform: translateZ(calc(var(--r) * -1)) rotateX(0deg); }
		16.6%, 29.6% { transform: translateZ(calc(var(--r) * -1)) rotateX(-60deg); }
		33.3%, 46.3% { transform: translateZ(calc(var(--r) * -1)) rotateX(-120deg); }
		50%, 63% { transform: translateZ(calc(var(--r) * -1)) rotateX(-180deg); }
		66.6%, 79.6% { transform: translateZ(calc(var(--r) * -1)) rotateX(-240deg); }
		83.3%, 96.3% { transform: translateZ(calc(var(--r) * -1)) rotateX(-300deg); }
		100% { transform: translateZ(calc(var(--r) * -1)) rotateX(-360deg); }
	}
	@media (prefers-reduced-motion: reduce) {
		.drum {
			animation: none;
			transform: translateZ(calc(var(--r) * -1)) rotateX(0deg);
		}
	}
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}
	.tldr {
		max-width: 46ch;
		margin: 1.1rem 0 0;
		color: var(--muted);
		font-size: 1.02rem;
		line-height: 1.55;
	}
	.tl {
		display: inline-block;
		margin-right: 0.5rem;
		padding: 0.1rem 0.45rem;
		border-radius: var(--radius-sm);
		background: var(--accent);
		color: var(--accent-fg);
		font-weight: 800;
		font-size: 0.7rem;
		letter-spacing: 0.08em;
		vertical-align: 1.5px;
	}
	.hero-cd {
		margin-top: 1.5rem;
	}
	.cta {
		display: flex;
		flex-wrap: wrap;
		gap: 0.7rem;
		margin-top: 1.5rem;
	}
	.btn.big {
		width: auto;
		padding: 0.95rem 1.4rem;
		font-size: 1rem;
	}
	.stats {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 0.5rem;
		margin: 2rem 0 0;
		padding: 1rem 0 0;
		border-top: 1px solid var(--border);
	}
	.stats div {
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
	}
	.stats dt {
		font-size: clamp(1.6rem, 5vw, 2.4rem);
		font-weight: 700;
		line-height: 1;
		color: var(--text);
	}
	.stats dd {
		margin: 0;
		font-size: 0.7rem;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--muted);
	}

	/* ---------- MARQUEE ---------- */
	.marquee {
		position: relative;
		margin: 1.5rem 0;
		padding: 0.55rem 0;
		border-top: 1px solid var(--border);
		border-bottom: 1px solid var(--border);
		background: linear-gradient(90deg, var(--accent) 0%, var(--accent-2) 100%);
		overflow: hidden;
	}
	.track {
		display: flex;
		width: max-content;
		animation: scroll 26s linear infinite;
	}
	.run {
		display: inline-flex;
		align-items: center;
		gap: 0.9rem;
		padding-right: 0.9rem;
		white-space: nowrap;
		color: var(--accent-ink);
		font-family: var(--font-display);
		font-size: 1.05rem;
		text-transform: uppercase;
		letter-spacing: 0.02em;
	}
	.run i {
		font-style: normal;
		opacity: 0.55;
	}
	@keyframes scroll {
		to {
			transform: translateX(-50%);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.track {
			animation: none;
		}
	}

	/* ---------- BLOCKS ---------- */
	.block {
		margin: clamp(2.5rem, 7vw, 4rem) 0 0;
	}
	.block h2 {
		margin: 0.35rem 0 1.2rem;
	}
	.grid3 {
		display: grid;
		gap: 0.85rem;
	}
	/* These layouts are grids spaced by `gap`; cancel the global stacked-card
	   `.card + .card` top margin so grid cells stay equal height and aligned. */
	.grid3 > .card + .card,
	.pts-grid > .card + .card {
		margin-top: 0;
	}
	@media (min-width: 720px) {
		.grid3 {
			grid-template-columns: repeat(3, 1fr);
		}
	}

	.ic {
		display: grid;
		place-items: center;
		width: 44px;
		height: 44px;
		flex: none;
		border-radius: var(--radius-sm);
		background: var(--surface-2);
		color: var(--accent);
		margin-bottom: 0.7rem;
	}
	.ic.alt {
		color: var(--accent-2);
	}
	/* Why cards read as [icon][text] rows — compact on mobile, no dead space
	   on desktop. */
	.why {
		display: flex;
		align-items: flex-start;
		gap: 0.9rem;
	}
	.why .ic {
		margin-bottom: 0;
	}
	.why h3 {
		margin-bottom: 0.3rem;
	}
	.why p {
		font-size: 0.92rem;
		line-height: 1.5;
		margin: 0;
	}

	/* ---------- TWO MODES ---------- */
	/* Each mode is a copy column beside a live demo of the real picker UI.
	   On desktop the Tips row flips so the demos zig-zag down the page. */
	.modes {
		display: flex;
		flex-direction: column;
		gap: 0.85rem;
	}
	/* Rows are spaced by the flex `gap`; cancel the global stacked-card margin. */
	.modes > .card + .card {
		margin-top: 0;
	}
	.mode-row {
		display: grid;
		gap: 1.25rem;
	}
	@media (min-width: 760px) {
		.mode-row {
			grid-template-columns: 1fr 1fr;
			gap: 2rem;
			align-items: center;
		}
		.mode-row.reverse .mode-copy {
			order: 2;
		}
	}
	.mode-copy {
		display: flex;
		flex-direction: column;
		/* sit at the top of the row rather than centering against a taller demo */
		align-self: start;
	}
	.mode-copy h3 {
		margin-bottom: 0.4rem;
	}
	.mode-copy p {
		font-size: 0.92rem;
		line-height: 1.5;
		margin: 0;
	}
	.mode-copy .pill {
		align-self: flex-start;
		margin-top: 0.9rem;
	}
	.mode-copy .pill.ok {
		color: var(--accent);
		border-color: color-mix(in srgb, var(--accent) 45%, var(--border));
	}

	/* ---------- LIVE DEMOS ---------- */
	.mode-demo {
		display: flex;
		flex-direction: column;
		gap: 0.7rem;
	}
	/* Demos are cards nested inside the mode card — tighter radius so the
	   nesting reads as intentional, not card-on-card. */
	.mode-demo .card {
		margin-top: 0;
		border-radius: var(--radius-sm);
	}
	/* shared flag chip — mirrors Flag.svelte */
	.gflag {
		width: 22px;
		height: 16px;
		flex: none;
		object-fit: cover;
		border-radius: 3px;
		border: 1px solid var(--border);
	}

	/* group orderer (.trow in the real forecast page) */
	.gdemo {
		padding: 0.85rem 1rem;
	}
	.glabel {
		margin: 0 0 0.3rem;
		font-weight: 800;
		font-size: 1.05rem;
	}
	.grow {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.45rem 0;
	}
	.grow + .grow {
		border-top: 1px solid var(--border);
	}
	.gpos {
		width: 1.2rem;
		text-align: center;
		font-weight: 800;
		color: var(--muted);
	}
	.gnm {
		flex: 1;
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.gtag {
		display: flex;
		align-items: center;
	}
	.gtag .pill.ok {
		color: var(--accent);
		border-color: color-mix(in srgb, var(--accent) 55%, var(--border));
	}
	.gord {
		display: flex;
		gap: 2px;
	}
	.gord button {
		display: grid;
		place-items: center;
		width: 30px;
		height: 26px;
		background: var(--surface-2);
		border: 1px solid var(--border);
		color: var(--accent);
		border-radius: 7px;
	}
	.gord button:disabled {
		color: var(--muted);
		opacity: 0.5;
	}

	/* knockout matchup (.bm in the real forecast page) */
	.kdemo {
		padding: 0.85rem 1rem;
	}
	.kmatch {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.kteam {
		flex: 1;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		min-width: 0;
		padding: 0.55rem 0.6rem;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--text);
	}
	.kteam.win {
		background: var(--accent);
		border-color: var(--accent);
		color: var(--accent-fg);
	}
	.kteam.win .gflag {
		border-color: color-mix(in srgb, var(--accent-fg) 30%, transparent);
	}
	.kn {
		font-weight: 600;
		font-size: 0.9rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.kvs {
		color: var(--muted);
		font-size: 0.8rem;
	}

	/* match tip card (TipCard.svelte, expanded) */
	.tdemo {
		padding: 0;
		overflow: hidden;
	}
	.thead {
		padding: 0.85rem 1rem;
	}
	.tteams {
		display: grid;
		grid-template-columns: 1fr auto 1fr;
		align-items: center;
		gap: 0.5rem;
	}
	.tt {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		min-width: 0;
	}
	.tt.right {
		justify-content: flex-end;
	}
	.ttn {
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.tsc {
		padding: 0 0.4rem;
	}
	.tsc .pred {
		color: var(--muted);
		font-size: 0.95rem;
		font-weight: 700;
	}
	.cln {
		opacity: 0.6;
	}
	.tmeta {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.5rem;
		font-size: 0.8rem;
	}
	.tspacer {
		flex: 1;
	}
	.tmeta .pill.ok {
		color: var(--accent);
		border-color: color-mix(in srgb, var(--accent) 45%, var(--border));
	}
	:global(.tdemo .tcv) {
		color: var(--muted);
		flex: none;
	}
	.tbody {
		padding: 0.25rem 1rem 1rem;
		border-top: 1px solid var(--border);
	}
	.tenter {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.6rem;
		margin: 0.8rem 0;
	}
	.tstep {
		display: inline-flex;
		align-items: center;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: var(--radius-pill);
	}
	.tstep button {
		display: grid;
		place-items: center;
		width: 34px;
		height: 34px;
		background: none;
		border: none;
		color: var(--accent);
	}
	.tval {
		min-width: 1.6rem;
		text-align: center;
		font-weight: 800;
		font-size: 1.05rem;
	}
	.tsep {
		font-weight: 800;
		opacity: 0.5;
	}
	.tsave {
		width: 100%;
	}

	/* ---------- LEAGUES ---------- */
	.leagues {
		display: flex;
		flex-wrap: wrap;
		gap: 1.25rem;
		align-items: center;
		justify-content: space-between;
	}
	.lg-copy {
		flex: 1 1 320px;
	}
	.lg-copy p {
		margin: 0 0 0.9rem;
		line-height: 1.55;
	}
	.lg-tags {
		display: flex;
		flex-wrap: wrap;
		gap: 0.45rem;
	}
	.invite {
		flex: 1 1 200px;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.4rem;
		padding: 1.4rem 1rem;
		border: 1px dashed color-mix(in srgb, var(--accent) 40%, var(--border));
		border-radius: var(--radius);
		background: var(--surface-2);
	}
	.invite-lbl {
		font-size: 0.68rem;
		letter-spacing: 0.18em;
		text-transform: uppercase;
		color: var(--muted);
	}
	.invite-code {
		font-size: clamp(1.6rem, 6vw, 2.2rem);
		font-weight: 700;
		color: var(--accent);
		letter-spacing: 0.04em;
	}

	/* ---------- POINTS ---------- */
	/* Tips (per-match) fills the left column; the two Forecast cards — groups +
	   knockout reach, equally weighted — stack on the right. */
	.pts-grid {
		display: grid;
		gap: 0.85rem;
	}
	@media (min-width: 720px) {
		.pts-grid {
			grid-template-columns: 1fr 1fr;
			align-items: stretch;
		}
		.pts-grid .tips {
			grid-row: span 2;
		}
	}
	.score {
		display: flex;
		flex-direction: column;
	}
	.score-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		margin-bottom: 0.9rem;
	}
	.max {
		font-size: 2rem;
		font-weight: 700;
		color: var(--accent);
		line-height: 1;
	}
	.max small {
		font-size: 0.6rem;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--muted);
		margin-left: 0.25rem;
		vertical-align: 3px;
	}
	.score-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.score-list li {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.6rem 0;
		border-top: 1px solid var(--border);
		font-size: 0.92rem;
	}
	.score-list li:first-child {
		border-top: none;
	}
	.score-list b {
		font-size: 1.05rem;
		color: var(--accent);
	}
	.fine {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		font-size: 0.78rem;
		margin: 0.9rem 0 0;
	}
	.reach {
		display: flex;
		gap: 0.3rem;
		text-align: center;
		flex: 1;
		align-items: center;
	}
	.rstep {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		padding: 0.5rem 0.1rem;
		border-radius: var(--radius-sm);
		background: var(--surface-2);
	}
	.rp {
		font-size: 1.15rem;
		font-weight: 700;
		color: var(--accent-2);
		line-height: 1;
	}
	.rr {
		font-size: 0.62rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--muted);
	}

	/* ---------- FINAL ---------- */
	.final {
		margin-top: clamp(2.5rem, 7vw, 4rem);
	}
	.cta-card {
		text-align: center;
		padding: clamp(1.8rem, 5vw, 2.8rem) 1.25rem;
		background:
			radial-gradient(120% 120% at 50% -20%, rgba(200, 251, 80, 0.12), transparent 60%),
			var(--surface);
	}
	.cta-card h2 {
		margin: 0 0 0.4rem;
	}
	.cta-card .cta {
		justify-content: center;
	}
	.foot {
		text-align: center;
		font-size: 0.8rem;
		margin: 1.25rem 0 0;
	}

	/* ---------- STICKY COUNTDOWN BAR ---------- */
	.cd-stickybar {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		z-index: 60;
		animation: cd-drop 0.25s ease;
	}
	@keyframes cd-drop {
		from {
			transform: translateY(-100%);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.cd-stickybar {
			animation: none;
		}
	}

	/* ---------- EASTER EGG ---------- */
	/* Burst anchored to the Save button's centre (left/top set inline); a fixed
	   sibling layer so it escapes the cards' overflow clipping. */
	.party {
		position: fixed;
		z-index: 100;
		transform: translate(-50%, -50%);
		pointer-events: none;
	}
</style>
