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
