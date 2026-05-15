import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// SPA mode: no SSR, fallback so client-side routes resolve when the
		// Go server serves index.html for unknown paths. Output goes straight
		// into the Go embed directory.
		adapter: adapter({
			pages: '../internal/web/build',
			assets: '../internal/web/build',
			fallback: 'index.html',
			precompress: false,
			strict: true
		})
	}
};

export default config;
