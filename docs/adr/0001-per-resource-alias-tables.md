# Per-resource alias tables instead of a single polymorphic table

We considered two storage models for aliases: a single polymorphic `aliases` table with a `resource_type` discriminator column, or dedicated per-resource tables (`genre_aliases`, `tag_aliases`, `series_aliases`, `person_aliases`, `publisher_aliases`, `imprint_aliases`).

We chose per-resource tables because alias uniqueness must span both primary names and aliases within the same resource type and library — a polymorphic table can't enforce this at the database level without triggers. Per-resource tables allow natural foreign keys with `ON DELETE CASCADE`, keep lookup queries simple (each `FindOrCreate` only JOINs its own alias table), and match the existing codebase pattern where each resource type has its own service, handlers, and migrations.

## Considered Options

- **Polymorphic table**: Fewer tables, but uniqueness constraints require application-level enforcement or SQLite triggers. Foreign keys can't reference multiple parent tables. Queries need a `WHERE resource_type = ?` filter on every access.
- **Per-resource tables**: More tables (6), but each is small, has proper FK constraints, and uniqueness is enforced by a simple unique index. Matches the existing one-service-per-resource-type architecture.
