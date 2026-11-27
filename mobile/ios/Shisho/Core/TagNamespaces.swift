import Foundation

/// A tag name split by its Library's tag namespaces.
struct SplitTag: Equatable, Hashable, Sendable {
    /// The namespace the tag is in, as the Library lists it, or nil.
    let namespace: String?
    /// The name to show: without the prefix when namespaced.
    let name: String
}

/// Which tags a list or picker shows: every tag, the plain tags outside
/// every namespace, or one namespace's.
enum TagScope: Hashable, Sendable {
    case all
    case plain
    case namespace(String)
}

/// Tag namespaces, as the web shows them (metadata.md, Tag Namespaces): a
/// Library lists prefixes, and a tag named "Circle: Moon Factory" is the tag
/// "Moon Factory" in Circle. Only the display changes; tag names stay whole.
///
/// The rule is the server's (`pkg/tags/namespaces.go`) and the web's
/// (`app/utils/tagNamespaces.ts`): a tag is in a namespace when its name
/// starts with the namespace and a colon, compared case-insensitively for
/// ASCII letters only, and something other than spaces follows. Change all
/// three together.
enum TagNamespaces {
    static func split(_ tag: String, _ namespaces: [String]) -> SplitTag {
        let folded = foldASCII(tag)
        for namespace in namespaces {
            let prefix = foldASCII(namespace) + ":"
            // Folding only ASCII letters keeps every character where it was,
            // so the prefix's length is the same in the tag.
            guard folded.hasPrefix(prefix) else { continue }
            let name = String(tag.unicodeScalars.dropFirst(prefix.unicodeScalars.count)).drop { $0 == " " }
            if !name.isEmpty { return SplitTag(namespace: namespace, name: String(name)) }
        }
        return SplitTag(namespace: nil, name: tag)
    }

    /// A Book's tags as the web's Book page lists them: the plain tags under
    /// Tags, then a row for each namespace in the Library's order, leaving
    /// out any with none. Each tag keeps what it came from, with the name to
    /// show.
    static func group<Tag>(_ tags: [Tag], name: (Tag) -> String,
                           namespaces: [String]) -> [(namespace: String?, tags: [(tag: Tag, name: String)])] {
        var rows: [(namespace: String?, tags: [(tag: Tag, name: String)])] = [(nil, [])] + namespaces.map { ($0, []) }
        for tag in tags {
            let split = split(name(tag), namespaces)
            if let index = rows.firstIndex(where: { $0.namespace == split.namespace }) {
                rows[index].tags.append((tag, split.name))
            }
        }
        return rows.filter { !$0.tags.isEmpty }
    }

    /// Every Library's namespaces once, in order, for every Library at once:
    /// the first Library to list one names it.
    static func merged(_ lists: [[String]]) -> [String] {
        var seen: Set<String> = []
        var out: [String] = []
        for namespace in lists.joined() where seen.insert(foldASCII(namespace)).inserted {
            out.append(namespace)
        }
        return out
    }

    /// The tags in scope, whole.
    static func filter(_ tags: [String], scope: TagScope, namespaces: [String]) -> [String] {
        switch scope {
        case .all: tags
        case .plain: tags.filter { split($0, namespaces).namespace == nil }
        case .namespace(let namespace): tags.filter { split($0, namespaces).namespace == namespace }
        }
    }

    private static func foldASCII(_ string: String) -> String {
        String(String.UnicodeScalarView(string.unicodeScalars.map { scalar in
            ("A"..."Z").contains(scalar) ? Unicode.Scalar(scalar.value + 32)! : scalar
        }))
    }
}
