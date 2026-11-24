import Compression
import Foundation

/// A zip archive read where it lies on the device, for a CBZ's pages: the
/// file is mapped, not read into memory, and an entry's bytes are copied
/// out only when its page is decoded.
///
/// Only what a CBZ needs: stored and deflated entries, and zip64 for
/// archives past 4 GB. Every offset is checked against the file, so a
/// damaged archive throws rather than reads out of bounds.
struct ZipArchive: Sendable {
    struct Entry: Equatable, Sendable {
        let name: String
        let method: UInt16
        let compressedSize: Int
        let uncompressedSize: Int
        let localHeaderOffset: Int
        let encrypted: Bool
    }

    enum Failure: Error, Equatable {
        case notAZip
        case damaged(String)
        case unsupported(String)
    }

    private let data: Data
    /// The entries in the central directory's order. Two can share a name,
    /// as the server keeps them.
    let entries: [Entry]

    init(url: URL) throws {
        try self.init(data: Data(contentsOf: url, options: .alwaysMapped))
    }

    init(data: Data) throws {
        // From 0, whatever the Data it came as, so offsets index it directly.
        self.data = data.startIndex == 0 ? data : Data(data)
        entries = try Self.directory(Reader(data: self.data))
    }

    /// The entries named names, in the order given. A name the archive
    /// holds more than once takes its entries in the archive's order, one
    /// for each time it's given, so two pages with one name are still two
    /// pages, as the server counts them. A name it lacks, or has run out
    /// of, is nil.
    /// Names are compared by their bytes, as the server compares them: two
    /// spellings of é that Swift counts as one string are two names.
    func entries(named names: [String]) -> [Entry?] {
        var byName: [[UInt8]: [Entry]] = [:]
        for entry in entries { byName[Array(entry.name.utf8), default: []].append(entry) }
        var used: [[UInt8]: Int] = [:]
        return names.map { name in
            let key = Array(name.utf8)
            let index = used[key, default: 0]
            used[key] = index + 1
            guard let matches = byName[key], index < matches.count else { return nil }
            return matches[index]
        }
    }

    /// The largest entry read in one piece, past which an entry is treated
    /// as damaged: no page image comes near it, and a page's bytes are held
    /// in memory while it decodes, two pages at a time in a spread.
    static let largestEntry = 128 * 1024 * 1024

    /// The entry's bytes, inflated when they're deflated.
    func bytes(of entry: Entry) throws -> Data {
        let (start, compressed) = try contents(of: entry)
        guard entry.uncompressedSize <= Self.largestEntry, compressed.count <= Self.largestEntry else {
            throw Failure.damaged("\(entry.name) is too large")
        }
        switch entry.method {
        case 0:
            return Data(compressed)
        case 8:
            if entry.uncompressedSize == 0 { return Data() }
            var output = Data(count: entry.uncompressedSize)
            let written = output.withUnsafeMutableBytes { out in
                data.withUnsafeBytes { input in
                    compression_decode_buffer(out.bindMemory(to: UInt8.self).baseAddress!, entry.uncompressedSize,
                                              input.bindMemory(to: UInt8.self).baseAddress! + start, entry.compressedSize,
                                              nil, COMPRESSION_ZLIB)
                }
            }
            guard written == entry.uncompressedSize else { throw Failure.damaged("\(entry.name) didn't inflate") }
            return output
        default:
            throw Failure.unsupported("\(entry.name) is compressed with method \(entry.method)")
        }
    }

    /// Up to limit bytes from the entry's start, which is all an image's
    /// header needs: its size is read without inflating the whole page.
    func prefix(of entry: Entry, limit: Int) throws -> Data {
        let (start, compressed) = try contents(of: entry)
        switch entry.method {
        case 0:
            return Data(compressed.prefix(limit))
        case 8:
            let wanted = min(limit, entry.uncompressedSize)
            if wanted == 0 { return Data() }
            var output = Data(count: wanted)
            let stream = UnsafeMutablePointer<compression_stream>.allocate(capacity: 1)
            defer { stream.deallocate() }
            guard compression_stream_init(stream, COMPRESSION_STREAM_DECODE, COMPRESSION_ZLIB) == COMPRESSION_STATUS_OK else {
                throw Failure.damaged("\(entry.name) couldn't start inflating")
            }
            defer { compression_stream_destroy(stream) }
            let produced = output.withUnsafeMutableBytes { out in
                data.withUnsafeBytes { input -> Int in
                    stream.pointee.src_ptr = input.bindMemory(to: UInt8.self).baseAddress! + start
                    stream.pointee.src_size = entry.compressedSize
                    stream.pointee.dst_ptr = out.bindMemory(to: UInt8.self).baseAddress!
                    stream.pointee.dst_size = wanted
                    let status = compression_stream_process(stream, 0)
                    return status == COMPRESSION_STATUS_ERROR ? -1 : wanted - stream.pointee.dst_size
                }
            }
            guard produced >= 0 else { throw Failure.damaged("\(entry.name) didn't inflate") }
            return output.prefix(produced)
        default:
            throw Failure.unsupported("\(entry.name) is compressed with method \(entry.method)")
        }
    }

    /// Where the entry's stored bytes start, and those bytes.
    private func contents(of entry: Entry) throws -> (Int, Data) {
        guard !entry.encrypted else { throw Failure.unsupported("\(entry.name) is encrypted") }
        let reader = Reader(data: data)
        let header = entry.localHeaderOffset
        guard try reader.u32(header) == 0x0403_4B50 else { throw Failure.damaged("\(entry.name) has no local header") }
        let start = header + 30 + (try reader.u16(header + 26)) + (try reader.u16(header + 28))
        guard start >= 0, entry.compressedSize >= 0, start <= data.count, entry.compressedSize <= data.count - start else {
            throw Failure.damaged("\(entry.name) runs past the end of the archive")
        }
        return (start, data[start..<(start + entry.compressedSize)])
    }

    // MARK: - The central directory

    private static func directory(_ reader: Reader) throws -> [Entry] {
        let end = try endOfDirectory(reader)
        var count = try reader.u16(end + 10)
        var offset = try reader.u32(end + 16)
        // Values at their maximum say the real ones are in zip64's record.
        if count == 0xFFFF || offset == 0xFFFF_FFFF {
            let locator = end - 20
            if locator >= 0, try reader.u32(locator) == 0x0706_4B50 {
                let record = try reader.u64(locator + 8)
                guard try reader.u32(record) == 0x0606_4B50 else { throw Failure.damaged("no zip64 end of directory") }
                count = try reader.u64(record + 32)
                offset = try reader.u64(record + 48)
            }
        }
        var entries: [Entry] = []
        entries.reserveCapacity(min(count, 100_000))
        var position = offset
        for _ in 0..<count {
            guard try reader.u32(position) == 0x0201_4B50 else { throw Failure.damaged("the central directory ends early") }
            let flags = try reader.u16(position + 8)
            let method = try reader.u16(position + 10)
            var compressed = try reader.u32(position + 20)
            var uncompressed = try reader.u32(position + 24)
            let nameLength = try reader.u16(position + 28)
            let extraLength = try reader.u16(position + 30)
            let commentLength = try reader.u16(position + 32)
            var local = try reader.u32(position + 42)
            let name = String(decoding: try reader.bytes(position + 46, nameLength), as: UTF8.self)
            // zip64's sizes and offset, for those at their maximum, in order.
            if uncompressed == 0xFFFF_FFFF || compressed == 0xFFFF_FFFF || local == 0xFFFF_FFFF {
                var extra = position + 46 + nameLength
                let extraEnd = extra + extraLength
                while extra + 4 <= extraEnd {
                    let id = try reader.u16(extra)
                    let size = try reader.u16(extra + 2)
                    if id == 0x0001 {
                        var field = extra + 4
                        if uncompressed == 0xFFFF_FFFF { uncompressed = try reader.u64(field); field += 8 }
                        if compressed == 0xFFFF_FFFF { compressed = try reader.u64(field); field += 8 }
                        if local == 0xFFFF_FFFF { local = try reader.u64(field) }
                        break
                    }
                    extra += 4 + size
                }
            }
            entries.append(Entry(name: name, method: UInt16(method), compressedSize: compressed, uncompressedSize: uncompressed,
                                 localHeaderOffset: local, encrypted: flags & 1 != 0))
            position += 46 + nameLength + extraLength + commentLength
        }
        return entries
    }

    /// The end of central directory record, searched for back from the end
    /// past the longest comment it can have.
    private static func endOfDirectory(_ reader: Reader) throws -> Int {
        let size = reader.data.count
        guard size >= 22 else { throw Failure.notAZip }
        var position = size - 22
        let earliest = max(0, size - 22 - 0xFFFF)
        while position >= earliest {
            if try reader.u32(position) == 0x0605_4B50 { return position }
            position -= 1
        }
        throw Failure.notAZip
    }

    /// Little-endian reads, each checked against the data's end.
    private struct Reader {
        let data: Data

        private func check(_ offset: Int, _ length: Int) throws {
            guard offset >= 0, length >= 0, offset <= data.count - length else { throw Failure.damaged("an offset is past the end") }
        }

        func u16(_ offset: Int) throws -> Int {
            try check(offset, 2)
            return data.withUnsafeBytes { Int(UInt16(littleEndian: $0.loadUnaligned(fromByteOffset: offset, as: UInt16.self))) }
        }

        func u32(_ offset: Int) throws -> Int {
            try check(offset, 4)
            return data.withUnsafeBytes { Int(UInt32(littleEndian: $0.loadUnaligned(fromByteOffset: offset, as: UInt32.self))) }
        }

        func u64(_ offset: Int) throws -> Int {
            try check(offset, 8)
            let value = data.withUnsafeBytes { UInt64(littleEndian: $0.loadUnaligned(fromByteOffset: offset, as: UInt64.self)) }
            guard value <= UInt64(Int.max) else { throw Failure.damaged("an offset is past the end") }
            return Int(value)
        }

        func bytes(_ offset: Int, _ length: Int) throws -> Data {
            try check(offset, length)
            return data[offset..<(offset + length)]
        }
    }
}
