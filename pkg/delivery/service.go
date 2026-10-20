package delivery

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// Service loads and saves users' default Delivery Rules.
type Service struct {
	db *bun.DB
}

// NewService returns a Service.
func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// UserRules is a user's default Delivery Rules: the built-in rules while
// FollowsBuiltIn, and the user's own list otherwise.
type UserRules struct {
	FollowsBuiltIn bool
	Rules          []Rule
}

// UserDefault returns the user's default Delivery Rules.
func (svc *Service) UserDefault(ctx context.Context, userID int) (*UserRules, error) {
	return userDefault(ctx, svc.db, userID)
}

func userDefault(ctx context.Context, db bun.IDB, userID int) (*UserRules, error) {
	var rows []*UserDeliveryRule
	err := db.NewSelect().
		Model(&rows).
		Where("udr.user_id = ?", userID).
		Order("udr.position ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(rows) == 0 {
		return &UserRules{FollowsBuiltIn: true, Rules: BuiltInRules()}, nil
	}
	rules := make([]Rule, len(rows))
	for i, row := range rows {
		rules[i] = Rule{SourceFileType: row.SourceFileType, DeliveredFileType: row.DeliveredFileType}
	}
	return &UserRules{Rules: rules}, nil
}

// SaveUserDefault replaces the user's default Delivery Rules with a
// customized list, in order. It returns a *RulesError for a list Validate
// refuses.
func (svc *Service) SaveUserDefault(ctx context.Context, userID int, rules []Rule) (*UserRules, error) {
	if err := Validate(rules); err != nil {
		return nil, err
	}
	var saved *UserRules
	err := svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := deleteUserDefault(ctx, tx, userID); err != nil {
			return err
		}
		rows := make([]*UserDeliveryRule, len(rules))
		for i, rule := range rules {
			rows[i] = &UserDeliveryRule{
				UserID:            userID,
				SourceFileType:    rule.SourceFileType,
				DeliveredFileType: rule.DeliveredFileType,
				Position:          i,
			}
		}
		if _, err := tx.NewInsert().Model(&rows).Exec(ctx); err != nil {
			return errors.WithStack(err)
		}
		var err error
		saved, err = userDefault(ctx, tx, userID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
}

// ResetUserDefault makes the user's default follow the built-in rules again,
// and returns it.
func (svc *Service) ResetUserDefault(ctx context.Context, userID int) (*UserRules, error) {
	if err := deleteUserDefault(ctx, svc.db, userID); err != nil {
		return nil, err
	}
	return &UserRules{FollowsBuiltIn: true, Rules: BuiltInRules()}, nil
}

func deleteUserDefault(ctx context.Context, db bun.IDB, userID int) error {
	_, err := db.NewDelete().
		Model((*UserDeliveryRule)(nil)).
		Where("user_id = ?", userID).
		Exec(ctx)
	return errors.WithStack(err)
}
