import SwiftUI

@main
struct ShishoApp: App {
    @State private var model = AppModel()
    @Environment(\.scenePhase) private var scenePhase

    init() {
        Theme.applyUIKitFonts()
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(model)
                .tint(Color("AccentColor"))
                // Geist for any text that doesn't name its own font, so
                // nothing falls back to the system font by omission.
                .font(.geist(.body))
        }
        .onChange(of: scenePhase) { _, phase in
            switch phase {
            case .active:
                model.didBecomeActive()
                Task { await model.syncNow() }
            case .inactive:
                model.didResignActive()
            case .background:
                model.didEnterBackground()
                model.scheduleBackgroundRefresh()
            default:
                break
            }
        }
        .backgroundTask(.appRefresh(AppModel.refreshTaskID)) {
            await model.backgroundRefresh()
        }
        // Downloads finished or stopped while the app wasn't running.
        .backgroundTask(.urlSession) { identifier in
            await model.backgroundDownloads(identifier: identifier)
        }
    }
}
