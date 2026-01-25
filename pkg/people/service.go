package people

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sortname"
	"github.com/uptrace/bun"
)

type RetrievePersonOptions struct {
	ID        *int
	Name      *string
	LibraryID *int
}

type ListPeopleOptions struct {
	Limit      *int
	Offset     *int
	LibraryID  *int
	LibraryIDs []int // Filter by multiple library IDs (for access control)
	Search     *string

	includeTotal bool
}

type UpdatePersonOptions struct {
	Columns []string
}

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db}
}

func (svc *Service) CreatePerson(ctx context.Context, person *models.Person) error {
	now := time.Now()
	if person.CreatedAt.IsZero() {
		person.CreatedAt = now
	}
	person.UpdatedAt = person.CreatedAt

	// Generate sort name if not provided
	if person.SortName == "" {
		person.SortName = sortname.ForPerson(person.Name)
		person.SortNameSource = models.DataSourceFilepath // Auto-generated
	}
	// Ensure source is set if not already
	if person.SortNameSource == "" {
		person.SortNameSource = models.DataSourceFilepath
	}

	_, err := svc.db.
		NewInsert().
		Model(person).
		Returning("*").
		Exec(ctx)
	return errors.WithStack(err)
}

func (svc *Service) RetrievePerson(ctx context.Context, opts RetrievePersonOptions) (*models.Person, error) {
	person := &models.Person{}

	q := svc.db.
		NewSelect().
		Model(person)

	if opts.ID != nil {
		q = q.Where("p.id = ?", *opts.ID)
	}
	if opts.Name != nil && opts.LibraryID != nil {
		// Case-insensitive match
		q = q.Where("LOWER(p.name) = LOWER(?) AND p.library_id = ?", *opts.Name, *opts.LibraryID)
	}

	err := q.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("Person")
		}
		return nil, errors.WithStack(err)
	}

	return person, nil
}

// FindOrCreatePerson finds an existing person or creates a new one (case-insensitive match).
func (svc *Service) FindOrCreatePerson(ctx context.Context, name string, libraryID int) (*models.Person, error) {
	// Normalize the name by trimming whitespace
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("person name cannot be empty")
	}

	person, err := svc.RetrievePerson(ctx, RetrievePersonOptions{
		Name:      &name,
		LibraryID: &libraryID,
	})
	if err == nil {
		return person, nil
	}
	if !errors.Is(err, errcodes.NotFound("Person")) {
		return nil, err
	}

	// Create new person
	person = &models.Person{
		LibraryID:      libraryID,
		Name:           name,
		SortName:       sortname.ForPerson(name),
		SortNameSource: models.DataSourceFilepath,
	}
	err = svc.CreatePerson(ctx, person)
	if err != nil {
		return nil, err
	}
	return person, nil
}

func (svc *Service) ListPeople(ctx context.Context, opts ListPeopleOptions) ([]*models.Person, error) {
	p, _, err := svc.listPeopleWithTotal(ctx, opts)
	return p, errors.WithStack(err)
}

func (svc *Service) ListPeopleWithTotal(ctx context.Context, opts ListPeopleOptions) ([]*models.Person, int, error) {
	opts.includeTotal = true
	return svc.listPeopleWithTotal(ctx, opts)
}

func (svc *Service) listPeopleWithTotal(ctx context.Context, opts ListPeopleOptions) ([]*models.Person, int, error) {
	var people []*models.Person
	var total int
	var err error

	q := svc.db.
		NewSelect().
		Model(&people).
		Order("p.sort_name ASC")

	if opts.LibraryID != nil {
		q = q.Where("p.library_id = ?", *opts.LibraryID)
	}
	if len(opts.LibraryIDs) > 0 {
		q = q.Where("p.library_id IN (?)", bun.In(opts.LibraryIDs))
	}
	// Search using FTS5
	if opts.Search != nil && *opts.Search != "" {
		ftsQuery := buildFTSPrefixQuery(*opts.Search)
		if ftsQuery != "" {
			q = q.Where("p.id IN (SELECT person_id FROM persons_fts WHERE persons_fts MATCH ?)", ftsQuery)
		}
	}
	if opts.Limit != nil {
		q = q.Limit(*opts.Limit)
	}
	if opts.Offset != nil {
		q = q.Offset(*opts.Offset)
	}

	if opts.includeTotal {
		total, err = q.ScanAndCount(ctx)
	} else {
		err = q.Scan(ctx)
	}
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	return people, total, nil
}

func (svc *Service) UpdatePerson(ctx context.Context, person *models.Person, opts UpdatePersonOptions) error {
	if len(opts.Columns) == 0 {
		return nil
	}

	now := time.Now()
	person.UpdatedAt = now
	columns := append(opts.Columns, "updated_at")

	_, err := svc.db.
		NewUpdate().
		Model(person).
		Column(columns...).
		WherePK().
		Exec(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("Person")
		}
		return errors.WithStack(err)
	}
	return nil
}

// DeletePerson deletes a person and all their associations.
func (svc *Service) DeletePerson(ctx context.Context, personID int) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Delete authors associations
		_, err := tx.NewDelete().
			Model((*models.Author)(nil)).
			Where("person_id = ?", personID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete narrators associations
		_, err = tx.NewDelete().
			Model((*models.Narrator)(nil)).
			Where("person_id = ?", personID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete the person
		_, err = tx.NewDelete().
			Model((*models.Person)(nil)).
			Where("id = ?", personID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// GetAuthoredBooks returns all books authored by this person.
func (svc *Service) GetAuthoredBooks(ctx context.Context, personID int) ([]*models.Book, error) {
	var books []*models.Book

	err := svc.db.NewSelect().
		Model(&books).
		Distinct().
		Join("INNER JOIN authors a ON a.book_id = b.id").
		Where("a.person_id = ?", personID).
		Order("b.title ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return books, nil
}

// GetNarratedFiles returns all files narrated by this person.
func (svc *Service) GetNarratedFiles(ctx context.Context, personID int) ([]*models.File, error) {
	var files []*models.File

	err := svc.db.NewSelect().
		Model(&files).
		Relation("Book").
		Join("INNER JOIN narrators n ON n.file_id = f.id").
		Where("n.person_id = ?", personID).
		Order("f.filepath ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return files, nil
}

// GetAuthoredBookCount returns the count of books authored by this person.
func (svc *Service) GetAuthoredBookCount(ctx context.Context, personID int) (int, error) {
	count, err := svc.db.NewSelect().
		Model((*models.Author)(nil)).
		Where("person_id = ?", personID).
		Count(ctx)
	return count, errors.WithStack(err)
}

// GetNarratedFileCount returns the count of files narrated by this person.
func (svc *Service) GetNarratedFileCount(ctx context.Context, personID int) (int, error) {
	count, err := svc.db.NewSelect().
		Model((*models.Narrator)(nil)).
		Where("person_id = ?", personID).
		Count(ctx)
	return count, errors.WithStack(err)
}

// MergePeople merges sourcePerson into targetPerson (moves all associations, deletes source).
func (svc *Service) MergePeople(ctx context.Context, targetID, sourceID int) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Update all authors from source to target
		_, err := tx.NewUpdate().
			Model((*models.Author)(nil)).
			Set("person_id = ?", targetID).
			Where("person_id = ?", sourceID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Update all narrators from source to target
		_, err = tx.NewUpdate().
			Model((*models.Narrator)(nil)).
			Set("person_id = ?", targetID).
			Where("person_id = ?", sourceID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete the source person
		_, err = tx.NewDelete().
			Model((*models.Person)(nil)).
			Where("id = ?", sourceID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// CleanupOrphanedPeople deletes people with no authors or narrators.
func (svc *Service) CleanupOrphanedPeople(ctx context.Context) (int, error) {
	result, err := svc.db.NewDelete().
		Model((*models.Person)(nil)).
		Where("id NOT IN (SELECT DISTINCT person_id FROM authors)").
		Where("id NOT IN (SELECT DISTINCT person_id FROM narrators)").
		Exec(ctx)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// buildFTSPrefixQuery builds an FTS5 query for prefix/typeahead search.
// It sanitizes the input to prevent FTS5 injection and appends a wildcard.
func buildFTSPrefixQuery(input string) string {
	const maxQueryLength = 100

	// Trim and limit length
	input = strings.TrimSpace(input)
	if len(input) > maxQueryLength {
		input = input[:maxQueryLength]
	}
	if input == "" {
		return ""
	}

	// Escape double quotes (used for phrase matching in FTS5)
	input = strings.ReplaceAll(input, `"`, `""`)

	// Wrap in double quotes and add prefix wildcard: "query"*
	return `"` + input + `"*`
}
