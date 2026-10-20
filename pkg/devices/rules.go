package devices

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// deviceTypeRules is what a Device type allows and starts with.
type deviceTypeRules struct {
	// allowed lists the File Types the type can receive. Nil allows any.
	allowed []string
	// preset is the type's preset, or nil when it has none and new Devices
	// follow the owner's default instead.
	preset []delivery.Rule
}

// typeRules holds each Device type's allowed File Types and preset. Presets
// live in code and change with releases, so a Device following one picks up
// the change without a data migration.
var typeRules = map[string]deviceTypeRules{
	DeviceTypeKobo: {
		allowed: []string{models.FileTypeKePub, models.FileTypeEPUB},
		preset: []delivery.Rule{
			{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeKePub},
			{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeKePub},
		},
	},
	// A Kindle's browser downloads only its own formats. AZW3 comes first
	// because it renders as KF8; MOBI covers books without one.
	DeviceTypeKindle: {
		allowed: []string{models.FileTypeAZW3, models.FileTypeMOBI},
		preset: []delivery.Rule{
			{SourceFileType: models.FileTypeAZW3, DeliveredFileType: models.FileTypeAZW3},
			{SourceFileType: models.FileTypeMOBI, DeliveredFileType: models.FileTypeMOBI},
		},
	},
	DeviceTypeOPDS:    {},
	DeviceTypeBrowser: {},
}

// AllowsFileType reports whether a Device of deviceType can receive a File
// delivered as fileType.
func AllowsFileType(deviceType, fileType string) bool {
	allowed := typeRules[deviceType].allowed
	return allowed == nil || slices.Contains(allowed, fileType)
}

// PresetRules returns deviceType's preset, or nil when it has none.
func PresetRules(deviceType string) []delivery.Rule {
	return slices.Clone(typeRules[deviceType].preset)
}

// RuleOptions lists every source File Type a custom rule for a Device of
// deviceType can name, in the built-in rules' order, with only the File
// Types the type can receive. A source File Type with none is left out.
func RuleOptions(deviceType string) []delivery.RuleOption {
	options := []delivery.RuleOption{}
	for _, builtIn := range delivery.BuiltInRules() {
		delivered := []string{}
		for _, fileType := range models.ReachableFileTypes(builtIn.SourceFileType) {
			if AllowsFileType(deviceType, fileType) {
				delivered = append(delivered, fileType)
			}
		}
		if len(delivered) > 0 {
			options = append(options, delivery.RuleOption{SourceFileType: builtIn.SourceFileType, DeliveredFileTypes: delivered})
		}
	}
	return options
}

// startingRulesMode is the mode a new Device of deviceType starts in: its
// preset when the type has one, and the owner's default otherwise.
func startingRulesMode(deviceType string) string {
	if typeRules[deviceType].preset != nil {
		return RulesModePreset
	}
	return RulesModeDefault
}

// allowedRules returns the rules that deliver a File Type deviceType can
// receive, so a Device following the owner's default skips the rest rather
// than receiving a File it can't open.
func allowedRules(deviceType string, rules []delivery.Rule) []delivery.Rule {
	kept := []delivery.Rule{}
	for _, rule := range rules {
		if AllowsFileType(deviceType, rule.DeliveredFileType) {
			kept = append(kept, rule)
		}
	}
	return kept
}

// ValidateRules checks a custom list for a Device of deviceType: everything
// delivery.Validate checks, and no rule delivering a File Type the type
// can't receive.
func ValidateRules(deviceType string, rules []delivery.Rule) error {
	if err := delivery.Validate(rules); err != nil {
		return err
	}
	for _, rule := range rules {
		if !AllowsFileType(deviceType, rule.DeliveredFileType) {
			return &delivery.RulesError{Reason: fmt.Sprintf("%s can't receive %s.", sentence(TypeLabelWithArticle(deviceType)), models.FileTypeLabel(rule.DeliveredFileType))}
		}
	}
	return nil
}

// TypeLabel is how a Device type is written for a reader.
func TypeLabel(deviceType string) string {
	switch deviceType {
	case DeviceTypeKobo:
		return "Kobo"
	case DeviceTypeKindle:
		return "Kindle"
	case DeviceTypeOPDS:
		return "OPDS app"
	}
	return "Browser"
}

// TypeLabelWithArticle is TypeLabel as a noun phrase, such as "a Kobo
// Device" or "an OPDS app Device".
func TypeLabelWithArticle(deviceType string) string {
	if deviceType == DeviceTypeOPDS {
		return "an OPDS app Device"
	}
	return "a " + TypeLabel(deviceType) + " Device"
}

// sentence capitalizes the first letter of s.
func sentence(s string) string {
	return strings.ToUpper(s[:1]) + s[1:]
}

// CheckRules returns a *delivery.RulesError when a Device of deviceType
// can't follow rules in mode: an unknown mode, a preset for a type with
// none, or a custom list ValidateRules refuses.
func CheckRules(deviceType, mode string, rules []delivery.Rule) error {
	switch mode {
	case RulesModeDefault:
		return nil
	case RulesModePreset:
		if typeRules[deviceType].preset == nil {
			return &delivery.RulesError{Reason: sentence(TypeLabelWithArticle(deviceType)) + " has no preset to follow."}
		}
		return nil
	case RulesModeCustom:
		return ValidateRules(deviceType, rules)
	}
	return &delivery.RulesError{Reason: fmt.Sprintf("%q is not a way a Device can follow Delivery Rules.", mode)}
}

// Rules returns the Delivery Rules device follows: its owner's default
// without the rules its type can't receive, its type's preset, or its own
// list. Every channel a Device connects through delivers by these.
func (s *Service) Rules(ctx context.Context, device *Device) ([]delivery.Rule, error) {
	rules, err := s.RulesFor(ctx, []*Device{device})
	if err != nil {
		return nil, err
	}
	return rules[device.ID], nil
}

// RulesFor returns the Delivery Rules each of devices follows, by Device
// id, as Rules does. It loads every custom list in one query and each
// owner's default once, so listing Devices costs the same few queries
// however many there are.
func (s *Service) RulesFor(ctx context.Context, devices []*Device) (map[string][]delivery.Rule, error) {
	rules := map[string][]delivery.Rule{}
	var customIDs []string
	defaults := map[int][]delivery.Rule{}
	for _, device := range devices {
		switch device.DeliveryRulesMode {
		case RulesModePreset:
			// Never nil, which would read as "no rules loaded": a type
			// without a preset delivers nothing.
			rules[device.ID] = append([]delivery.Rule{}, PresetRules(device.DeviceType)...)
		case RulesModeCustom:
			customIDs = append(customIDs, device.ID)
			rules[device.ID] = []delivery.Rule{}
		default:
			userDefault, ok := defaults[device.UserID]
			if !ok {
				loaded, err := delivery.NewService(s.db).UserDefault(ctx, device.UserID)
				if err != nil {
					return nil, errors.WithStack(err)
				}
				userDefault = loaded.Rules
				defaults[device.UserID] = userDefault
			}
			rules[device.ID] = allowedRules(device.DeviceType, userDefault)
		}
	}
	if len(customIDs) == 0 {
		return rules, nil
	}
	var rows []*DeviceDeliveryRule
	err := s.db.NewSelect().
		Model(&rows).
		Where("ddr.device_id IN (?)", bun.List(customIDs)).
		Order("ddr.device_id ASC", "ddr.position ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, row := range rows {
		rules[row.DeviceID] = append(rules[row.DeviceID], delivery.Rule{SourceFileType: row.SourceFileType, DeliveredFileType: row.DeliveredFileType})
	}
	return rules, nil
}

// SetRules makes userID's Device id follow the owner's default, follow its
// type's preset, or use rules as its own list, and returns the Device. A
// following mode ignores rules and drops any list saved before. It returns
// a *delivery.RulesError for a mode or list the Device can't have.
func (s *Service) SetRules(ctx context.Context, userID int, id string, mode string, rules []delivery.Rule) (*Device, error) {
	return s.Update(ctx, userID, id, UpdateDeviceOptions{Rules: &RulesChange{Mode: mode, Rules: rules}})
}

// saveRules stores device's mode, replacing its own list with rules when
// the mode is custom and dropping it otherwise. Callers validate first.
func saveRules(ctx context.Context, tx bun.IDB, device *Device, mode string, rules []delivery.Rule) error {
	if _, err := tx.NewDelete().Model((*DeviceDeliveryRule)(nil)).Where("device_id = ?", device.ID).Exec(ctx); err != nil {
		return errors.WithStack(err)
	}
	if mode == RulesModeCustom {
		rows := make([]*DeviceDeliveryRule, len(rules))
		for i, rule := range rules {
			rows[i] = &DeviceDeliveryRule{
				DeviceID:          device.ID,
				SourceFileType:    rule.SourceFileType,
				DeliveredFileType: rule.DeliveredFileType,
				Position:          i,
			}
		}
		if _, err := tx.NewInsert().Model(&rows).Exec(ctx); err != nil {
			return errors.WithStack(err)
		}
	}
	device.DeliveryRulesMode = mode
	device.UpdatedAt = time.Now()
	_, err := tx.NewUpdate().Model(device).Column("delivery_rules_mode", "updated_at").WherePK().Exec(ctx)
	return errors.WithStack(err)
}
