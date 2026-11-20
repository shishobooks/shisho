import Foundation
import SQLite3

private let transient = unsafeBitCast(-1, to: sqlite3_destructor_type.self)

/// A value bound to, or read from, a SQLite statement.
enum SQLValue: Equatable {
    case int(Int)
    case double(Double)
    case text(String)
    case null

    init(_ value: String?) { self = value.map { .text($0) } ?? .null }
    init(_ value: Int?) { self = value.map { .int($0) } ?? .null }
    init(_ value: Double?) { self = value.map { .double($0) } ?? .null }
}

/// One result row, read by column index.
struct Row {
    fileprivate let statement: OpaquePointer

    func int(_ column: Int32) -> Int { Int(sqlite3_column_int64(statement, column)) }
    func double(_ column: Int32) -> Double { sqlite3_column_double(statement, column) }
    func string(_ column: Int32) -> String {
        sqlite3_column_text(statement, column).map { String(cString: $0) } ?? ""
    }
    func optionalString(_ column: Int32) -> String? {
        sqlite3_column_type(statement, column) == SQLITE_NULL ? nil : string(column)
    }
    func optionalDouble(_ column: Int32) -> Double? {
        sqlite3_column_type(statement, column) == SQLITE_NULL ? nil : double(column)
    }
}

struct DatabaseError: Error, CustomStringConvertible {
    let description: String
}

/// One SQLite file, owned by native code (ADR 0021). Calls are serialized,
/// so the sync and the UI can share it.
final class Database: @unchecked Sendable {
    private var handle: OpaquePointer?
    private let lock = NSRecursiveLock()

    init(path: String) throws {
        let flags = SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE | SQLITE_OPEN_NOMUTEX
        guard sqlite3_open_v2(path, &handle, flags, nil) == SQLITE_OK else {
            let message = handle.map { String(cString: sqlite3_errmsg($0)) } ?? "cannot open \(path)"
            sqlite3_close(handle)
            throw DatabaseError(description: message)
        }
        sqlite3_busy_timeout(handle, 5000)
        try execute("PRAGMA journal_mode = WAL")
        try execute("PRAGMA foreign_keys = ON")
    }

    deinit {
        sqlite3_close(handle)
    }

    var userVersion: Int {
        get { (try? query("PRAGMA user_version") { $0.int(0) }.first) ?? 0 }
        set { try? execute("PRAGMA user_version = \(newValue)") }
    }

    func execute(_ sql: String, _ params: [SQLValue] = []) throws {
        try withStatement(sql, params) { statement in
            var result = sqlite3_step(statement)
            while result == SQLITE_ROW {
                result = sqlite3_step(statement)
            }
            guard result == SQLITE_DONE else { throw error() }
        }
    }

    /// Runs several statements with no parameters, such as a schema.
    func executeScript(_ sql: String) throws {
        try locked {
            guard sqlite3_exec(handle, sql, nil, nil, nil) == SQLITE_OK else { throw error() }
        }
    }

    func query<T>(_ sql: String, _ params: [SQLValue] = [], _ map: (Row) throws -> T) throws -> [T] {
        try withStatement(sql, params) { statement in
            var rows: [T] = []
            while true {
                let result = sqlite3_step(statement)
                if result == SQLITE_DONE { break }
                guard result == SQLITE_ROW else { throw error() }
                rows.append(try map(Row(statement: statement)))
            }
            return rows
        }
    }

    /// Runs body in one write transaction, rolled back if it throws.
    func transaction<T>(_ body: () throws -> T) throws -> T {
        try locked {
            try execute("BEGIN IMMEDIATE")
            do {
                let value = try body()
                try execute("COMMIT")
                return value
            } catch {
                try? execute("ROLLBACK")
                throw error
            }
        }
    }

    private func withStatement<T>(_ sql: String, _ params: [SQLValue], _ body: (OpaquePointer) throws -> T) throws -> T {
        try locked {
            var statement: OpaquePointer?
            guard sqlite3_prepare_v2(handle, sql, -1, &statement, nil) == SQLITE_OK, let statement else {
                throw error()
            }
            defer { sqlite3_finalize(statement) }
            for (index, value) in params.enumerated() {
                let position = Int32(index + 1)
                switch value {
                case .int(let v): sqlite3_bind_int64(statement, position, Int64(v))
                case .double(let v): sqlite3_bind_double(statement, position, v)
                case .text(let v): sqlite3_bind_text(statement, position, v, -1, transient)
                case .null: sqlite3_bind_null(statement, position)
                }
            }
            return try body(statement)
        }
    }

    private func locked<T>(_ body: () throws -> T) rethrows -> T {
        lock.lock()
        defer { lock.unlock() }
        return try body()
    }

    private func error() -> DatabaseError {
        DatabaseError(description: handle.map { String(cString: sqlite3_errmsg($0)) } ?? "SQLite error")
    }
}
