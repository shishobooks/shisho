// Package gettingstarted serves each user's Getting Started list: a few
// first steps that tick themselves off when what they ask for exists. The
// state (whether the list is on, dismissed, which steps are "Not for me",
// and whether the panel opens on its own) lives on the user's row.
package gettingstarted

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// steps lists every step in panel order.
var steps = []string{StepLibrary, StepPlugins, StepInvite, StepDevice, StepToRead, StepStartReading}

// Service reads and writes Getting Started state.
type Service struct {
	db *bun.DB
}

// NewService returns a Service over db.
func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// state is the Getting Started columns of one user's row.
type state struct {
	bun.BaseModel `bun:"table:users,alias:u"`

	Enabled        bool              `bun:"getting_started_enabled"`
	AutoOpen       bool              `bun:"getting_started_auto_open"`
	DismissedAt    *time.Time        `bun:"getting_started_dismissed_at"`
	NotForMe       models.StringList `bun:"getting_started_not_for_me"`
	DismissedSteps models.StringList `bun:"getting_started_dismissed_steps"`
}

// Status builds user's Getting Started list from their stored state and
// what exists now. Steps apply by permission, not role name, and a step that
// doesn't apply is never checked. A list that is off has no steps. A
// dismissed list has none either, unless a step now applies that didn't when
// it was dismissed and still needs doing: then the list comes back.
func (s *Service) Status(ctx context.Context, user *models.User) (GettingStartedResponse, error) {
	st := state{}
	if err := s.db.NewSelect().Model(&st).
		Column("getting_started_enabled", "getting_started_auto_open", "getting_started_dismissed_at",
			"getting_started_not_for_me", "getting_started_dismissed_steps").
		Where("u.id = ?", user.ID).
		Scan(ctx); err != nil {
		return GettingStartedResponse{}, errors.WithStack(err)
	}

	resp := GettingStartedResponse{
		Enabled:   st.Enabled,
		Dismissed: st.DismissedAt != nil,
		AutoOpen:  st.AutoOpen,
		Steps:     make([]GettingStartedStep, 0, len(steps)),
	}
	if !resp.Enabled {
		return resp, nil
	}

	applies, err := s.applicable(ctx, user)
	if err != nil {
		return GettingStartedResponse{}, err
	}

	if resp.Dismissed {
		// Every page asks for the list, so only the steps that could bring it
		// back are checked.
		var fresh []string
		for _, key := range applies {
			if !slices.Contains(st.DismissedSteps, key) && !slices.Contains(st.NotForMe, key) {
				fresh = append(fresh, key)
			}
		}
		done, err := s.done(ctx, user, fresh)
		if err != nil {
			return GettingStartedResponse{}, err
		}
		if !slices.ContainsFunc(fresh, func(key string) bool { return !done[key] }) {
			return resp, nil
		}
		resp.Dismissed = false
	}

	done, err := s.done(ctx, user, applies)
	if err != nil {
		return GettingStartedResponse{}, err
	}
	for _, key := range steps {
		step := GettingStartedStep{Key: key, Applies: slices.Contains(applies, key)}
		if step.Applies {
			step.Done = done[key]
			step.NotForMe = slices.Contains(st.NotForMe, key)
		}
		resp.Steps = append(resp.Steps, step)
	}
	return resp, nil
}

// applicable returns the steps the user can do now, in panel order.
func (s *Service) applicable(ctx context.Context, user *models.User) ([]string, error) {
	canReadBooks := user.HasPermission(models.ResourceBooks, models.OperationRead)
	canWriteConfig := user.HasPermission(models.ResourceConfig, models.OperationWrite)
	applies := map[string]bool{
		StepLibrary: user.HasPermission(models.ResourceLibraries, models.OperationWrite),
		StepInvite:  user.HasPermission(models.ResourceUsers, models.OperationWrite),
		StepDevice:  canReadBooks,
	}
	// Before a Library exists, setting one up covers plugins.
	if canWriteConfig {
		libraryExists, err := s.db.NewSelect().Model((*models.Library)(nil)).Exists(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		applies[StepPlugins] = libraryExists
	}
	// To Read and reading need a book, so those steps wait until the user can
	// reach a library. A Device can be set up before then.
	if canReadBooks {
		reachable, err := s.canReachLibrary(ctx, user)
		if err != nil {
			return nil, err
		}
		applies[StepToRead] = reachable
		applies[StepStartReading] = reachable
	}
	keys := make([]string, 0, len(steps))
	for _, key := range steps {
		if applies[key] {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// done reports, for each of keys, whether what the step asks for exists.
func (s *Service) done(ctx context.Context, user *models.User, keys []string) (map[string]bool, error) {
	done := make(map[string]bool, len(keys))
	for _, key := range keys {
		var (
			exists bool
			err    error
		)
		switch key {
		case StepLibrary:
			exists, err = s.db.NewSelect().Model((*models.Library)(nil)).Exists(ctx)
		case StepPlugins:
			exists, err = s.db.NewSelect().Model((*models.PluginHookConfig)(nil)).
				Where("phc.hook_type = ?", models.PluginHookMetadataEnricher).
				Exists(ctx)
		case StepInvite:
			exists, err = s.db.NewSelect().Model((*models.User)(nil)).
				Where("u.id != ?", user.ID).
				Where("u.is_active = ?", true).
				Exists(ctx)
		case StepDevice:
			exists, err = s.db.NewSelect().Model((*devices.Device)(nil)).
				Where("d.user_id = ?", user.ID).
				Exists(ctx)
		case StepToRead:
			exists, err = s.hasUsedToRead(ctx, user.ID)
		case StepStartReading:
			exists, err = s.hasStartedReading(ctx, user.ID)
		}
		if err != nil {
			return nil, errors.WithStack(err)
		}
		done[key] = exists
	}
	return done, nil
}

// canReachLibrary reports whether a library the user has access to exists.
func (s *Service) canReachLibrary(ctx context.Context, user *models.User) (bool, error) {
	q := s.db.NewSelect().Model((*models.Library)(nil))
	if ids := user.GetAccessibleLibraryIDs(); ids != nil {
		if len(ids) == 0 {
			return false, nil
		}
		q = q.Where("l.id IN (?)", bun.List(ids))
	}
	reachable, err := q.Exists(ctx)
	return reachable, errors.WithStack(err)
}

// hasUsedToRead reports whether the user's To Read holds a Book, or held
// one they then started: starting a Book takes it off To Read, and its
// Read-through keeps the place it left.
func (s *Service) hasUsedToRead(ctx context.Context, userID int) (bool, error) {
	queued, err := s.db.NewSelect().Model((*models.UserBook)(nil)).
		Where("ub.user_id = ?", userID).
		Where("ub.to_read_position IS NOT NULL").
		Exists(ctx)
	if err != nil || queued {
		return queued, errors.WithStack(err)
	}
	started, err := s.db.NewSelect().Model((*models.ReadThrough)(nil)).
		Where("rt.user_id = ?", userID).
		Where("rt.left_to_read_position IS NOT NULL").
		Exists(ctx)
	return started, errors.WithStack(err)
}

// hasStartedReading reports whether the user has any Reading Position or
// Read-through.
func (s *Service) hasStartedReading(ctx context.Context, userID int) (bool, error) {
	positioned, err := s.db.NewSelect().Model((*models.ReadingPosition)(nil)).
		Where("rp.user_id = ?", userID).
		Exists(ctx)
	if err != nil || positioned {
		return positioned, errors.WithStack(err)
	}
	read, err := s.db.NewSelect().Model((*models.ReadThrough)(nil)).
		Where("rt.user_id = ?", userID).
		Exists(ctx)
	return read, errors.WithStack(err)
}

// errUnknownStep refuses a step key that isn't one of the steps.
func errUnknownStep(step string) error {
	return errcodes.ValidationError(fmt.Sprintf("Unknown Getting Started step %q.", step))
}

// SetNotForMe marks step "Not for me" for the user, or undoes that.
func (s *Service) SetNotForMe(ctx context.Context, userID int, step string, notForMe bool) error {
	if !slices.Contains(steps, step) {
		return errUnknownStep(step)
	}
	return errors.WithStack(s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		st := state{}
		if err := tx.NewSelect().Model(&st).
			Column("getting_started_not_for_me").
			Where("u.id = ?", userID).
			Scan(ctx); err != nil {
			return errors.WithStack(err)
		}
		list := slices.DeleteFunc(slices.Clone(st.NotForMe), func(key string) bool { return key == step })
		if notForMe {
			list = append(list, step)
		}
		_, err := tx.NewUpdate().Model((*models.User)(nil)).
			Set("getting_started_not_for_me = ?", list).
			Where("id = ?", userID).
			Exec(ctx)
		return errors.WithStack(err)
	}))
}

// Dismiss hides the user's list until they restore it, or until a step
// applies that doesn't now. It records the steps that apply now to tell.
func (s *Service) Dismiss(ctx context.Context, user *models.User) error {
	applies, err := s.applicable(ctx, user)
	if err != nil {
		return err
	}
	_, err = s.db.NewUpdate().Model((*models.User)(nil)).
		Set("getting_started_dismissed_at = ?", time.Now()).
		Set("getting_started_dismissed_steps = ?", models.StringList(applies)).
		Where("id = ?", user.ID).
		Exec(ctx)
	return errors.WithStack(err)
}

// Restore brings the user's list back after a dismissal, and turns it on for
// a user it was never on for. "Not for me" choices are kept.
func (s *Service) Restore(ctx context.Context, userID int) error {
	_, err := s.db.NewUpdate().Model((*models.User)(nil)).
		Set("getting_started_enabled = ?", true).
		Set("getting_started_dismissed_at = NULL").
		Where("id = ?", userID).
		Exec(ctx)
	return errors.WithStack(err)
}

// ClearAutoOpen records that the panel has opened on its own.
func (s *Service) ClearAutoOpen(ctx context.Context, userID int) error {
	_, err := s.db.NewUpdate().Model((*models.User)(nil)).
		Set("getting_started_auto_open = ?", false).
		Where("id = ?", userID).
		Exec(ctx)
	return errors.WithStack(err)
}

// Show is an admin's "Show Getting Started" for another user: it turns the
// list on, clears their dismissal and "Not for me" choices, and opens the
// panel on their next visit. It returns errcodes.NotFound when no such user
// exists.
func (s *Service) Show(ctx context.Context, userID int) error {
	result, err := s.db.NewUpdate().Model((*models.User)(nil)).
		Set("getting_started_enabled = ?", true).
		Set("getting_started_auto_open = ?", true).
		Set("getting_started_dismissed_at = NULL").
		Set("getting_started_not_for_me = ?", models.StringList{}).
		Where("id = ?", userID).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return errors.WithStack(err)
	}
	if affected == 0 {
		return errcodes.NotFound("User")
	}
	return nil
}
