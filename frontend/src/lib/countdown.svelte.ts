import { pb } from './pb';
import { serverClock } from './serverclock.svelte';

// Drives the landing-page countdown to the forecast / first-matchup lock.
// Both lock at the tournament's opening kickoff (the earliest match), which is
// publicly readable — so this works for signed-out visitors. Time comes from
// the server clock (/api/now) so the countdown honours the dev virtual clock.
class Countdown {
	kickoff = $state<number | null>(null); // ms epoch of the first kickoff
	remaining = $state(0); // ms until kickoff, on the server clock
	ready = $state(false); // first kickoff has been resolved (or failed)

	#timer: ReturnType<typeof setInterval> | null = null;
	#loaded = false;

	// Locked once the opening whistle has passed (and we actually know when).
	get locked() {
		return this.ready && this.kickoff !== null && this.remaining <= 0;
	}

	get parts() {
		const s = Math.floor(Math.max(0, this.remaining) / 1000);
		return {
			days: Math.floor(s / 86400),
			hours: Math.floor((s % 86400) / 3600),
			mins: Math.floor((s % 3600) / 60),
			secs: s % 60
		};
	}

	async start() {
		if (!this.#loaded) {
			this.#loaded = true;
			await serverClock.refresh();
			try {
				const r = await pb
					.collection('matches')
					.getList(1, 1, { sort: 'kickoff', fields: 'kickoff' });
				const first = r.items[0]?.kickoff;
				this.kickoff = first ? new Date(first).getTime() : null;
			} catch {
				this.kickoff = null;
			}
			this.ready = true;
		}
		this.#tick();
		if (!this.#timer && !this.locked) {
			this.#timer = setInterval(() => this.#tick(), 1000);
		}
	}

	stop() {
		if (this.#timer) clearInterval(this.#timer);
		this.#timer = null;
	}

	#tick() {
		if (this.kickoff === null) {
			this.remaining = 0;
			return;
		}
		this.remaining = this.kickoff - serverClock.now();
		if (this.remaining <= 0) this.stop();
	}
}

export const countdown = new Countdown();
