# Metadata Entity Editing Design

## Overview

Allow direct editing, merging, and deletion of metadata entities (Person, Series, Genre, Tag) from their detail pages. Currently, fixing a typo in an entity name requires editing every linked book individually. With this feature, users can edit the entity once and the change propagates everywhere.

## Requirements

- **Edit**: Rename entities (name, sort_name where applicable)
- **Merge**: Combine multiple duplicate entities into one, transferring all book relationships
- **Delete**: Remove orphaned entities (those with no associated books)
- **UI Location**: Detail pages with edit button (PersonDetail, SeriesDetail, new GenreDetail, new TagDetail)

## Data Model

Existing many-to-many relationships via join tables:
- `authors` (book_id, person_id, role)
- `narrators` (file_id, person_id)
- `book_series` (book_id, series_id, series_number)
- `book_genres` (book_id, genre_id)
- `book_tags` (book_id, tag_id)

No schema changes needed - operations update existing tables.

## API Endpoints

### Person
| Method | Endpoint | Body | Behavior |
|--------|----------|------|----------|
| `PATCH` | `/api/persons/:id` | `{name, sort_name}` | Update person fields |
| `POST` | `/api/persons/:id/merge` | `{source_ids: [1, 2, 3]}` | Merge multiple sources into target |
| `DELETE` | `/api/persons/:id` | - | Delete if orphaned |

### Series
| Method | Endpoint | Body | Behavior |
|--------|----------|------|----------|
| `PATCH` | `/api/series/:id` | `{name, sort_name, description}` | Update series fields |
| `POST` | `/api/series/:id/merge` | `{source_ids: [1, 2, 3]}` | Merge multiple sources into target |
| `DELETE` | `/api/series/:id` | - | Delete if orphaned |

### Genre
| Method | Endpoint | Body | Behavior |
|--------|----------|------|----------|
| `PATCH` | `/api/genres/:id` | `{name}` | Update genre name |
| `POST` | `/api/genres/:id/merge` | `{source_ids: [1, 2, 3]}` | Merge multiple sources into target |
| `DELETE` | `/api/genres/:id` | - | Delete if orphaned |

### Tag
| Method | Endpoint | Body | Behavior |
|--------|----------|------|----------|
| `PATCH` | `/api/tags/:id` | `{name}` | Update tag name |
| `POST` | `/api/tags/:id/merge` | `{source_ids: [1, 2, 3]}` | Merge multiple sources into target |
| `DELETE` | `/api/tags/:id` | - | Delete if orphaned |

## Backend Implementation

### Merge Logic (transactional)

```go
func (s *Service) MergePeople(ctx context.Context, targetID int, sourceIDs []int) (*Person, error) {
    return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
        // 1. Validate all entities exist and are in same library
        // 2. For each source:
        //    a. Update authors: SET person_id = target WHERE person_id = source
        //       (skip if would create duplicate book+person+role combo)
        //    b. Update narrators: same logic
        //    c. Delete source person
        // 3. Queue sidecar update job for affected files
        // 4. Return updated target with refreshed counts
    })
}
```

### Delete Validation

Return 400 error if entity still has associated books.

### Sidecar Updates

On rename/merge, queue a background job to update sidecars for all affected files:
- Job type: `update_sidecars` with payload containing file IDs and updated metadata
- For Person: update `authors` and `narrators` arrays in sidecars
- For Series: update `series` array in sidecars
- For Genre: update `genres` array in sidecars
- For Tag: update `tags` array in sidecars

## Frontend Implementation

### New Files
- `app/components/pages/GenreDetail.tsx` - New detail page
- `app/components/pages/TagDetail.tsx` - New detail page
- `app/components/library/MetadataEditDialog.tsx` - Reusable edit dialog for all entity types
- `app/components/library/MetadataMergeDialog.tsx` - Reusable merge dialog with multi-select

### New Hooks (in `app/hooks/queries/`)
- `useUpdatePerson`, `useMergePeople`, `useDeletePerson`
- `useUpdateSeries`, `useMergeSeries`, `useDeleteSeries`
- `useUpdateGenre`, `useMergeGenres`, `useDeleteGenre`
- `useUpdateTag`, `useMergeTags`, `useDeleteTag`
- `useGenre` (single genre fetch for detail page)
- `useTag` (single tag fetch for detail page)

### Router Changes (`app/router.tsx`)
- Add `/libraries/:libraryId/genres/:id` → `GenreDetail`
- Add `/libraries/:libraryId/tags/:id` → `TagDetail`

### List Page Changes
- `GenresList.tsx`: Link to `/libraries/:libraryId/genres/:id` instead of `?genre_ids=`
- `TagsList.tsx`: Link to `/libraries/:libraryId/tags/:id` instead of `?tag_ids=`

## UI Design

### Detail Page Header Actions
Each detail page header will have an actions area with:
- **Edit button** → Opens edit dialog with name field (and sort_name for Person/Series)
- **Merge button** → Opens merge dialog to select entities to merge into this one
- **Delete button** → Only visible when book_count is 0, with confirmation

### Edit Dialog
```
┌─────────────────────────────────────┐
│ Edit Person                         │
├─────────────────────────────────────┤
│ Name:      [Stephen King________]   │
│ Sort Name: [King, Stephen_______]   │
│                                     │
│            [Cancel]  [Save]         │
└─────────────────────────────────────┘
```

### Merge Dialog (multi-select)
```
┌─────────────────────────────────────┐
│ Merge into "Stephen King"           │
├─────────────────────────────────────┤
│ Select entities to merge:           │
│ [Search..._______________]          │
│                                     │
│ ☑ Steven King (3 books)             │
│ ☑ S. King (1 book)                  │
│ ☐ Stephen R. King (2 books)         │
│                                     │
│ 2 selected — will move all books    │
│ to "Stephen King" and delete them.  │
│                                     │
│            [Cancel]  [Merge]        │
└─────────────────────────────────────┘
```

## Authorization

- Edit/merge/delete require write permission on the library
- Use existing RBAC checks consistent with book editing

## Edge Cases

| Scenario | Handling |
|----------|----------|
| Merge source = target | Return 400 error |
| Source in different library | Return 400 error |
| Merge creates duplicate author+role on same book | Skip duplicate, keep one link |
| Delete entity with books | Return 400 error with message |
| Concurrent merge of same entity | Transaction isolation handles it |
| Name collision after rename | Allow it (duplicates are what merge is for) |

## OPDS Impact

None - OPDS serves books, entity names come from relations, so edits propagate automatically.
