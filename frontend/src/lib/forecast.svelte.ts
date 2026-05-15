import { pb } from './pb';
import { auth } from './auth.svelte';
import type { Team } from './tips.svelte';

export interface KOMatch {
	num: number;
	stage: string;
	round: string;
	homeLabel: string;
	awayLabel: string;
}
export interface ThirdSlot {
	matchNum: number;
	allowed: string[];
}
export interface GroupDef {
	letter: string;
	teams: string[];
}

/** Stable key for a KO match: its number, or the stage for the number-less
 *  Final / third-place matches. */
export function koKey(m: { num: number; stage: string }): string {
	return m.num > 0 ? String(m.num) : m.stage;
}

class ForecastStore {
	loaded = $state(false);
	locked = $state(false);
	tournamentStart = $state<string>('');
	teams = $state<Record<string, Team>>({});
	groups = $state<GroupDef[]>([]);
	knockout = $state<KOMatch[]>([]);
	thirdSlots = $state<ThirdSlot[]>([]);

	// Editable forecast.
	recId: string | undefined;
	groupOrder = $state<Record<string, string[]>>({}); // letter -> [id x4]
	thirds = $state<Record<string, string>>({}); // matchNum -> teamId
	bracket = $state<Record<string, string>>({}); // koKey -> winner teamId

	async load() {
		const [structure, teams, mine] = await Promise.all([
			pb.send('/api/forecast/structure', { method: 'GET' }),
			pb.collection('teams').getFullList({ sort: 'name' }),
			pb
				.collection('forecasts')
				.getFullList({ filter: `user = "${auth.user?.id}"` })
		]);
		const tmap: Record<string, Team> = {};
		for (const t of teams)
			tmap[t.id] = {
				id: t.id,
				name: t.name,
				iso2: t.iso2,
				fifaCode: t.fifaCode
			};
		this.teams = tmap;
		this.groups = structure.groups;
		this.knockout = structure.knockout;
		this.thirdSlots = structure.thirdSlots ?? [];
		this.tournamentStart = structure.tournamentStart;
		this.locked = structure.locked;

		const f = mine[0];
		this.recId = f?.id;
		// Default group order = the group's team list until the user reorders.
		const order: Record<string, string[]> = {};
		for (const g of this.groups)
			order[g.letter] = f?.groupOrder?.[g.letter]?.length
				? [...f.groupOrder[g.letter]]
				: [...g.teams];
		this.groupOrder = order;
		this.thirds = f?.thirdQualifiers ?? {};
		this.bracket = f?.bracket ?? {};
		this.loaded = true;
	}

	team(id: string) {
		return this.teams[id];
	}

	move(letter: string, idx: number, dir: -1 | 1) {
		const arr = [...this.groupOrder[letter]];
		const j = idx + dir;
		if (j < 0 || j >= arr.length) return;
		[arr[idx], arr[j]] = [arr[j], arr[idx]];
		this.groupOrder[letter] = arr;
	}

	/** Resolve a placeholder label ("1A","2B","3A/B/..","W74","L101") to a
	 *  team id given the current predictions, or '' if undecidable. */
	resolve(label: string, forMatchNum: number, seen = new Set<number>()): string {
		if (!label) return '';
		const c = label[0];
		if (c === '1' || c === '2') {
			const letter = label.slice(1);
			return this.groupOrder[letter]?.[c === '1' ? 0 : 1] ?? '';
		}
		if (c === '3') return this.thirdAssignment()[forMatchNum] ?? '';
		if (c === 'W' || c === 'L') {
			const n = parseInt(label.slice(1), 10);
			if (seen.has(n)) return '';
			seen.add(n);
			const w = this.bracket[String(n)] ?? '';
			if (c === 'W') return w;
			const src = this.knockout.find((m) => m.num === n);
			if (!src || !w) return '';
			const h = this.resolve(src.homeLabel, n, seen);
			const a = this.resolve(src.awayLabel, n, seen);
			return w === h ? a : w === a ? h : '';
		}
		return '';
	}

	sides(m: KOMatch): [string, string] {
		return [
			this.resolve(m.homeLabel, m.num),
			this.resolve(m.awayLabel, m.num)
		];
	}

	pick(m: KOMatch, teamId: string) {
		if (!teamId) return;
		this.bracket[koKey(m)] = teamId;
	}

	readonly maxThirds = 8;

	/** The predicted 3rd-placed team of a group (from the current order). */
	groupThird(letter: string): string {
		return this.groupOrder[letter]?.[2] ?? '';
	}

	/** Letters the user ticked to advance as a best third. */
	get chosenThirdLetters(): string[] {
		return Object.keys(this.thirds);
	}

	toggleThird(letter: string) {
		if (this.thirds[letter]) {
			delete this.thirds[letter];
			this.thirds = { ...this.thirds };
		} else if (this.chosenThirdLetters.length < this.maxThirds) {
			this.thirds = { ...this.thirds, [letter]: this.groupThird(letter) };
		}
	}

	/** Deterministically slot the 8 chosen thirds into the 8 R32 third-slots:
	 *  slots in match order, each filled by the lowest-letter chosen third its
	 *  rule allows that isn't used yet. Mirrors the backend so Forecast
	 *  knockout scoring agrees. */
	thirdAssignment(): Record<number, string> {
		const chosen = this.chosenThirdLetters.sort();
		const used = new Set<string>();
		const out: Record<number, string> = {};
		for (const slot of [...this.thirdSlots].sort(
			(a, b) => a.matchNum - b.matchNum
		)) {
			for (const letter of chosen) {
				if (used.has(letter) || !slot.allowed.includes(letter)) continue;
				out[slot.matchNum] = this.groupThird(letter);
				used.add(letter);
				break;
			}
		}
		return out;
	}

	async save() {
		// Persist thirds as {groupLetter: currentThirdTeamId} so the value
		// stays correct even if the group order changed after ticking.
		const thirdQualifiers: Record<string, string> = {};
		for (const letter of this.chosenThirdLetters)
			thirdQualifiers[letter] = this.groupThird(letter);
		const data = {
			user: auth.user?.id,
			groupOrder: this.groupOrder,
			thirdQualifiers,
			bracket: this.bracket
		};
		const rec = this.recId
			? await pb.collection('forecasts').update(this.recId, data)
			: await pb.collection('forecasts').create(data);
		this.recId = rec.id;
	}
}

export const forecastStore = new ForecastStore();
