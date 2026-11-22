import SwiftUI

@main
struct ShishoApp: App {
    @State private var model = AppModel()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(model)
                .tint(Color("AccentColor"))
        }
        .onChange(of: scenePhase) { _, phase in
            switch phase {
            case .active:
                model.didBecomeActive()
                Task { await model.syncNow() }
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
    }
}
