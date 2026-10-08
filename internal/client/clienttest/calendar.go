package clienttest

import (
	"context"

	"github.com/kzark/gwen/internal/wire"
)

func (f *Fake) CalendarStatus(context.Context) (*wire.CalendarStatus, error) {
	return result[*wire.CalendarStatus](f, "CalendarStatus")
}

func (f *Fake) CalendarAuthStart(context.Context) (*wire.CalendarAuth, error) {
	return result[*wire.CalendarAuth](f, "CalendarAuthStart")
}

func (f *Fake) CalendarSync(context.Context) (*wire.CalendarStatus, error) {
	return result[*wire.CalendarStatus](f, "CalendarSync")
}

func (f *Fake) CalendarCalendars(context.Context) (*wire.CalendarList, error) {
	return result[*wire.CalendarList](f, "CalendarCalendars")
}
