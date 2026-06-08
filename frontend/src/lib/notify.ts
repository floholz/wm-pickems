// The canonical notification events, shared by the user settings grid and the
// admin delivery-policy console. Keys must match the backend event identifiers
// (internal/notify) — they're used as ledger/pref/override map keys.

export interface NotifyEvent {
	key: string;
	label: string;
	hint: string;
}

export const NOTIFY_EVENTS: NotifyEvent[] = [
	{
		key: 'kickoff_countdown',
		label: 'Countdown to kickoff',
		hint: 'A daily reminder in the final days before the World Cup kicks off.'
	},
	{
		key: 'stage_starting',
		label: 'Stage starting soon',
		hint: 'When the next stage (group stage, knockout rounds) is about to begin.'
	},
	{
		key: 'tips_reminder',
		label: 'Tip reminders',
		hint: "Before upcoming matches if you haven't entered a tip yet."
	},
	{
		key: 'forecast_reminder',
		label: 'Forecast deadline',
		hint: "Before the tournament starts if your Forecast isn't finished."
	},
	{
		key: 'results_recap',
		label: 'Results recap',
		hint: 'A daily summary of how your points and ranking moved.'
	},
	{
		key: 'league_lead',
		label: 'Took the lead',
		hint: 'When you climb to #1 in one of your leagues.'
	},
	{
		key: 'league_chat',
		label: 'League chat',
		hint: 'New messages in your league chats — push is prompt; email is a periodic digest.'
	}
];
