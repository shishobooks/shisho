import GRDB

extension CatalogStore {
    /// This device's place in each File to the whole percent covers and
    /// lists show, kept by triggers on local_positions. The player saves its
    /// place every few seconds; a list that reads local_progress instead of
    /// local_positions runs again only when the percent it shows changes.
    static func createLocalProgress(_ db: Database) throws {
        try db.create(table: "local_progress") { t in
            t.primaryKey("file_id", .integer)
            t.column("percentage", .double).notNull()
        }
        try db.execute(sql: """
            INSERT INTO local_progress (file_id, percentage) SELECT file_id, ROUND(percentage, 2) FROM local_positions;

            -- Each trigger deletes, then inserts: an OR REPLACE in a trigger
            -- yields to the conflict clause of the statement that fired it.

            CREATE TRIGGER local_progress_insert AFTER INSERT ON local_positions BEGIN
                DELETE FROM local_progress WHERE file_id = new.file_id;
                INSERT INTO local_progress (file_id, percentage) VALUES (new.file_id, ROUND(new.percentage, 2));
            END;

            -- Only a change in the whole percent writes, so a move within
            -- one percent tells no one.
            CREATE TRIGGER local_progress_update AFTER UPDATE OF percentage ON local_positions
            WHEN ROUND(new.percentage, 2) IS NOT (SELECT percentage FROM local_progress WHERE file_id = new.file_id)
            BEGIN
                DELETE FROM local_progress WHERE file_id = new.file_id;
                INSERT INTO local_progress (file_id, percentage) VALUES (new.file_id, ROUND(new.percentage, 2));
            END;

            CREATE TRIGGER local_progress_delete AFTER DELETE ON local_positions BEGIN
                DELETE FROM local_progress WHERE file_id = old.file_id;
            END;
            """)
    }
}
