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
	private: boolean;
	members: number;
}

export interface LeaderboardRow {
	userId: string;
	name: string;
	role?: string; // "member" | "admin" | "bot" (empty/absent => member)
	total: number;
	tipsPoints: number;
	forecastPoints: number;
	predicted: number;
	exactScores: number;
	correctWinners: number;
	gdDeviation: number;
	forecast?: Record<string, number>;
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
	// Public — resolves an invite code to a league name for the /join page.
	invitePreview: (code: string) =>
		get<{ id: string; name: string }>(
			`/api/invite/${encodeURIComponent(code)}`
		),
	myLeagues: () => get<{ leagues: LeagueSummary[] }>('/api/leagues/mine'),
	leaderboard: (id: string) =>
		get<{
			league: { id: string; name: string };
			rows: LeaderboardRow[];
			scoring?: Record<string, unknown>;
		}>(`/api/leagues/${id}/leaderboard`),
	// Owner-only league management.
	renameLeague: (id: string, name: string) =>
		post<{ id: string; name: string }>(`/api/leagues/${id}/rename`, { name }),
	regenerateCode: (id: string) =>
		post<{ inviteCode: string }>(`/api/leagues/${id}/code/regenerate`, {}),
	setCodePrivacy: (id: string, isPrivate: boolean) =>
		post<{ private: boolean }>(`/api/leagues/${id}/code/visibility`, {
			private: isPrivate
		}),
	removeMember: (id: string, userId: string) =>
		post<{ ok: boolean }>(`/api/leagues/${id}/members/remove`, { userId })
};
