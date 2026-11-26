import Foundation
import GRDB
import ShishoAPI

/// One snapshot of a sitting's Reading Session made on this device: waiting
/// in the journal, or taken in by the server and kept in unstated_sessions
/// until every copy of Reading Stats and the calendar counts it.
struct SessionSnapshot: Equatable, Sendable {
    let eventID: String
    let payload: Shishoapp.JournalSessionPayload
    /// When the server took it in, or nil while it waits to be sent.
    let acceptedAt: Double?

    /// The sitting it belongs to: every snapshot of a sitting carries its
    /// id, and one without stands alone.
    var key: String {
        payload.sessionId.isEmpty ? "event:" + eventID : payload.sessionId
    }
}

/// A sitting a copy of the server's stats doesn't count all of: its latest
/// snapshot, and the seconds of it the copy already counts, which an
/// earlier snapshot the server took in before the copy was fetched holds.
struct UncountedSession: Equatable, Sendable {
    let key: String
    let payload: Shishoapp.JournalSessionPayload
    let counted: Double

    var seconds: Double { CatalogStore.seconds(payload) }
}

/// This device's reading that a copy of the server's stats doesn't count
/// yet, shared by Reading Stats' Overview and its Calendar so both drop a
/// sitting by the same rule once the server's copy counts it.
enum UnsyncedReading {
    /// The sittings a copy fetched at fetchedAt doesn't count all of. A
    /// snapshot the server took in before the fetch is counted by it, as is
    /// what counted (counted_sessions) says of sittings taken in before
    /// every copy was fetched; each sitting's latest snapshot adds only what
    /// it has past that. Sorted by key.
    static func uncounted(_ snapshots: [SessionSnapshot], counted: [String: Double], fetchedAt: Double) -> [UncountedSession] {
        var counted = counted
        var latest: [String: Shishoapp.JournalSessionPayload] = [:]
        for snapshot in snapshots {
            let key = snapshot.key
            if let accepted = snapshot.acceptedAt, accepted < fetchedAt {
                counted[key] = max(counted[key] ?? 0, CatalogStore.seconds(snapshot.payload))
                continue
            }
            if let kept = latest[key],
               (CatalogStore.timestamp(kept.endedAt) ?? 0) >= (CatalogStore.timestamp(snapshot.payload.endedAt) ?? 0) { continue }
            latest[key] = snapshot.payload
        }
        return latest.keys.sorted().compactMap { key in
            guard let payload = latest[key] else { return nil }
            return UncountedSession(key: key, payload: payload, counted: counted[key] ?? 0)
        }
    }

    /// How a stretch of time from start to end, seconds long, falls on the
    /// days of zone, split at local midnight in proportion to the time on
    /// each, as the server's splitByDay does. A stretch that doesn't move
    /// forward is all on its first day.
    static func splitByDay(start: Date, end: Date, seconds total: Double, zone: TimeZone) -> [(day: String, seconds: Double)] {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = zone
        func day(_ date: Date) -> String {
            let parts = calendar.dateComponents([.year, .month, .day], from: date)
            return String(format: "%04d-%02d-%02d", parts.year ?? 0, parts.month ?? 0, parts.day ?? 0)
        }
        guard end > start else { return [(day(start), total)] }
        let span = end.timeIntervalSince(start)
        var parts: [(day: String, seconds: Double)] = []
        var current = start
        while current < end {
            var next = calendar.date(byAdding: .day, value: 1, to: calendar.startOfDay(for: current)) ?? end
            if next <= current { next = current.addingTimeInterval(3600) }
            if next > end { next = end }
            parts.append((day(current), total * next.timeIntervalSince(current) / span))
            current = next
        }
        return parts
    }

    /// Where the time a copy doesn't count of a sitting begins: past the
    /// share of it from start to end that counted seconds, of its total,
    /// already brought.
    static func uncountedStart(start: Date, end: Date, seconds total: Double, counted: Double) -> Date {
        let span = end.timeIntervalSince(start)
        guard span > 0, total > 0, counted > 0 else { return start }
        return start.addingTimeInterval(span * min(counted / total, 1))
    }

    /// The day an instant falls on in zone.
    static func day(_ instant: Date, zone: TimeZone) -> String {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = zone
        let parts = calendar.dateComponents([.year, .month, .day], from: instant)
        return String(format: "%04d-%02d-%02d", parts.year ?? 0, parts.month ?? 0, parts.day ?? 0)
    }
}

extension CatalogStore {
    /// Every snapshot of this device's Reading Sessions the stats may not
    /// count yet: those waiting in the journal, and those the server took in
    /// that some copy may not count.
    static func sessionSnapshots(_ db: Database) throws -> [SessionSnapshot] {
        let decoder = makeShishoDecoder()
        return try Row.fetchAll(db, sql: """
            SELECT event_id, payload, NULL AS accepted_at FROM journal WHERE type = 'reading_session'
            UNION ALL SELECT event_id, payload, accepted_at FROM unstated_sessions
            """).compactMap { row in
            guard let payload = try? decoder.decode(Shishoapp.JournalSessionPayload.self, from: row["payload"] as Data) else { return nil }
            return SessionSnapshot(eventID: row["event_id"], payload: payload, acceptedAt: row["accepted_at"])
        }
    }

    /// The seconds of each sitting every copy counts, by its id.
    static func countedSessions(_ db: Database) throws -> [String: Double] {
        var counted: [String: Double] = [:]
        for row in try Row.fetchAll(db, sql: "SELECT session_id, seconds FROM counted_sessions") {
            counted[row["session_id"]] = row["seconds"]
        }
        return counted
    }

    /// Whether each File is listened to, an audiobook, by id.
    static func listenedFiles(_ db: Database, ids: Set<Int>) throws -> [Int: Bool] {
        guard !ids.isEmpty else { return [:] }
        var listening: [Int: Bool] = [:]
        for row in try Row.fetchAll(db, sql: "SELECT id, file_type FROM book_files WHERE id IN (\(ids.map(String.init).joined(separator: ",")))") {
            listening[row["id"]] = (row["file_type"] as String) == "m4b"
        }
        return listening
    }
}
