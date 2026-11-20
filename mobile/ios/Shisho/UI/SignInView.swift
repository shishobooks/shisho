import SwiftUI

struct SignInView: View {
    @Environment(AppModel.self) private var model
    @State private var server = ""
    @State private var username = ""
    @State private var password = ""
    @State private var busy = false
    @State private var error: SignInError?
    @FocusState private var focus: Field?

    private enum Field { case server, username, password }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    VStack(spacing: 10) {
                        Image("Logo")
                            .resizable()
                            .scaledToFit()
                            .frame(width: 72, height: 72)
                            .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
                            .accessibilityHidden(true)
                        Text("SHISHO")
                            .font(.geist(.title2, weight: .bold))
                            .tracking(1.2)
                        Text("Sign in with the same username and password you use on the web.")
                            .font(.geist(.subheadline))
                            .foregroundStyle(Theme.mutedForeground)
                            .multilineTextAlignment(.center)
                    }
                    .frame(maxWidth: .infinity)
                    .listRowBackground(Color.clear)
                }

                if let account = model.signedOutAccount, error == nil {
                    Section {
                        Label("This device was signed out of \(account.username) on \(account.serverURL.host() ?? account.serverURL.absoluteString). Sign in again to keep syncing. Everything on this device is still here.", systemImage: "person.crop.circle.badge.exclamationmark")
                            .font(.geist(.subheadline))
                            .accessibilityIdentifier("signin.signedOut")
                    }
                }

                Section("Server") {
                    TextField("https://shisho.example.com", text: $server)
                        .textContentType(.URL)
                        .keyboardType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .submitLabel(.next)
                        .focused($focus, equals: .server)
                        .onSubmit { focus = .username }
                        .accessibilityLabel("Server address")
                        .accessibilityIdentifier("signin.server")
                }

                Section("Account") {
                    TextField("Username", text: $username)
                        .textContentType(.username)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .submitLabel(.next)
                        .focused($focus, equals: .username)
                        .onSubmit { focus = .password }
                        .accessibilityIdentifier("signin.username")
                    SecureField("Password", text: $password)
                        .textContentType(.password)
                        .submitLabel(.go)
                        .focused($focus, equals: .password)
                        .onSubmit(submit)
                        .accessibilityIdentifier("signin.password")
                }

                if let error {
                    Section {
                        VStack(alignment: .leading, spacing: 8) {
                            Label(error.message, systemImage: "exclamationmark.triangle")
                                .font(.geist(.subheadline))
                                .foregroundStyle(Theme.destructive)
                                .accessibilityIdentifier("signin.error")
                            if error.offersUpdate {
                                Link("Update the app", destination: Installation.updateURL)
                                    .font(.geist(.subheadline, weight: .semibold))
                            }
                        }
                    }
                }

                Section {
                    Button(action: submit) {
                        HStack {
                            Spacer()
                            if busy {
                                ProgressView()
                                    .accessibilityLabel("Signing in")
                            } else {
                                Text("Sign In")
                                    .font(.geist(.headline, weight: .semibold))
                            }
                            Spacer()
                        }
                    }
                    .disabled(busy || server.isEmpty || username.isEmpty || password.isEmpty)
                    .accessibilityIdentifier("signin.submit")
                }
            }
            .navigationTitle("Sign In")
            .navigationBarTitleDisplayMode(.inline)
            .frame(maxWidth: 560)
            .frame(maxWidth: .infinity)
            .background(Color(.systemGroupedBackground))
        }
        .onAppear {
            if let account = model.signedOutAccount {
                server = account.serverURL.absoluteString
                username = account.username
            }
        }
    }

    private func submit() {
        guard !busy, !server.isEmpty, !username.isEmpty, !password.isEmpty else { return }
        busy = true
        error = nil
        Task {
            do {
                try await model.signIn(server: server, username: username, password: password)
            } catch let signInError as SignInError {
                error = signInError
            } catch {
                self.error = SignInError(message: error.localizedDescription)
            }
            busy = false
        }
    }
}
