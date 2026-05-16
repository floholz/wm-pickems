<script lang="ts">
	import '../app.css';
	import { auth } from '$lib/auth.svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import Logo from '$lib/components/Logo.svelte';
	import UserMenu from '$lib/components/UserMenu.svelte';
	import NavLinks from '$lib/components/NavLinks.svelte';
	import { serverClock } from '$lib/serverclock.svelte';

	let { children } = $props();

	// Pull the (possibly simulated) server clock once so lock checks and the
	// dev-tools link are correct app-wide.
	$effect(() => {
		if (auth.isAuthed && !serverClock.loaded) serverClock.refresh();
	});

	// Signed-out-only pages. /join/* is open to everyone (the invite landing
	// decides what to show), so it never triggers the auth bounce.
	const authPages = ['/login', '/register'];
	let path = $derived($page.url.pathname);
	let isAuthPage = $derived(authPages.includes(path));
	let isJoin = $derived(path.startsWith('/join'));
	// No app chrome on the standalone auth / invite screens.
	let chrome = $derived(auth.isAuthed && !isAuthPage && !isJoin);

	// SPA auth guard.
	$effect(() => {
		const invite = $page.url.searchParams.get('invite');
		if (!auth.isAuthed && !isAuthPage && !isJoin) {
			goto('/login', { replaceState: true });
		}
		// Already signed in: skip the auth pages. If they arrived via an
		// invite, send them to the join flow so it auto-joins.
		if (auth.isAuthed && isAuthPage) {
			goto(invite ? `/join/${invite}` : '/', { replaceState: true });
		}
	});
</script>

{#if chrome}
	<!-- Mobile: top header (logo / user menu) -->
	<header class="topbar">
		<Logo />
		<div class="spacer"></div>
		<UserMenu align="right" />
	</header>

	<!-- Desktop: left rail (logo top, links, user menu bottom) -->
	<aside class="siderail">
		<div class="rail-logo"><Logo /></div>
		<NavLinks variant="rail" />
		<div class="spacer"></div>
		<div class="rail-user"><UserMenu align="left" up showName /></div>
	</aside>

	<!-- Mobile: bottom tab bar -->
	<nav class="tabbar"><NavLinks variant="tab" /></nav>
{/if}

<div class="app-shell" class:with-chrome={chrome}>
	{@render children()}
</div>
