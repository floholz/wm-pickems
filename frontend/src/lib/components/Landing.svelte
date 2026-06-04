<script lang="ts">
	import { auth } from '$lib/auth.svelte';
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
		Sparkles
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

	// Forecast knockout-reach escalation.
	const reach = [
		{ r: 'R32', p: '1' },
		{ r: 'R16', p: '2' },
		{ r: 'QF', p: '3' },
		{ r: 'SF', p: '5' },
		{ r: 'Final', p: '8' },
		{ r: 'Champ', p: '13' }
	];
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
			Predict the cup.<br /><span class="grad"
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
		<div class="grid2">
			<div class="card mode">
				<span class="ic"><Telescope size={22} /></span>
				<h3>Forecast</h3>
				<p class="muted">
					One pre-tournament prediction: full group standings 1–4, the eight
					best-third qualifiers and the entire knockout bracket. Locks at the
					opening kickoff — then scores tick in stage by stage.
				</p>
				<span class="pill ok"><Lock size={13} /> Locks at first kickoff</span>
			</div>
			<div class="card mode">
				<span class="ic alt"><Volleyball size={22} /></span>
				<h3>Tips</h3>
				<p class="muted">
					Predict the score of every match, editable right up to kickoff.
					Knockouts go deeper — 90′, extra time, then penalties. Once a game
					starts your tip locks and you can see what everyone else picked.
				</p>
				<span class="pill"><Check size={13} /> Editable until kickoff</span>
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
		<div class="grid2">
			<div class="card score">
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
					<Sparkles size={13} /> Plus points for every correct group position and
					advancer — a perfect group earns a bonus.
				</p>
			</div>
		</div>
	</section>

	<!-- ============ FINAL CTA ============ -->
	<section class="block final">
		<div class="card cta-card">
			<h2>Kickoff is coming.</h2>
			<p class="muted">Make your picks before the rest of the group does.</p>
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

<style>
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
	.grid3,
	.grid2 {
		display: grid;
		gap: 0.85rem;
	}
	@media (min-width: 720px) {
		.grid3 {
			grid-template-columns: repeat(3, 1fr);
		}
		.grid2 {
			grid-template-columns: repeat(2, 1fr);
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
	.mode h3 {
		margin-bottom: 0.4rem;
	}
	.why p,
	.mode p {
		font-size: 0.92rem;
		line-height: 1.5;
		margin: 0;
	}
	.mode {
		display: flex;
		flex-direction: column;
	}
	.mode .pill {
		align-self: flex-start;
		margin-top: 0.9rem;
	}
	.mode .pill.ok {
		color: var(--accent);
		border-color: color-mix(in srgb, var(--accent) 45%, var(--border));
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
		display: grid;
		grid-template-columns: repeat(6, 1fr);
		gap: 0.3rem;
		text-align: center;
		flex: 1;
		align-content: center;
	}
	.rstep {
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
</style>
