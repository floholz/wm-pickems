import { pb } from './pb';

// Calls our custom Go endpoints. pb.send attaches the auth token and resolves
// relative to the SDK base URL (same origin).
async function post<T>(path: string, body: unknown): Promise<T> {
	return pb.send(path, { method: 'POST', body });
}
async function get<T>(path: string): Promise<T> {
	return pb.send(path, { method: 'GET' });
}
async function del<T>(path: string): Promise<T> {
	return pb.send(path, { method: 'DELETE' });
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
	avatar?: string; // file name in the users.avatar field; empty/absent => none
	role?: string; // "admin" | "bot"; empty/absent => normal member
	total: number;
	tipsPoints: number;
	forecastPoints: number;
	predicted: number;
	exactScores: number;
	correctWinners: number;
	gdDeviation: number;
	forecast?: Record<string, number>;
}

export interface BotSummary {
	userId: string;
	name: string;
	avatar?: string;
	botKind?: string;
}

export interface ChatMessage {
	id: string;
	user: string; // sender user id
	text: string; // empty when deleted
	created: string; // RFC3339
	deleted?: boolean;
	// Moderation fields, returned only to app-admins for deleted messages:
	original?: string;
	deletedBy?: string;
	deletedAt?: string;
}

export interface ChatMember {
	userId: string;
	name: string;
	avatar?: string;
	role?: string;
}

export type AnnounceLevel = 'info' | 'success' | 'warn';

export interface Announcement {
	id: string;
	title: string;
	body: string;
	level: AnnounceLevel;
	active: boolean;
	highPriority: boolean; // high-urgency push when broadcast
	persistent: boolean; // can't be dismissed — only collapsed
	notifiedAt: string; // RFC3339, empty if never broadcast
	created: string;
}

export interface AnnouncePayload {
	title?: string;
	body?: string;
	level?: AnnounceLevel;
	active?: boolean;
	highPriority?: boolean;
	persistent?: boolean;
}

export interface SyncLastRun {
	at: string; // RFC3339
	source: string;
	updated: number;
	ok: boolean;
	error?: string;
}

export interface SyncStatus {
	source: string; // 'api-football' | 'openfootball' | 'none'
	autoSync: boolean;
	cron: string;
	lastRun: SyncLastRun | null;
	account?: {
		subscription?: { plan?: string; active?: boolean; end?: string };
		requests?: { current?: number; limit_day?: number };
	};
	accountError?: string;
}

export interface OwnerStats {
	users: number; // real users (bots excluded)
	usersLast24h: number;
	activeUsers: number; // >=3 tips or a complete forecast
	leagues: number; // user-created (Global excluded)
	activeLeagues: number; // >1 member and some tips
	pushEnabled: number;
	notifyDisabled: number; // opted out of >=1 notification
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
		post<{ ok: boolean }>(`/api/leagues/${id}/members/remove`, { userId }),
	// Owner-only: bot accounts not yet in the league, and adding one.
	availableBots: (id: string) =>
		get<{ bots: BotSummary[] }>(`/api/leagues/${id}/bots`),
	addBot: (id: string, userId: string) =>
		post<{ ok: boolean; already?: boolean }>(`/api/leagues/${id}/bots/add`, {
			userId
		}),
	// League chat (private leagues only).
	chatHistory: (leagueId: string, before?: string) =>
		get<{ messages: ChatMessage[]; hasMore: boolean }>(
			`/api/leagues/${leagueId}/chat${before ? `?before=${encodeURIComponent(before)}` : ''}`
		),
	chatMembers: (leagueId: string) =>
		get<{ members: ChatMember[] }>(`/api/leagues/${leagueId}/members`),
	chatPost: (leagueId: string, text: string) =>
		post<ChatMessage>(`/api/leagues/${leagueId}/chat`, { text }),
	chatDelete: (leagueId: string, msgId: string) =>
		del<ChatMessage>(`/api/leagues/${leagueId}/chat/${msgId}`),
	chatRestore: (leagueId: string, msgId: string) =>
		post<ChatMessage>(`/api/leagues/${leagueId}/chat/${msgId}/restore`, {}),
	chatMarkRead: (leagueId: string) =>
		post<{ ok: boolean }>(`/api/leagues/${leagueId}/chat/read`, {}),
	chatUnread: () => get<{ unread: Record<string, number> }>('/api/chat/unread'),

	// Owner-only app stats dashboard.
	ownerStats: () => get<OwnerStats>('/api/stats/owner'),

	// Owner-only results-sync dashboard: status + manual trigger.
	syncStatus: () => get<SyncStatus>('/api/admin/sync/status'),
	syncRun: () =>
		post<{ status?: string; updated?: number; error?: string; lastRun: SyncLastRun | null }>(
			'/api/admin/sync/run',
			{}
		),

	// Announcements: active list (any signed-in user, for the banner) + the
	// owner/admin-only management endpoints.
	activeAnnouncements: () =>
		get<{ announcements: Announcement[] }>('/api/announce/active'),
	allAnnouncements: () =>
		get<{ announcements: Announcement[] }>('/api/admin/announce'),
	createAnnouncement: (p: AnnouncePayload) =>
		post<Announcement>('/api/admin/announce', p),
	updateAnnouncement: (id: string, p: AnnouncePayload) =>
		post<Announcement>(`/api/admin/announce/${id}`, p),
	deleteAnnouncement: (id: string) =>
		del<{ ok: boolean }>(`/api/admin/announce/${id}`),
	sendAnnouncement: (id: string) =>
		post<{
			announcement: Announcement;
			result: { considered: number; sent: number; failed: number; skipped: number };
		}>(`/api/admin/announce/${id}/send`, {})
};
