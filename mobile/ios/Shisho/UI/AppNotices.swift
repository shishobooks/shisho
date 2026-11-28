import SwiftUI

/// What the user has to know whichever tab they're on: why the device's
/// copy can't be brought up to date (as the app or the server being too
/// old), changes the server refused, and accounts signed out or replaced.
/// Each tab's first screen shows it at its top.
struct AppNotices: View {
    @Environment(AppModel.self) private var model
    @State private var notes: [JournalNote]?

    /// Anything to say, so a screen with nothing to say keeps no gap.
    private var showsAny: Bool {
        let syncing = switch model.syncStatus {
        case .idle, .offline: false
        default: true
        }
        return syncing || model.endedSignIn != nil || model.replacedAccountNotice?.accountID == model.session?.account.id
            || !(notes ?? []).isEmpty
    }

    var body: some View {
        VStack(spacing: 10) {
            SyncBanner()
            if let ended = model.endedSignIn {
                EndedSignInBanner(account: ended, serverReplaced: model.serverReplacedAccountID == ended.id,
                                  signIn: { model.switchAccount(to: ended.id) }, dismiss: model.dismissEndedSignIn)
            }
            if let notice = model.replacedAccountNotice, notice.accountID == model.session?.account.id {
                ReplacedAccountBanner(removedChanges: notice.removedChanges) { model.dismissReplacedAccountNotice() }
            }
            if let notes, !notes.isEmpty {
                JournalNotesBanner(notes: notes) { model.dismissJournalNotes() }
            }
        }
        .padding(.top, showsAny ? 10 : 0)
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $notes) { db in
            try CatalogStore.journalNotes(db)
        }
    }
}

/// The server ended this device's sign-in to another account, and one
/// still signed in took over: says so, with a way to sign in again. A server
/// set up again is a new account once signed in to, as the sign-in screen
/// says too.
struct EndedSignInBanner: View {
    let account: Account
    let serverReplaced: Bool
    let signIn: () -> Void
    let dismiss: () -> Void

    private var text: String {
        let host = account.serverURL.host() ?? account.serverURL.absoluteString
        return serverReplaced
            ? String(localized: "The server at \(host) was set up again, so this device was signed out of \(account.username) there. Signing in again starts fresh on it, and removes this account's library and the changes this device hadn't sent.")
            : String(localized: "This device was signed out of \(account.username) on \(host). Its library is still on this device; sign in again to keep syncing it.")
    }

    var body: some View {
        NoticeCard(
            text: text,
            icon: .userX,
            tint: Theme.primary
        ) {
            HStack(spacing: 16) {
                Button("Sign In", action: signIn)
                    .font(.geist(.subheadline, weight: .semibold))
                    .frame(minHeight: 44)
                    .accessibilityIdentifier("signedOut.signIn")
                Button("Dismiss", action: dismiss)
                    .font(.geist(.subheadline, weight: .semibold))
                    .frame(minHeight: 44)
                    .accessibilityIdentifier("signedOut.dismiss")
            }
        }
        .accessibilityIdentifier("signedOut.notice")
    }
}
