// The Shisho API types. Every type lives in a namespace named for its Go
// package, such as Shishoapp.FeedBook or Models.FileType. The generated
// files are in Generated/; run `mise tygo` after changing a Go type.

import Foundation

/// Decodes a Shisho API response. The generated types are lenient on their
/// own, so this is a plain JSONDecoder.
public func makeShishoDecoder() -> JSONDecoder {
    JSONDecoder()
}
