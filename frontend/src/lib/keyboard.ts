// Reflects the on-screen keyboard as a `kb-open` class + a `--kb` height var on
// the document, using the VisualViewport API (keyboard overlays the layout and
// shrinks the visual viewport). Lets CSS hide the bottom nav while typing and
// lift fixed bottom bars (the chat composer) above the keyboard.
//
// Side-effecting on import; no-op on the server or without VisualViewport.
if (typeof window !== 'undefined' && window.visualViewport) {
	const vv = window.visualViewport;
	const root = document.documentElement;
	let raf = 0;

	const apply = () => {
		raf = 0;
		// How much of the layout viewport the keyboard covers at the bottom.
		const kb = Math.max(0, window.innerHeight - vv.height - vv.offsetTop);
		root.style.setProperty('--kb', `${kb}px`);
		// Threshold ignores small UI shifts (URL bar) — real keyboards are taller.
		document.body.classList.toggle('kb-open', kb > 120);
	};

	const schedule = () => {
		if (!raf) raf = requestAnimationFrame(apply);
	};

	vv.addEventListener('resize', schedule);
	vv.addEventListener('scroll', schedule);
	apply();
}
