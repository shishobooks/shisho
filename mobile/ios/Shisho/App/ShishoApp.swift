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
            if phase == .active {
                Task { await model.syncNow() }
            }
        }
    }
}
