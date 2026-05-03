package aliases

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/uptrace/bun"
)

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

type ResourceConfig struct {
	AliasTable    string
	ResourceFK    string
	ResourceTable string
}

func (svc *Service) ListAliases(ctx context.Context, cfg ResourceConfig, resourceID int) ([]string, error) {
	var names []string
	err := svc.db.NewSelect().
		TableExpr(cfg.AliasTable).
		Column("name").
		Where(cfg.ResourceFK+" = ?", resourceID).
		Order("name ASC").
		Scan(ctx, &names)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if names == nil {
		return []string{}, nil
	}
	return names, nil
}

func (svc *Service) AddAlias(ctx context.Context, cfg ResourceConfig, resourceID int, name string, libraryID int) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errcodes.ValidationError("Alias name cannot be empty")
	}

	var primaryName string
	err := svc.db.NewSelect().
		TableExpr(cfg.ResourceTable).
		Column("name").
		Where("id = ?", resourceID).
		Scan(ctx, &primaryName)
	if err != nil {
		return errors.WithStack(err)
	}
	if strings.EqualFold(name, primaryName) {
		return errcodes.ValidationError("Alias cannot match the resource's own name")
	}

	var conflictCount int
	conflictCount, err = svc.db.NewSelect().
		TableExpr(cfg.ResourceTable).
		Where("LOWER(name) = LOWER(?) AND library_id = ?", name, libraryID).
		Count(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	if conflictCount > 0 {
		return errcodes.ValidationError("Alias conflicts with an existing name")
	}

	var aliasCount int
	aliasCount, err = svc.db.NewSelect().
		TableExpr(cfg.AliasTable).
		Where("LOWER(name) = LOWER(?) AND library_id = ?", name, libraryID).
		Count(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	if aliasCount > 0 {
		return errcodes.ValidationError("Alias conflicts with an existing alias")
	}

	_, err = svc.db.NewRaw(
		"INSERT INTO "+cfg.AliasTable+" (created_at, "+cfg.ResourceFK+", name, library_id) VALUES (?, ?, ?, ?)",
		time.Now(), resourceID, name, libraryID,
	).Exec(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return errcodes.ValidationError("Alias conflicts with an existing alias")
		}
		return errors.WithStack(err)
	}

	return nil
}

func (svc *Service) RemoveAlias(ctx context.Context, cfg ResourceConfig, resourceID int, name string) error {
	_, err := svc.db.NewDelete().
		TableExpr(cfg.AliasTable).
		Where(cfg.ResourceFK+" = ? AND LOWER(name) = LOWER(?)", resourceID, name).
		Exec(ctx)
	return errors.WithStack(err)
}

func (svc *Service) RemoveAllAliases(ctx context.Context, cfg ResourceConfig, resourceID int) error {
	_, err := svc.db.NewDelete().
		TableExpr(cfg.AliasTable).
		Where(cfg.ResourceFK+" = ?", resourceID).
		Exec(ctx)
	return errors.WithStack(err)
}

func (svc *Service) SyncAliases(ctx context.Context, cfg ResourceConfig, resourceID int, libraryID int, desired []string) error {
	current, err := svc.ListAliases(ctx, cfg, resourceID)
	if err != nil {
		return err
	}

	currentSet := make(map[string]bool, len(current))
	for _, a := range current {
		currentSet[strings.ToLower(a)] = true
	}

	desiredSet := make(map[string]bool, len(desired))
	for _, a := range desired {
		desiredSet[strings.ToLower(strings.TrimSpace(a))] = true
	}

	for _, a := range current {
		if !desiredSet[strings.ToLower(a)] {
			if err := svc.RemoveAlias(ctx, cfg, resourceID, a); err != nil {
				return err
			}
		}
	}

	for _, a := range desired {
		trimmed := strings.TrimSpace(a)
		if trimmed == "" {
			continue
		}
		if !currentSet[strings.ToLower(trimmed)] {
			if err := svc.AddAlias(ctx, cfg, resourceID, trimmed, libraryID); err != nil {
				return err
			}
		}
	}

	return nil
}
