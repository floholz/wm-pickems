/** Collapse a sticky page header on scroll (hide the big title + intro, keep
 *  the kicker line + tabs), expanding again only near the very top.
 *
 *  Uses hysteresis — collapse past `collapseAt`, expand only back under
 *  `expandAt` — so slow scrolling or sitting exactly on a single threshold
 *  can't make the header oscillate (the collapse itself shifts layout height,
 *  which would otherwise re-cross a single cutoff and vibrate). The class is
 *  only touched on an actual state change, coalesced via rAF. */
export function collapseOnScroll(
	node: HTMLElement,
	opts: { collapseAt?: number; expandAt?: number } = {}
) {
	const collapseAt = opts.collapseAt ?? 64;
	const expandAt = opts.expandAt ?? 6;

	let collapsed = window.scrollY > collapseAt;
	let ticking = false;
	node.classList.toggle('scrolled', collapsed);

	const apply = () => {
		ticking = false;
		const y = window.scrollY;
		if (!collapsed && y > collapseAt) {
			collapsed = true;
			node.classList.add('scrolled');
		} else if (collapsed && y < expandAt) {
			collapsed = false;
			node.classList.remove('scrolled');
			// Expanding grows the header, which would push us off the top and
			// re-cross the threshold (jump/oscillation). We only get here
			// within `expandAt` px of the top, so pin to 0 and let the header
			// grow from a stable anchor.
			window.scrollTo(0, 0);
		}
	};
	const onScroll = () => {
		if (ticking) return;
		ticking = true;
		requestAnimationFrame(apply);
	};

	window.addEventListener('scroll', onScroll, { passive: true });
	return {
		destroy() {
			window.removeEventListener('scroll', onScroll);
		}
	};
}
