package main

import "github.com/kzark/gwen/internal/wire"

func (a *App) SyncStatus() (*wire.SyncStatus, error) { return call(a.api.SyncStatus(a.ctx)) }
func (a *App) SyncNow() (*wire.SyncStatus, error)    { return call(a.api.SyncNow(a.ctx)) }
