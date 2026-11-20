import Foundation

/// Follows a redirect only while it stays on the same server: the same host
/// and port, or the same host moving from HTTP to HTTPS. Every request
/// carries a password or a credential, and URLSession would otherwise resend
/// it, body and all, to wherever a redirect points.
final class RedirectPolicy: NSObject, URLSessionTaskDelegate, Sendable {
    static func allows(from: URL, to: URL) -> Bool {
        guard let fromHost = from.host()?.lowercased(), fromHost == to.host()?.lowercased(),
              let fromScheme = from.scheme?.lowercased(), let toScheme = to.scheme?.lowercased()
        else { return false }
        switch (fromScheme, toScheme) {
        case ("https", "https"), ("http", "http"):
            return port(from) == port(to)
        case ("http", "https"):
            return port(from) == port(to) || (port(from) == 80 && port(to) == 443)
        default:
            return false
        }
    }

    private static func port(_ url: URL) -> Int? {
        url.port ?? (url.scheme?.lowercased() == "https" ? 443 : 80)
    }

    /// The redirect's request to follow, or nil to stop at the redirect.
    /// URLSession drops Authorization on any redirect, so one that stays on
    /// the server carries it on: without it the server would answer the
    /// request as signed out.
    static func follow(_ next: URLRequest, from current: URLRequest) -> URLRequest? {
        guard let from = current.url, let to = next.url, allows(from: from, to: to) else {
            return nil
        }
        var next = next
        if let credential = current.value(forHTTPHeaderField: "Authorization") {
            next.setValue(credential, forHTTPHeaderField: "Authorization")
        }
        return next
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest) async -> URLRequest? {
        guard let current = task.currentRequest else { return nil }
        return Self.follow(request, from: current)
    }
}
