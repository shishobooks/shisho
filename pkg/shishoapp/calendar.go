package shishoapp

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/reading"
)

// Reading Stats' Calendar (CapabilityReadingCalendar): the same heatmap,
// month, and day the web's Calendar tab shows, with each Book named by its
// id for the app to find in its own catalog.

// statsHeatmap sends the user's reading for a year, or the last 12 months,
// a day at a time.
func (h *handler) statsHeatmap(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	var q StatsHeatmapQuery
	if err := c.Bind(&q); err != nil {
		return errors.WithStack(err)
	}
	resp, err := h.reading.StatsHeatmap(c.Request().Context(), user, q.Year)
	if err != nil {
		return err
	}
	out := StatsHeatmapResponse{Today: resp.Today, From: resp.From, To: resp.To, FirstDay: resp.FirstDay,
		Days: make([]StatsDay, 0, len(resp.Days))}
	for _, d := range resp.Days {
		out.Days = append(out.Days, StatsDay{Day: d.Day, ReadingSeconds: d.ReadingSeconds, ListeningSeconds: d.ListeningSeconds})
	}
	return errors.WithStack(c.JSON(http.StatusOK, out))
}

// statsCalendar sends one month of the user's Reading Stats calendar.
func (h *handler) statsCalendar(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	var q StatsCalendarQuery
	if err := c.Bind(&q); err != nil {
		return errors.WithStack(err)
	}
	resp, err := h.reading.StatsCalendar(c.Request().Context(), user, q.Year, q.Month)
	if err != nil {
		return err
	}
	out := StatsCalendarResponse{Year: resp.Year, Month: resp.Month, Books: bookRefs(resp.Books),
		Days: make([]StatsCalendarDay, 0, len(resp.Days))}
	for _, d := range resp.Days {
		day := StatsCalendarDay{Day: d.Day, Books: make([]StatsCalendarDayBook, 0, len(d.Books))}
		for _, b := range d.Books {
			day.Books = append(day.Books, StatsCalendarDayBook{Key: b.Key, Seconds: b.Seconds, Finished: b.Finished})
		}
		out.Days = append(out.Days, day)
	}
	return errors.WithStack(c.JSON(http.StatusOK, out))
}

// statsDay sends one day of the user's Reading Sessions and the
// Read-throughs finished on it.
func (h *handler) statsDay(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	var q StatsDayQuery
	if err := c.Bind(&q); err != nil {
		return errors.WithStack(err)
	}
	resp, err := h.reading.StatsDay(c.Request().Context(), user, q.Day)
	if err != nil {
		return err
	}
	out := StatsDayResponse{Day: resp.Day, TimeZone: resp.TimeZone, Finished: bookRefs(resp.Finished),
		Sessions: make([]StatsDaySession, 0, len(resp.Sessions))}
	for _, s := range resp.Sessions {
		out.Sessions = append(out.Sessions, StatsDaySession{
			ID: s.ID, Book: bookRef(s.Book), StartedAt: s.StartedAt, EndedAt: s.EndedAt, Seconds: s.Seconds,
			StartPercentage: s.StartPercentage, EndPercentage: s.EndPercentage, Kind: s.Kind, Source: s.Source,
			Measured: s.Measured, DeviceID: s.DeviceID, DeviceName: s.DeviceName,
		})
	}
	return errors.WithStack(c.JSON(http.StatusOK, out))
}

func bookRef(in reading.ReadingStatsBookRef) StatsBookRef {
	out := StatsBookRef{Key: in.Key, Title: in.Title, Removed: in.Removed}
	if in.Book != nil {
		out.BookID = &in.Book.ID
	}
	return out
}

func bookRefs(in []reading.ReadingStatsBookRef) []StatsBookRef {
	out := make([]StatsBookRef, 0, len(in))
	for _, ref := range in {
		out = append(out, bookRef(ref))
	}
	return out
}
