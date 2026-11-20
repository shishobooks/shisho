import SwiftUI

struct SignInView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.scenePhase) private var scenePhase
    @State private var server = ""
    @State private var username = ""
    @State private var password = ""
    @State private var busy = false
    @State private var error: SignInError?
    @State private var failures = 0
    @FocusState private var focus: Field?

    private enum Field: CaseIterable { case server, username, password }

    private var canSubmit: Bool {
        !busy && !server.isEmpty && !username.isEmpty && !password.isEmpty
    }

    var body: some View {
        ScrollView {
            VStack(spacing: 28) {
                brand
                    .padding(.top, 48)

                if let account = model.signedOutAccount, error == nil {
                    NoticeCard(
                        text: String(localized: "This device was signed out of \(account.username) on \(account.serverURL.host() ?? account.serverURL.absoluteString). Sign in again to keep syncing. Everything on this device is still here."),
                        systemImage: "person.crop.circle.badge.exclamationmark",
                        tint: Theme.primary
                    )
                    .accessibilityIdentifier("signin.signedOut")
                }

                VStack(spacing: 16) {
                    LabeledInput(label: "Server", systemImage: "server.rack", focused: focus == .server) {
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
                    LabeledInput(label: "Username", systemImage: "person", focused: focus == .username) {
                        TextField("Username", text: $username)
                            .textContentType(.username)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .submitLabel(.next)
                            .focused($focus, equals: .username)
                            .onSubmit { focus = .password }
                            .accessibilityIdentifier("signin.username")
                    }
                    LabeledInput(label: "Password", systemImage: "lock", focused: focus == .password) {
                        SecureField("Password", text: $password)
                            .textContentType(.password)
                            .submitLabel(.go)
                            .focused($focus, equals: .password)
                            .onSubmit(submit)
                            .accessibilityIdentifier("signin.password")
                    }
                }

                if let error {
                    NoticeCard(text: error.message, systemImage: "exclamationmark.triangle.fill", tint: Theme.destructive) {
                        if error.offersUpdate {
                            UpdateAppLink()
                        }
                        if error.offersSettings {
                            SettingsLink()
                        }
                    }
                    .accessibilityIdentifier("signin.error")
                    .transition(.move(edge: .top).combined(with: .opacity))
                }

                VStack(spacing: 14) {
                    Button(action: submit) {
                        ZStack {
                            Text("Sign In").opacity(busy ? 0 : 1)
                            if busy {
                                ProgressView()
                                    .tint(Theme.primaryForeground)
                                    .accessibilityLabel("Signing in")
                            }
                        }
                    }
                    .buttonStyle(PrimaryButtonStyle())
                    .disabled(!canSubmit)
                    .accessibilityIdentifier("signin.submit")

                    Text("Use the same username and password as on the web. The server's address is the one you open in a browser.")
                        .font(.geist(.footnote))
                        .foregroundStyle(Theme.mutedForeground)
                        .multilineTextAlignment(.center)
                }
            }
            .padding(.horizontal, 24)
            .padding(.bottom, 32)
            .frame(maxWidth: 460)
            .frame(maxWidth: .infinity)
            .animation(.spring(response: 0.35, dampingFraction: 0.85), value: error?.message)
        }
        .scrollDismissesKeyboard(.interactively)
        .toolbar {
            ToolbarItemGroup(placement: .keyboard) {
                Button { moveFocus(by: -1) } label: { Image(systemName: "chevron.up") }
                    .disabled(focus == Field.allCases.first)
                    .accessibilityLabel("Previous field")
                    .accessibilityIdentifier("keyboard.previous")
                Button { moveFocus(by: 1) } label: { Image(systemName: "chevron.down") }
                    .disabled(focus == Field.allCases.last)
                    .accessibilityLabel("Next field")
                    .accessibilityIdentifier("keyboard.next")
                Spacer()
                if #available(iOS 26, *) {
                    Button(role: .confirm) { focus = nil }
                        .accessibilityIdentifier("keyboard.done")
                } else {
                    Button("Done") { focus = nil }
                        .fontWeight(.semibold)
                        .accessibilityIdentifier("keyboard.done")
                }
            }
        }
        // A sign-in that couldn't get through while iOS asked for the Local
        // Network permission tries again once the prompt is answered.
        .onChange(of: scenePhase) { old, phase in
            if old == .inactive, phase == .active, error?.retriesWhenActive == true {
                submit()
            }
        }
        .background(backdrop)
        .sensoryFeedback(.error, trigger: failures)
        .onAppear {
            if let account = model.signedOutAccount {
                server = account.serverURL.absoluteString
                username = account.username
            }
        }
    }

    private var brand: some View {
        VStack(spacing: 18) {
            Image("Logo")
                .resizable()
                .scaledToFit()
                .frame(width: 84, height: 84)
                .clipShape(RoundedRectangle(cornerRadius: 20, style: .continuous))
                .shadow(color: Theme.primary.opacity(0.35), radius: 22, y: 10)
                .accessibilityHidden(true)
            Wordmark(size: 30, showsMark: false)
            Text("Your library, in your pocket.")
                .font(.geist(.callout))
                .foregroundStyle(Theme.mutedForeground)
        }
    }

    /// A soft glow of the primary color behind the brand.
    private var backdrop: some View {
        ZStack(alignment: .top) {
            Theme.background
            RadialGradient(colors: [Theme.primary.opacity(0.16), .clear], center: .top, startRadius: 0, endRadius: 420)
                .frame(height: 520)
        }
        .ignoresSafeArea()
        .accessibilityHidden(true)
    }

    private func moveFocus(by step: Int) {
        guard let focus, let index = Field.allCases.firstIndex(of: focus) else { return }
        let next = index + step
        if Field.allCases.indices.contains(next) {
            self.focus = Field.allCases[next]
        }
    }

    private func submit() {
        guard canSubmit else { return }
        busy = true
        error = nil
        Task {
            do {
                try await model.signIn(server: server, username: username, password: password)
            } catch let signInError as SignInError {
                error = signInError
                failures += 1
            } catch {
                self.error = SignInError(message: error.localizedDescription)
                failures += 1
            }
            busy = false
        }
    }
}
