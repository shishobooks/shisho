package devices

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rule(source, delivered string) delivery.Rule {
	return delivery.Rule{SourceFileType: source, DeliveredFileType: delivered}
}

// TestService_Rules resolves the Delivery Rules a Device follows, for each
// way a Device can follow them. userDefault is saved as the owner's default
// first (nil leaves it following the built-in rules), and custom is saved as
// the Device's own list when the mode is custom.
func TestService_Rules(t *testing.T) {
	t.Parallel()

	builtIn := delivery.BuiltInRules()
	koboPreset := []delivery.Rule{
		rule(models.FileTypeKePub, models.FileTypeKePub),
		rule(models.FileTypeEPUB, models.FileTypeKePub),
		rule(models.FileTypeCBZ, models.FileTypeKePub),
	}
	cases := []struct {
		name        string
		deviceType  string
		mode        string // "" keeps the mode a new Device starts with
		userDefault []delivery.Rule
		custom      []delivery.Rule
		wantMode    string
		want        []delivery.Rule
	}{
		{
			name:       "a new Kobo follows its preset",
			deviceType: DeviceTypeKobo,
			wantMode:   RulesModePreset,
			want:       koboPreset,
		},
		{
			name:       "a new Kindle follows its preset",
			deviceType: DeviceTypeKindle,
			wantMode:   RulesModePreset,
			want:       []delivery.Rule{rule(models.FileTypeAZW3, models.FileTypeAZW3), rule(models.FileTypeMOBI, models.FileTypeMOBI)},
		},
		{
			name:       "a new OPDS app follows the built-in default",
			deviceType: DeviceTypeOPDS,
			wantMode:   RulesModeDefault,
			want:       builtIn,
		},
		{
			name:        "a new Browser follows a customized default",
			deviceType:  DeviceTypeBrowser,
			userDefault: []delivery.Rule{rule(models.FileTypeCBZ, models.FileTypeEPUB), rule(models.FileTypeEPUB, models.FileTypeEPUB)},
			wantMode:    RulesModeDefault,
			want:        []delivery.Rule{rule(models.FileTypeCBZ, models.FileTypeEPUB), rule(models.FileTypeEPUB, models.FileTypeEPUB)},
		},
		{
			name:        "a Kobo following the default drops the rules it can't receive",
			deviceType:  DeviceTypeKobo,
			mode:        RulesModeDefault,
			userDefault: []delivery.Rule{rule(models.FileTypeCBZ, models.FileTypeCBZ), rule(models.FileTypeEPUB, models.FileTypeEPUB), rule(models.FileTypePDF, models.FileTypePDF)},
			wantMode:    RulesModeDefault,
			want:        []delivery.Rule{rule(models.FileTypeEPUB, models.FileTypeEPUB)},
		},
		{
			name:        "a Kindle following a default with no Kindle rule delivers nothing",
			deviceType:  DeviceTypeKindle,
			mode:        RulesModeDefault,
			userDefault: []delivery.Rule{rule(models.FileTypeEPUB, models.FileTypeEPUB)},
			wantMode:    RulesModeDefault,
			want:        []delivery.Rule{},
		},
		{
			name:        "a custom list ignores the default",
			deviceType:  DeviceTypeOPDS,
			mode:        RulesModeCustom,
			userDefault: []delivery.Rule{rule(models.FileTypeEPUB, models.FileTypeEPUB)},
			custom:      []delivery.Rule{rule(models.FileTypeCBZ, models.FileTypeCBZ)},
			wantMode:    RulesModeCustom,
			want:        []delivery.Rule{rule(models.FileTypeCBZ, models.FileTypeCBZ)},
		},
		{
			name:       "a custom list keeps its order",
			deviceType: DeviceTypeKobo,
			mode:       RulesModeCustom,
			custom:     []delivery.Rule{rule(models.FileTypeCBZ, models.FileTypeKePub), rule(models.FileTypeEPUB, models.FileTypeEPUB)},
			wantMode:   RulesModeCustom,
			want:       []delivery.Rule{rule(models.FileTypeCBZ, models.FileTypeKePub), rule(models.FileTypeEPUB, models.FileTypeEPUB)},
		},
		{
			name:       "a Kobo set back to its preset",
			deviceType: DeviceTypeKobo,
			mode:       RulesModePreset,
			wantMode:   RulesModePreset,
			want:       koboPreset,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			service := NewService(db)
			insertTestUser(t, db, 1, "reader")
			ctx := context.Background()

			if tc.userDefault != nil {
				_, err := delivery.NewService(db).SaveUserDefault(ctx, 1, tc.userDefault)
				require.NoError(t, err)
			}
			device, err := service.Create(ctx, 1, "Device", tc.deviceType)
			require.NoError(t, err)
			if tc.mode != "" {
				device, err = service.SetRules(ctx, 1, device.ID, tc.mode, tc.custom)
				require.NoError(t, err)
			}

			device, err = service.Retrieve(ctx, 1, device.ID)
			require.NoError(t, err)
			assert.Equal(t, tc.wantMode, device.DeliveryRulesMode)
			rules, err := service.Rules(ctx, device)
			require.NoError(t, err)
			assert.Equal(t, tc.want, rules)
		})
	}
}

// Under the Kobo preset, a book with a KePub and an EPUB sends the KePub as
// itself, and a book with only an EPUB still gets it as a KePub.
func TestKoboPreset_PrefersKePub(t *testing.T) {
	t.Parallel()
	preset := PresetRules(DeviceTypeKobo)
	book := &models.Book{ID: 1, Files: []*models.File{
		{ID: 1, BookID: 1, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain},
		{ID: 2, BookID: 1, FileType: models.FileTypeKePub, FileRole: models.FileRoleMain},
	}}

	result := delivery.Resolve(preset, book)
	require.Len(t, result.Ebook, 1)
	assert.Equal(t, 2, result.Ebook[0].File.ID)
	assert.Equal(t, models.FileTypeKePub, result.Ebook[0].DeliveredFileType)
	assert.Empty(t, result.Ebook[0].Chain, "a KePub goes as itself, with no Conversion")

	book.Files = book.Files[:1]
	result = delivery.Resolve(preset, book)
	require.Len(t, result.Ebook, 1)
	assert.Equal(t, 1, result.Ebook[0].File.ID)
	assert.Equal(t, models.FileTypeKePub, result.Ebook[0].DeliveredFileType)
}

// A Device following the default follows later edits to it, and stops once
// it is customized.
func TestService_Rules_FollowingTracksTheDefault(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	deliveryService := delivery.NewService(db)
	insertTestUser(t, db, 1, "reader")
	ctx := context.Background()

	device, err := service.Create(ctx, 1, "KOReader", DeviceTypeOPDS)
	require.NoError(t, err)
	edited := []delivery.Rule{rule(models.FileTypeCBZ, models.FileTypeCBZ)}
	_, err = deliveryService.SaveUserDefault(ctx, 1, edited)
	require.NoError(t, err)
	rules, err := service.Rules(ctx, device)
	require.NoError(t, err)
	assert.Equal(t, edited, rules)

	custom := []delivery.Rule{rule(models.FileTypeEPUB, models.FileTypeEPUB)}
	device, err = service.SetRules(ctx, 1, device.ID, RulesModeCustom, custom)
	require.NoError(t, err)
	_, err = deliveryService.SaveUserDefault(ctx, 1, []delivery.Rule{rule(models.FileTypePDF, models.FileTypePDF)})
	require.NoError(t, err)
	rules, err = service.Rules(ctx, device)
	require.NoError(t, err)
	assert.Equal(t, custom, rules)

	// Following again drops the custom list.
	device, err = service.SetRules(ctx, 1, device.ID, RulesModeDefault, nil)
	require.NoError(t, err)
	rules, err = service.Rules(ctx, device)
	require.NoError(t, err)
	assert.Equal(t, []delivery.Rule{rule(models.FileTypePDF, models.FileTypePDF)}, rules)
	var count int
	require.NoError(t, db.NewRaw(`SELECT COUNT(*) FROM device_delivery_rules`).Scan(ctx, &count))
	assert.Zero(t, count, "a following Device keeps no copy")
}

func TestService_SetRules_Refuses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		deviceType string
		mode       string
		rules      []delivery.Rule
	}{
		{name: "an empty custom list", deviceType: DeviceTypeOPDS, mode: RulesModeCustom, rules: []delivery.Rule{}},
		{name: "a Kindle rule delivering EPUB", deviceType: DeviceTypeKindle, mode: RulesModeCustom, rules: []delivery.Rule{rule(models.FileTypeAZW3, models.FileTypeAZW3), rule(models.FileTypeEPUB, models.FileTypeEPUB)}},
		{name: "a Kobo rule delivering CBZ", deviceType: DeviceTypeKobo, mode: RulesModeCustom, rules: []delivery.Rule{rule(models.FileTypeCBZ, models.FileTypeCBZ)}},
		{name: "two rules for one source", deviceType: DeviceTypeBrowser, mode: RulesModeCustom, rules: []delivery.Rule{rule(models.FileTypeCBZ, models.FileTypeCBZ), rule(models.FileTypeCBZ, models.FileTypeEPUB)}},
		{name: "a preset for a type with none", deviceType: DeviceTypeOPDS, mode: RulesModePreset},
		{name: "an unknown mode", deviceType: DeviceTypeOPDS, mode: "sometimes"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			service := NewService(db)
			insertTestUser(t, db, 1, "reader")
			ctx := context.Background()
			device, err := service.Create(ctx, 1, "Device", tc.deviceType)
			require.NoError(t, err)

			_, err = service.SetRules(ctx, 1, device.ID, tc.mode, tc.rules)
			var rulesErr *delivery.RulesError
			require.True(t, errors.As(err, &rulesErr), "got %v", err)
			assert.NotEmpty(t, rulesErr.Reason)

			unchanged, err := service.Retrieve(ctx, 1, device.ID)
			require.NoError(t, err)
			assert.Equal(t, device.DeliveryRulesMode, unchanged.DeliveryRulesMode)
		})
	}
}

// Every type accepts a custom list of the File Types it allows.
func TestService_SetRules_AcceptsAllowedTypes(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	ctx := context.Background()

	for deviceType, rules := range map[string][]delivery.Rule{
		DeviceTypeKobo:    {rule(models.FileTypeEPUB, models.FileTypeEPUB), rule(models.FileTypeCBZ, models.FileTypeKePub)},
		DeviceTypeKindle:  {rule(models.FileTypeMOBI, models.FileTypeMOBI)},
		DeviceTypeOPDS:    {rule(models.FileTypeCBZ, models.FileTypeEPUB), rule(models.FileTypeM4B, models.FileTypeM4B)},
		DeviceTypeBrowser: {rule(models.FileTypePDF, models.FileTypePDF)},
	} {
		device, err := service.Create(ctx, 1, "Device", deviceType)
		require.NoError(t, err)
		_, err = service.SetRules(ctx, 1, device.ID, RulesModeCustom, rules)
		require.NoError(t, err, deviceType)
	}
}

// Changing a Device's type puts it back on the rules its new type starts
// with, since a custom list may name File Types the new type can't receive.
func TestService_Update_TypeChangeResetsRules(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	ctx := context.Background()

	device, err := service.Create(ctx, 1, "Reader", DeviceTypeBrowser)
	require.NoError(t, err)
	_, err = service.SetRules(ctx, 1, device.ID, RulesModeCustom, []delivery.Rule{rule(models.FileTypeEPUB, models.FileTypeEPUB)})
	require.NoError(t, err)

	kindle := DeviceTypeKindle
	device, err = service.Update(ctx, 1, device.ID, UpdateDeviceOptions{DeviceType: &kindle})
	require.NoError(t, err)
	assert.Equal(t, DeviceTypeKindle, device.DeviceType)
	assert.Equal(t, RulesModePreset, device.DeliveryRulesMode)
	rules, err := service.Rules(ctx, device)
	require.NoError(t, err)
	assert.Equal(t, PresetRules(DeviceTypeKindle), rules)

	browser := DeviceTypeBrowser
	device, err = service.Update(ctx, 1, device.ID, UpdateDeviceOptions{DeviceType: &browser})
	require.NoError(t, err)
	assert.Equal(t, RulesModeDefault, device.DeliveryRulesMode)
}
