<script lang="ts">
	import '../app.css';
	import { auth } from '$lib/auth.svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import Logo from '$lib/components/Logo.svelte';
	import UserMenu from '$lib/components/UserMenu.svelte';
	import NavLinks from '$lib/components/NavLinks.svelte';
	import PwaInstallButton from '$lib/components/PwaInstallButton.svelte';
	import PwaInstallBanner from '$lib/components/PwaInstallBanner.svelte';
	import NotifyAnnounce from '$lib/components/NotifyAnnounce.svelte';
	import { serverClock } from '$lib/serverclock.svelte';
	import { CircleHelp } from '@lucide/svelte';

	let { children } = $props();

	// Pull the (possibly simulated) server clock once so lock checks and the
	// dev-tools link are correct app-wide.
	$effect(() => {
		if (auth.isAuthed && !serverClock.loaded) serverClock.refresh();
	});

	// Signed-out-only pages — visible to anonymous users; signed-in users
	// bounce away to home (or /join if an invite is attached).
	const authPages = ['/login', '/register', '/forgot-password'];
	let path = $derived($page.url.pathname);
	let isAuthPage = $derived(authPages.includes(path));
	// Public routes — anyone can land here regardless of auth state:
	//   /join/<code>                 invite landing
	//   /confirm-password-reset/<t>  email reset target (must work even for
	//                                a still-signed-in user whose token was
	//                                requested by someone with their email)
	//   /welcome                     chrome-less landing/help page (any auth state)
	let isPublic = $derived(
		path.startsWith('/join') ||
			path.startsWith('/confirm-password-reset/') ||
			path === '/welcome'
	);
	// The home route doubles as the public landing page for signed-out
	// visitors (app home once authed) — never bounce anon users away from it.
	let isLanding = $derived(path === '/');
	// No app chrome on the standalone auth / invite / reset screens.
	let chrome = $derived(auth.isAuthed && !isAuthPage && !isPublic);

	// SPA auth guard.
	$effect(() => {
		const invite = $page.url.searchParams.get('invite');
		if (!auth.isAuthed && !isAuthPage && !isPublic && !isLanding) {
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
	<!-- Mobile: top header (logo / help / install / user menu) -->
	<header class="topbar">
		<Logo />
		<div class="spacer"></div>
		<a class="topbar-help" href="/welcome" aria-label="What is WM Tips?">
			<CircleHelp size={20} />
		</a>
		<PwaInstallButton />
		<UserMenu align="right" />
	</header>

	<!-- Desktop: left rail (logo top, links, help + user menu bottom) -->
	<aside class="siderail">
		<div class="rail-logo"><Logo /></div>
		<NavLinks variant="rail" />
		<div class="spacer"></div>
		<a class="rail-help" href="/welcome">
			<CircleHelp size={20} />
			<span>How does it work?</span>
		</a>
		<div class="rail-user"><UserMenu align="left" up showName /></div>
	</aside>

	<!-- Mobile: bottom tab bar -->
	<nav class="tabbar"><NavLinks variant="tab" /></nav>
{/if}

<div class="app-shell" class:with-chrome={chrome}>
	{#if chrome}
		<PwaInstallBanner />
		<NotifyAnnounce />
	{/if}
	{@render children()}
</div>
