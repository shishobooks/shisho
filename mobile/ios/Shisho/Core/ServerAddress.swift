import Foundation

/// A Shisho server's address, as typed on the sign-in screen.
struct ServerAddress: Equatable {
    let url: URL

    enum Problem: Error, Equatable {
        case empty
        case invalid
        /// Plain HTTP to a host on the internet, which would send the
        /// password unencrypted.
        case insecureHTTP
    }

    /// Reads what someone typed: a scheme is optional, and a host on the
    /// local network without one gets http, anything else https. Plain
    /// HTTP is allowed only to the local network.
    init(_ text: String) throws {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { throw Problem.empty }
        var candidate = trimmed
        if !candidate.contains("://") {
            let host = candidate.split(separator: "/").first.map(String.init) ?? candidate
            let bareHost = host.split(separator: ":").first.map(String.init) ?? host
            candidate = (ServerAddress.isLocal(host: bareHost) ? "http://" : "https://") + candidate
        }
        guard var components = URLComponents(string: candidate),
              let scheme = components.scheme?.lowercased(),
              scheme == "http" || scheme == "https",
              let host = components.host, !host.isEmpty
        else { throw Problem.invalid }
        if scheme == "http" && !ServerAddress.isLocal(host: host) {
            throw Problem.insecureHTTP
        }
        components.scheme = scheme
        components.query = nil
        components.fragment = nil
        while components.path.hasSuffix("/") {
            components.path.removeLast()
        }
        guard let url = components.url else { throw Problem.invalid }
        self.url = url
    }

    /// Whether host is on the local network: localhost, a .local or
    /// single-label name, or a private, loopback, or link-local address.
    static func isLocal(host: String) -> Bool {
        let host = host.lowercased().trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        if host == "localhost" || host.hasSuffix(".local") || host.hasSuffix(".localhost") {
            return true
        }
        let octets = host.split(separator: ".").compactMap { Int($0) }
        if octets.count == 4 && host.split(separator: ".").count == 4 {
            switch (octets[0], octets[1]) {
            case (10, _), (127, _), (192, 168), (169, 254): return true
            case (172, 16...31): return true
            case (100, 64...127): return true // carrier-grade NAT, as Tailscale uses
            default: return false
            }
        }
        if host.contains(":") {
            return host == "::1" || host.hasPrefix("fe80:") || host.hasPrefix("fc") || host.hasPrefix("fd")
        }
        // A name with no dots, such as "nas", only resolves on the LAN.
        return !host.contains(".")
    }
}
