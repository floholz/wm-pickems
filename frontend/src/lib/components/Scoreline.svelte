<!-- Canonical match scoreline used across the app. An arrow on the winning side
     gives an instant visual anchor for the result:
       home wins:  ◂ 3 : 0        away wins:  0 : 3 ▸
     Both arrow slots are always rendered (the non-winning side is hidden but
     still reserves its width) so the line is the same width regardless of who
     won — the score stays centred and consistent across a list of matches, with
     a small gap between the arrow and the digits.
     The winner is `winner` when given (a knockout's advancer, which accounts
     for extra time / penalties); otherwise it falls back to the higher score,
     so group games get the arrow too. A draw shows no arrow.
     For a knockout that goes to extra time it also shows the after-ET score
     (what the result points are based on) with the 90' score in parentheses,
     e.g. ◂ (1) 2 : 2 (1) when the home side wins on penalties. -->
<script lang="ts">
	let {
		home,
		away,
		etHome = 0,
		etAway = 0,
		et = false,
		winner = ''
	}: {
		home: number;
		away: number;
		etHome?: number;
		etAway?: number;
		// True when this is a knockout decided in extra time / penalties, i.e.
		// the 90' score was a draw — show the ET-inclusive form.
		et?: boolean;
		// Which side wins, when it can't be read off the score (a knockout's
		// advancer). Empty falls back to comparing the scoreline below.
		winner?: '' | 'home' | 'away';
	} = $props();

	// Resolve the arrow side: explicit winner first, else the higher score
	// (extra-time score when it went to ET, otherwise the 90' score).
	let side = $derived.by<'' | 'home' | 'away'>(() => {
		if (winner) return winner;
		const [h, a] = et ? [etHome, etAway] : [home, away];
		if (h > a) return 'home';
		if (a > h) return 'away';
		return '';
	});
</script>

<span class="adv l" class:on={side === 'home'}>◂</span
>{#if et}<span class="ninety">({home})</span>
	{etHome}
	<span class="cln">:</span>
	{etAway}
	<span class="ninety">({away})</span>{:else}{home}<span class="cln">:</span>{away}{/if}<span
	class="adv r"
	class:on={side === 'away'}>▸</span
>

<style>
	/* The parenthesised 90' score inside an extra-time result. */
	.ninety {
		font-size: 0.78em;
		opacity: 0.6;
		font-weight: 600;
	}
	/* Arrow pointing to the winning side. Both slots always occupy space; the
	   non-winning one is hidden so the line width stays constant. */
	.adv {
		display: inline-block;
		font-size: 1.2em;
		line-height: 1;
		color: var(--accent);
		visibility: hidden;
	}
	.adv.on {
		visibility: visible;
	}
	.adv.l {
		margin-right: 0.34em;
	}
	.adv.r {
		margin-left: 0.34em;
	}
</style>
