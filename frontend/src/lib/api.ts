import { pb } from './pb';

// Calls our custom Go endpoints. pb.send attaches the auth token and resolves
// relative to the SDK base URL (same origin).
async function post<T>(path: string, body: unknown): Promise<T> {
	return pb.send(path, { method: 'POST', body });
}
async function get<T>(path: string): Promise<T> {
	return pb.send(path, { method: 'GET' });
}

export interface LeagueSummary {
	id: string;
	name: string;
	inviteCode: string;
	role: string;
	members: number;
}

export interface LeaderboardRow {
	userId: string;
	name: string;
	total: number;
	tipsPoints: number;
	forecastPoints: number;
	exactScores: number;
	correctWinners: number;
	gdDeviation: number;
}

export const api = {
	createLeague: (name: string) =>
		post<{ id: string; name: string; inviteCode: string }>(
			'/api/leagues/create',
			{ name }
		),
	joinLeague: (code: string) =>
		post<{ id: string; name: string; already?: boolean }>(
			'/api/leagues/join',
			{ code }
		),
	myLeagues: () => get<{ leagues: LeagueSummary[] }>('/api/leagues/mine'),
	leaderboard: (id: string) =>
		get<{ league: { id: string; name: string }; rows: LeaderboardRow[] }>(
			`/api/leagues/${id}/leaderboard`
		)
};
