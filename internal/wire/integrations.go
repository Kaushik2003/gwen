package wire

// CalendarStatus is the GET /v1/calendar/status response.
type CalendarStatus struct {
	Enabled    bool    `json:"enabled"`
	Connected  bool    `json:"connected"`
	CalendarID *string `json:"calendar_id"`
	LastSyncAt *int64  `json:"last_sync_at"`
	LastError  *string `json:"last_error"`
}

// CalendarAuth is the POST /v1/calendar/auth/start response.
type CalendarAuth struct {
	AuthURL string `json:"auth_url"`
}

// CalendarInfo is one of the user's calendars. The main one has the id
// "primary".
type CalendarInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Color   string `json:"color"`
	Primary bool   `json:"primary"`
}

// CalendarList is the GET /v1/calendar/calendars response: every calendar
// but Gwen's own, the main one first.
type CalendarList struct {
	Calendars []CalendarInfo `json:"calendars"`
}

// CalendarEvent is an event of a busy calendar, which takes its time out of
// the plan.
type CalendarEvent struct {
	CalendarID string `json:"calendar_id"`
	Title      string `json:"title"`
	StartAt    int64  `json:"start_at"`
	EndAt      int64  `json:"end_at"`
}

// SyncStatus is the GET /v1/sync/status response.
type SyncStatus struct {
	Configured bool    `json:"configured"`
	LastPushAt *int64  `json:"last_push_at"`
	LastPullAt *int64  `json:"last_pull_at"`
	LastError  *string `json:"last_error"`
}

// IntegrationChanged is the data of integration_changed.
type IntegrationChanged struct {
	Calendar CalendarStatus `json:"calendar"`
	Sync     SyncStatus     `json:"sync"`
}
