// SPA mode: render entirely on the client. The Go server serves index.html
// for unknown paths (adapter-static fallback), and SvelteKit routes from
// there. No prerendering since pages depend on auth + live data.
export const ssr = false;
export const prerender = false;
