import Foundation
import ShishoAPI

/// Why a request to a Shisho server didn't succeed, in the terms the app
/// explains to the user.
enum APIError: Error, Equatable {
    /// The server's Shisho predates the Shisho app.
    case serverPredatesApp
    /// The server needs a newer app. serverVersion is known once signed in.
    case appTooOld(serverVersion: String?)
    /// The app needs a newer server.
    case serverTooOld(serverVersion: String?)
    /// A Demo Mode server, which the app doesn't connect to.
    case demoMode
    /// The username or password is wrong.
    case wrongPassword
    /// The account's password must be changed on the web first.
    case passwordResetRequired
    /// The account can't read Books.
    case noAccess
    /// The Device was revoked, or its user deactivated: sign in again.
    case signedOut
    /// The server couldn't be reached.
    case unreachable(String)
    /// Anything else the server said.
    case server(status: Int, message: String)
}

/// The one HTTP layer every request of one account goes through. Requests
/// skip the URL cache, so every response the app reads capabilities from
/// came from the server (ADR 0020). The system's trust store, including
/// root certificates the user installed and trusted, checks HTTPS.
final class APIClient: Sendable {
    let baseURL: URL
    private let credential: String?
    private let session: URLSession
    private let decoder = makeShishoDecoder()
    /// Called with each capability list a server response carries. It's
    /// fixed at creation, since requests on any thread read it.
    private let onCapabilities: (@Sendable ([String]) -> Void)?

    init(baseURL: URL, credential: String?, onCapabilities: (@Sendable ([String]) -> Void)? = nil) {
        self.baseURL = baseURL
        self.credential = credential
        self.onCapabilities = onCapabilities
        let configuration = URLSessionConfiguration.ephemeral
        configuration.urlCache = nil
        configuration.requestCachePolicy = .reloadIgnoringLocalCacheData
        configuration.httpCookieStorage = nil
        configuration.timeoutIntervalForRequest = 30
        configuration.waitsForConnectivity = false
        session = URLSession(configuration: configuration)
    }

    // MARK: - Requests

    func serverInfo() async throws -> Shishoapp.ServerInfo {
        try await json("mobile/server")
    }

    func signIn(username: String, password: String, deviceName: String) async throws -> Shishoapp.SignInResponse {
        let payload = Shishoapp.SignInPayload(
            username: username, password: password, installationId: Installation.id,
            deviceName: deviceName, appVersion: Installation.appVersion
        )
        do {
            return try await json("mobile/sign-in", method: "POST", body: try JSONEncoder().encode(payload))
        } catch APIError.server(let status, _) where status == 404 || status == 405 {
            throw APIError.serverPredatesApp
        } catch APIError.signedOut {
            throw APIError.wrongPassword
        }
    }

    /// Signs the Device out on the server, so its credential stops working.
    /// It's best effort: a sign-out offline still forgets the credential.
    func signOut() async {
        _ = try? await data("mobile/device", method: "DELETE")
    }

    func changes(cursor: Int, limit: Int = 500) async throws -> Shishoapp.FeedResponse {
        try await json("mobile/changes", query: [URLQueryItem(name: "cursor", value: String(cursor)),
                                              URLQueryItem(name: "limit", value: String(limit))])
    }

    /// A Book's cover at size pixels, keyed by its cover version so the
    /// server sends it as immutable.
    func cover(bookID: Int, version: String, size: Int, renderKey: String) async throws -> Data {
        try await data("mobile/books/\(bookID)/cover", query: [
            URLQueryItem(name: "v", value: version),
            URLQueryItem(name: "size", value: String(size)),
            URLQueryItem(name: "r", value: renderKey),
        ], readsCapabilities: false)
    }

    // MARK: - Plumbing

    private func json<T: Decodable>(_ path: String, method: String = "GET", query: [URLQueryItem] = [], body: Data? = nil) async throws -> T {
        let data = try await data(path, method: method, query: query, body: body)
        do {
            return try decoder.decode(T.self, from: data)
        } catch {
            // A server from before the app answers its paths with the web
            // app's page.
            throw APIError.serverPredatesApp
        }
    }

    private func data(_ path: String, method: String = "GET", query: [URLQueryItem] = [], body: Data? = nil,
                      readsCapabilities: Bool = true) async throws -> Data {
        var components = URLComponents(url: baseURL.appendingPathComponent(path), resolvingAgainstBaseURL: false)!
        if !query.isEmpty {
            components.queryItems = query
        }
        var request = URLRequest(url: components.url!)
        request.httpMethod = method
        request.setValue(String(Installation.appProtocol), forHTTPHeaderField: "X-Shisho-App-Protocol")
        request.setValue(Installation.appVersion, forHTTPHeaderField: "X-Shisho-App-Version")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let credential {
            request.setValue("Bearer \(credential)", forHTTPHeaderField: "Authorization")
        }
        if let body {
            request.httpBody = body
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch {
            throw APIError.unreachable(error.localizedDescription)
        }
        guard let http = response as? HTTPURLResponse else {
            throw APIError.unreachable("No response")
        }
        if readsCapabilities, let listed = http.value(forHTTPHeaderField: "X-Shisho-Capabilities") {
            onCapabilities?(listed.split(separator: ",").map(String.init))
        }
        if (200..<300).contains(http.statusCode) {
            return data
        }
        throw APIClient.error(status: http.statusCode, response: http, body: data)
    }

    private struct ErrorBody: Decodable {
        struct Inner: Decodable {
            let code: String?
            let message: String?
        }

        let error: Inner?
    }

    static func error(status: Int, response: HTTPURLResponse, body: Data) -> APIError {
        let parsed = try? JSONDecoder().decode(ErrorBody.self, from: body)
        let code = parsed?.error?.code ?? ""
        let message = parsed?.error?.message ?? HTTPURLResponse.localizedString(forStatusCode: status)
        let serverVersion = response.value(forHTTPHeaderField: "X-Shisho-Server-Version").flatMap { $0.isEmpty ? nil : $0 }
        switch (status, code) {
        case (403, "demo_mode"):
            return .demoMode
        case (426, "server_update_required"):
            return .serverTooOld(serverVersion: serverVersion)
        case (426, _):
            return .appTooOld(serverVersion: serverVersion)
        case (403, "password_reset_required"):
            return .passwordResetRequired
        case (403, _):
            return .noAccess
        case (401, _):
            return .signedOut
        default:
            return .server(status: status, message: message)
        }
    }
}
