# Per-resource alias tables instead of a single polymorphic table

We considered two storage models for aliases: a single polymorphic `aliases` table with a `resource_type` discriminator column, or dedicated per-resource tables (`genre_aliases`, `tag_aliases`, `series_aliases`, `person_aliases`, `publisher_aliases`, `imprint_aliases`).

We chose per-resource tables because they allow natural foreign keys with `ON DELETE CASCADE`, alias-to-alias uniqueness enforced by a simple DB unique index per table, and straightforward lookup queries (each `FindOrCreate` only JOINs its own alias table). Cross-table uniqueness (alias vs. primary name) requires application-level validation either way, but per-resource tables avoid the additional complexity of a discriminator column and `WHERE resource_type = ?` filters on every query. The approach also matches the existing codebase pattern where each resource type has its own service, handlers, and migrations.

## Considered Options

- **Polymorphic table**: Fewer tables, but uniqueness constraints require application-level enforcement or SQLite triggers. Foreign keys can't reference multiple parent tables. Queries need a `WHERE resource_type = ?` filter on every access.
- **Per-resource tables**: More tables (6), but each is small, has proper FK constraints, and uniqueness is enforced by a simple unique index. Matches the existing one-service-per-resource-type architecture.
