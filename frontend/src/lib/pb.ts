import PocketBase from 'pocketbase';
import { browser } from '$app/environment';

// Same-origin in production (Go serves both API and app). In dev, Vite proxies
// /api to the local PocketBase, so same-origin works there too.
export const pb = new PocketBase(browser ? window.location.origin : '/');

// Keep the SDK from auto-cancelling overlapping requests; the app fires
// several reads in parallel (tables, bracket, leaderboard).
pb.autoCancellation(false);
