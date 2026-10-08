package models

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

const (
	//tygo:emit export type FileType = typeof FileTypeAZW3 | typeof FileTypeCBZ | typeof FileTypeEPUB | typeof FileTypeM4B | typeof FileTypeMOBI | typeof FileTypePDF;
	FileTypeAZW3 = "azw3"
	FileTypeCBZ  = "cbz"
	FileTypeEPUB = "epub"
	FileTypeM4B  = "m4b"
	FileTypeMOBI = "mobi"
	FileTypePDF  = "pdf"
)

// builtInFileTypesByExtension maps every extension Shisho parses itself
// (lowercase, no dot) to its file type. A file type is not always its
// extension: .azw and .prc files are MOBI.
var builtInFileTypesByExtension = map[string]string{
	"azw3": FileTypeAZW3,
	"cbz":  FileTypeCBZ,
	"epub": FileTypeEPUB,
	"m4b":  FileTypeM4B,
	"mobi": FileTypeMOBI,
	"azw":  FileTypeMOBI,
	"prc":  FileTypeMOBI,
	"pdf":  FileTypePDF,
}

// FileTypeForPath returns the file type for a path: the built-in type its
// extension maps to, or else the lowercase extension without the dot, which
// is how plugin-parsed files and supplements are typed.
func FileTypeForPath(path string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	if fileType, ok := builtInFileTypesByExtension[ext]; ok {
		return fileType
	}
	return ext
}

// IsBuiltInFileExtension reports whether an extension (lowercase, no dot)
// belongs to a built-in file type. Plugin file parsers cannot claim these.
func IsBuiltInFileExtension(ext string) bool {
	_, ok := builtInFileTypesByExtension[ext]
	return ok
}

// BuiltInFileExtensions returns every extension (lowercase, no dot) of a
// built-in file type, sorted.
func BuiltInFileExtensions() []string {
	return slices.Sorted(maps.Keys(builtInFileTypesByExtension))
}

// BuiltInFileTypes are the file types Shisho parses itself. Each is eligible
// to be a main file.
var BuiltInFileTypes = []string{FileTypeAZW3, FileTypeCBZ, FileTypeEPUB, FileTypeM4B, FileTypeMOBI, FileTypePDF}

// IsBuiltInFileType reports whether Shisho parses this file type itself.
func IsBuiltInFileType(fileType string) bool {
	return slices.Contains(BuiltInFileTypes, fileType)
}

// EbookFileTypes are the file types in the ebook cover category, as opposed
// to M4B audiobooks. Preferred Cover is exclusive within a category.
var EbookFileTypes = []string{FileTypeEPUB, FileTypeAZW3, FileTypeMOBI, FileTypeCBZ, FileTypePDF}

// IsEbookFileType reports whether a file type is in the ebook cover category.
func IsEbookFileType(fileType string) bool {
	return slices.Contains(EbookFileTypes, fileType)
}

// EbookCoverRank orders ebook files when no file is the Preferred Cover:
// EPUB, then AZW3, then MOBI, then the other ebook formats, which share a rank
// so a stable sort keeps their order. app/utils/coverSelection.ts mirrors it.
func EbookCoverRank(fileType string) int {
	switch fileType {
	case FileTypeEPUB:
		return 0
	case FileTypeAZW3:
		return 1
	case FileTypeMOBI:
		return 2
	}
	return 3
}

const (
	//tygo:emit export type FileRole = typeof FileRoleMain | typeof FileRoleSupplement;
	FileRoleMain       = "main"
	FileRoleSupplement = "supplement"
)

const (
	//tygo:emit export type ReviewOverride = typeof ReviewOverrideReviewed | typeof ReviewOverrideUnreviewed;
	ReviewOverrideReviewed   = "reviewed"
	ReviewOverrideUnreviewed = "unreviewed"
)

// ReviewedFilter values for the book list endpoint's reviewed_filter query
// param. "" and "all" both mean "all books"; the other two scope to
// needs-review or reviewed books respectively.
const (
	//tygo:emit export type ReviewedFilter = typeof ReviewedFilterAll | typeof ReviewedFilterNeedsReview | typeof ReviewedFilterReviewed;
	ReviewedFilterAll         = "all"
	ReviewedFilterNeedsReview = "needs_review"
	ReviewedFilterReviewed    = "reviewed"
)

type File struct {
	bun.BaseModel `bun:"table:files,alias:f" tstype:"-"`

	ID                       int               `bun:",pk,nullzero" json:"id"`
	CreatedAt                time.Time         `json:"created_at"`
	UpdatedAt                time.Time         `json:"updated_at"`
	LibraryID                int               `bun:",nullzero" json:"library_id"`
	BookID                   int               `bun:",nullzero" json:"book_id"`
	Book                     *Book             `bun:"rel:belongs-to" json:"book" tstype:"Book"`
	Filepath                 string            `bun:",nullzero" json:"filepath"`
	FileType                 string            `bun:",nullzero" json:"file_type" tstype:"FileType"`
	FileRole                 string            `bun:",nullzero,default:'main'" json:"file_role" tstype:"FileRole"`
	FilesizeBytes            int64             `json:"filesize_bytes"`
	FileModifiedAt           *time.Time        `json:"file_modified_at"`
	ScanError                *string           `json:"scan_error"` // Why the last scan could not parse the file; nil when it parsed fine
	CoverImageFilename       *string           `json:"cover_image_filename"`
	CoverMimeType            *string           `json:"cover_mime_type"`
	CoverSource              *string           `json:"cover_source" tstype:"DataSource"`
	CoverPage                *int              `json:"cover_page"` // 0-indexed page number for CBZ/PDF cover, NULL for EPUB/M4B
	Name                     *string           `json:"name"`
	NameSource               *string           `json:"name_source" tstype:"DataSource"`
	PageCount                *int              `json:"page_count"` // Number of pages for CBZ/PDF files, NULL for EPUB/M4B
	AudiobookDurationSeconds *float64          `json:"audiobook_duration_seconds"`
	AudiobookBitrateBps      *int              `json:"audiobook_bitrate_bps"`
	AudiobookCodec           *string           `json:"audiobook_codec"`
	Narrators                []*Narrator       `bun:"rel:has-many,join:id=file_id" json:"narrators,omitempty" tstype:"Narrator[]"`
	NarratorSource           *string           `json:"narrator_source" tstype:"DataSource"`
	Identifiers              []*FileIdentifier `bun:"rel:has-many,join:id=file_id" json:"identifiers,omitempty" tstype:"FileIdentifier[]"`
	IdentifierSource         *string           `json:"identifier_source" tstype:"DataSource"`
	Chapters                 []*Chapter        `bun:"rel:has-many,join:id=file_id" json:"chapters,omitempty" tstype:"Chapter[]"`
	URL                      *string           `json:"url"`
	URLSource                *string           `json:"url_source" tstype:"DataSource"`
	ReleaseDate              *time.Time        `json:"release_date"`
	ReleaseDateSource        *string           `json:"release_date_source" tstype:"DataSource"`
	PublisherID              *int              `json:"publisher_id"`
	PublisherSource          *string           `json:"publisher_source" tstype:"DataSource"`
	Publisher                *Publisher        `bun:"rel:belongs-to,join:publisher_id=id" json:"publisher,omitempty" tstype:"Publisher"`
	ChapterSource            *string           `json:"chapter_source" tstype:"DataSource"`
	Language                 *string           `json:"language"`
	LanguageSource           *string           `json:"language_source" tstype:"DataSource"`
	Abridged                 *bool             `json:"abridged"`
	AbridgedSource           *string           `json:"abridged_source" tstype:"DataSource"`
	ReviewOverride           *string           `json:"review_override" tstype:"ReviewOverride"`
	ReviewOverriddenAt       *time.Time        `json:"review_overridden_at"`
	Reviewed                 *bool             `json:"reviewed"`
	IsPreferredCover         bool              `bun:",default:false" json:"is_preferred_cover"`
	// DisplayName is the label the UI shows for the file. It is not stored:
	// AfterScanRow fills it for files loaded directly, and the book loaders
	// call ResolveBookFileDisplayNames for Book.Files, which Bun loads as a
	// has-many relation without row hooks.
	DisplayName string `bun:"-" json:"display_name"`
}

// ResolveDisplayName returns the label for the file. A main file shows its
// name (the edition title), falling back to the filename. A supplement shows
// its filename unless the user set a name (source manual): the scanner stores
// the filename stem as the name when it first sees a supplement, and that
// stored value goes stale when the book is renamed or reorganized. A sidecar
// source does not count, because book edits write every file's sidecar with
// its stored name, so a rescan can restore the stale stem as sidecar.
func (f *File) ResolveDisplayName() string {
	if f.Name != nil && *f.Name != "" {
		manual := f.NameSource != nil && *f.NameSource == DataSourceManual
		if f.FileRole != FileRoleSupplement || manual {
			return *f.Name
		}
	}
	if f.Filepath == "" {
		return ""
	}
	return filepath.Base(f.Filepath)
}

func (f *File) CoverExtension() string {
	if f.CoverMimeType == nil {
		return ""
	}
	ext := ""
	switch *f.CoverMimeType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	}
	return ext
}

// FileTypeMimeType returns the media type a file of this type is served as,
// or "" for a type Shisho does not know (a supplement's extension or a format
// only a plugin parses), which is typed by its extension instead. Downloads
// use it rather than the host's mime table, which in the Alpine image has no
// entry for .epub, .cbz, or .m4b.
func FileTypeMimeType(fileType string) string {
	switch fileType {
	case FileTypeEPUB:
		return "application/epub+zip"
	case FileTypeCBZ:
		return "application/vnd.comicbook+zip"
	case FileTypeM4B:
		return "audio/mp4"
	case FileTypePDF:
		return "application/pdf"
	case FileTypeMOBI:
		return "application/x-mobipocket-ebook"
	case FileTypeAZW3:
		return "application/vnd.amazon.mobi8-ebook"
	}
	return ""
}

// IsPageBasedFileType returns true for file types that derive covers from page
// content (CBZ, PDF). These formats should never have their covers replaced by
// external sources (plugins, uploads).
func IsPageBasedFileType(fileType string) bool {
	return fileType == FileTypeCBZ || fileType == FileTypePDF
}

// AfterScanRow is a Bun hook that resolves DisplayName for files loaded by a
// direct query (a single file, or a slice of files). It does not run for
// Book.Files: Bun scans has-many relations without row hooks.
func (f *File) AfterScanRow(_ context.Context) error {
	f.DisplayName = f.ResolveDisplayName()
	return nil
}

// ResolveFileDisplayNames sets DisplayName on each file.
func ResolveFileDisplayNames(files []*File) {
	for _, f := range files {
		if f != nil {
			f.DisplayName = f.ResolveDisplayName()
		}
	}
}

// ResolveBookFileDisplayNames sets DisplayName on every file of each book.
// Call it wherever books are loaded with their Files relation.
func ResolveBookFileDisplayNames(books ...*Book) {
	for _, b := range books {
		if b != nil {
			ResolveFileDisplayNames(b.Files)
		}
	}
}
