package main

import "github.com/kzark/gwen/internal/wire"

func (a *App) CalendarStatus() (*wire.CalendarStatus, error) {
	return call(a.api.CalendarStatus(a.ctx))
}
func (a *App) CalendarAuthStart() (*wire.CalendarAuth, error) {
	return call(a.api.CalendarAuthStart(a.ctx))
}
func (a *App) CalendarSync() (*wire.CalendarStatus, error) { return call(a.api.CalendarSync(a.ctx)) }
