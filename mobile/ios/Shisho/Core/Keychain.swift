import Foundation
import Security

/// Keeps each account's Device credential in the Keychain, readable after
/// the device is first unlocked so syncing in the background works, and
/// never backed up to another device.
enum Keychain {
    private static let service = "com.shishobooks.shisho.credential"

    static func credential(for accountID: String) -> String? {
        var query = baseQuery(accountID)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var item: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &item) == errSecSuccess,
              let data = item as? Data
        else { return nil }
        return String(data: data, encoding: .utf8)
    }

    struct WriteError: Error {
        let status: OSStatus
    }

    /// Saves the credential, or throws when the Keychain refuses it, such
    /// as for a build with no signature to own the item.
    static func setCredential(_ credential: String, for accountID: String) throws {
        removeCredential(for: accountID)
        var query = baseQuery(accountID)
        query[kSecValueData as String] = Data(credential.utf8)
        query[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        let status = SecItemAdd(query as CFDictionary, nil)
        guard status == errSecSuccess else { throw WriteError(status: status) }
    }

    static func removeCredential(for accountID: String) {
        SecItemDelete(baseQuery(accountID) as CFDictionary)
    }

    private static func baseQuery(_ accountID: String) -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: accountID,
        ]
    }
}
