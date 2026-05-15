import { House, Target, Telescope, Network, Trophy } from '@lucide/svelte';
import type { Component } from 'svelte';

export interface NavItem {
	href: string;
	label: string;
	icon: Component;
}

export const navItems: NavItem[] = [
	{ href: '/', label: 'Home', icon: House },
	{ href: '/tips', label: 'Tips', icon: Target },
	{ href: '/forecast', label: 'Forecast', icon: Telescope },
	{ href: '/tournament', label: 'Bracket', icon: Network },
	{ href: '/leagues', label: 'Leagues', icon: Trophy }
];

export function isActive(href: string, path: string): boolean {
	return href === '/' ? path === '/' : path.startsWith(href);
}
