import SwiftUI

/// The signed-in account: which server, the versions on both sides, and
/// signing out.
struct AccountView: View {
    @Environment(AppModel.self) private var model
    @State private var confirmingSignOut = false
    @State private var lastSynced: Date?

    var body: some View {
        NavigationStack {
            Form {
                if let account = model.session?.account {
                    Section("Signed In") {
                        LabeledContent("Server", value: account.serverURL.absoluteString)
                        LabeledContent("Username", value: account.username)
                    }
                    Section("Versions") {
                        LabeledContent("Shisho server", value: account.serverVersion)
                        LabeledContent("This app", value: Installation.appVersion)
                    }
                    Section {
                        Button {
                            Task { await model.syncNow() }
                        } label: {
                            HStack {
                                Text("Sync Now")
                                Spacer()
                                if model.syncStatus == .syncing {
                                    ProgressView()
                                }
                            }
                        }
                        .accessibilityIdentifier("account.sync")
                        if let lastSynced {
                            LabeledContent("Last synced") {
                                Text(lastSynced, style: .relative)
                            }
                        }
                    }
                    Section {
                        Button("Sign Out", role: .destructive) { confirmingSignOut = true }
                            .accessibilityIdentifier("account.signOut")
                    } footer: {
                        Text("Signing out removes this device from your account. The Books on it stay until you sign in again.")
                    }
                }
            }
            .font(.geist(.body))
            .navigationTitle("Account")
            .confirmationDialog("Sign out of this server?", isPresented: $confirmingSignOut, titleVisibility: .visible) {
                Button("Sign Out", role: .destructive) {
                    Task { await model.signOut() }
                }
                .accessibilityIdentifier("account.signOut.confirm")
            }
            .task(id: model.catalogRevision) {
                lastSynced = try? model.session?.store.syncState().lastSyncedAt
            }
        }
    }
}
