# The Shisho App

The Shisho app for iPhone and iPad (`ios/`), and one suite of Maestro flows (`harness/flows/`) that the Android app will run unchanged. Each platform is written natively with no shared code (ADR 0021); `android/` holds only the generated API types and their decode tests until the Android app joins it. The decisions are in ADRs 0020 to 0026; the server side is in `docs/agents/backend/change-feed.md`.

## Rules every change needs

- **Every screen reads the account's own SQLite database; the network only updates it.** A screen never shows a server response directly, so everything shown works offline. Each account has its own database under its folder, owned by native code. On iOS it's GRDB: screens observe it with `observing(...)` (`ios/Shisho/UI/Observed.swift`) rather than refetching after a sync.
- **A change the user makes goes into the account's journal, never straight to the server** (ADR 0024): write it through `CatalogStore` (`ios/Shisho/Core/CatalogStore+Listening.swift`) and the sync sends it. Record it where the work happens, such as `AudioPlayer`, never in a view, so it's kept with the screen closed, the phone locked, or from CarPlay. A new kind of change is a new event type in `pkg/shishoapp/types.go`, sent only to servers that list its capability.
- **A schema change is a new GRDB migration**, appended to `CatalogStore.migrator`. Never edit one that has shipped: devices that already ran it won't run it again. An unreleased one is edited in place; debug builds rebuild a database whose migrations changed.
- **Swift 6 language mode, so data races are compile errors.** Make code provably safe (value types, `let`s of `Sendable` types, actors, `@MainActor`) rather than reaching for an escape hatch; `mise lint:swiftconcurrency` rejects one without a reason.
- **Never write an API type by hand.** They're generated from the Go structs by `mise tygo` (`tools/apptypes`) into gitignored folders: Swift in `ios/ShishoAPI/Sources/ShishoAPI/Generated/`, in namespaces named for the Go packages (`Shishoapp.FeedBook`), and Kotlin in `android/api/src/main/kotlin/generated/`, one package per Go package. Change the Go struct and run `mise tygo`.
- **Decode leniently and keep data on refusals** (ADR 0020). Unknown kinds and fields are skipped, an unknown enum value is its own case, and a 426 or an unreachable server leaves everything on the device working.
- **One HTTP layer per account carries every request** (`ios/Shisho/Core/APIClient.swift`). It skips the URL cache, so capability headers are only read from live responses.
- **Every control has an accessibility identifier and a VoiceOver label.** Flows find controls by identifier or visible text, never by position, so the Android app can run them unchanged: give the same control the same identifier on both platforms.
- **Strings go in the String Catalog** (`ios/Shisho/Resources/Localizable.xcstrings`). Command-line builds don't update it; run `mise ios:strings` after adding or removing strings.
- **The look comes from the web app**: its colors, from `app/index.css`, are in the asset catalog, and its typefaces, Geist and Geist Mono, are bundled. Use `Theme`, `Font.geist`, and `Font.geistMono`, never system colors or system fonts for brand surfaces. Icons are the web's Lucide set, through `Icon` and `Lucide` (`ios/Shisho/UI/Icons.swift`), never SF Symbols: `mise ios:icons` writes a new one into the asset catalog from the web app's `lucide-react`. Only what iOS draws itself, such as alerts and the AirPlay picker, keeps its own icons. CJK text falls back to iOS's own fonts, such as Hiragino for Japanese, on purpose: bundling Noto Sans CJK would add megabytes and needs a font cascade that SwiftUI's `.custom` can't express.

## Accessibility

Each screen has to work with VoiceOver, at accessibility text sizes, and on iPad. These are the traps agents have hit:

- **VoiceOver doesn't run in the Simulator.** Check what it would read in the accessibility tree (the device tools' `snapshot`). For a wider sweep, run Apple's `performAccessibilityAudit` from a throwaway UI-test target. Its contrast findings flag ordinary dark-on-light Geist text, so judge them against the web's colors instead of fixing each one. Say what you couldn't hear, so the user tries it on a phone.
- **A screen hidden with opacity is still read.** VoiceOver reaches into hidden screens kept alive that way, even with `accessibilityHidden`, once they hold a `NavigationStack`. Keep tabs alive in a `TabView` with its bar hidden, as `MainTabs` does.
- **Don't wrap a `Button` in a combined or ignored accessibility element.** It becomes a button inside a button, which VoiceOver and the harness can't press reliably. Label the button itself and add `.isSelected` when it's on.
- **Fixed sizes clip at large text.** Size text with `Font.geist` relative to a text style, size row heights with `@ScaledMetric`, and use `minHeight` rather than `height`. Check new screens at `accessibility-extra-large` (the device tools' `settings text-size`), which breaks layouts that look fine at default size. Things meant to stay a fixed size, like the tab bar, cap their size and offer the Large Content Viewer.
- **Every gesture needs another way in.** Drag to reorder, swipe to dismiss, and sliders need `accessibilityActions` or `accessibilityAdjustableAction`.
- **Tap targets are at least 44 points**, even when the drawn control is smaller.
- **Text uppercased for looks is read letter by letter.** Give it an `accessibilityLabel` in normal case. Decorative chevrons and icons beside a label are `accessibilityHidden`, and a picker announces its current choice as its `accessibilityValue`.

## Building

The mise tasks are the way in: `mise tasks | grep -E '^(ios|android|mobile)'` lists them. The Xcode project is generated from `ios/project.yml` by XcodeGen and isn't committed; edit `project.yml`, never the project.

- **Command-line builds use `ios/build/`**, never Xcode's DerivedData, so a terminal build and Xcode, or two worktrees, never build or resolve packages in the same place. Pass `-derivedDataPath build/DerivedData` to any `xcodebuild` you run yourself.
- **Simulator builds sign ad hoc with the team** (`CODE_SIGN_STYLE=Manual CODE_SIGN_IDENTITY=-`), as the tasks do. An unsigned build (`CODE_SIGNING_ALLOWED=NO`) can't write the Keychain, so it signs in and then forgets the sign-in on relaunch.
- **Signing**: `ios/Config/Shisho.xcconfig` sets the maintainer's team and the bundle id. Someone else copies `Signing.local.xcconfig.example` to the gitignored `Signing.local.xcconfig`.
- **QA against `mise start:qa`**, a throwaway server seeded with the harness's fixtures, never the dev database. It listens on the LAN too, so a phone can reach it.
- **Simulators are cloned per run** by `tools/appharness`, from a base it makes once. Never boot or change the base, and don't run tests on a simulator you share with another worktree; use `mise ios:test`.

## Checks

`mise mobile:check` runs them all; `mise check:quiet` leaves them out. While iterating:

- Generated types → `mise lint:apptypes`, then `mise ios:test` (decode tests on the host, app tests on a simulator) and `mise android:test`.
- App code → `mise ios:test`, which builds the app first.
- Anything a flow covers → `mise ios:harness`, which needs ffmpeg. `-- -keep` keeps the server log and Maestro's screenshots. A flow that changes the world around the app, such as taking the server offline, calls the harness's control endpoint (`tools/appharness/ios.go`).
- The too-old paths → start the server with `SHISHO_DEV_APP_PROTOCOLS` set to the range it should accept: a range above the app's protocol makes the app too old, and one below it makes the server too old for it.

## Writing flows

CI's macOS simulators stall now and then: a view hierarchy read or the app's main thread can freeze for around 10 seconds, so a flow that passes locally can still fail there. When a flow fails on CI, read the timestamps in the Maestro logs of the `harness-output` artifact before treating the failure as an app bug. Write flows to tolerate those stalls:

- **Never assert a state that lasts only a few seconds**, such as a short chapter or a narrow window of the playback position. Assert a state that stays put (the last chapter once playback stops), or an open-ended one ("at least 0:12", not "0:12 to 0:19"). To listen for a while from a place that varies, build the pattern from that place, as `scripts/note_online_pause.js` does.
- **Wait for the UI with `extendedWaitUntil` and a timeout of at least 30 seconds.** A longer timeout costs nothing when the flow passes, and a 10 to 15 second one fails on a single stall.
- **Check the server in a `retry`** around a wait, the script, and an `assertTrue` on what the script recorded, never after one fixed wait. `retry` only retries Maestro's own failures, so a script that throws fails the flow on its first try; the `assert_*.js` scripts record their verdict in `output.serverCheck` instead.
- **Tap a screen that keeps changing**, such as a playing player, through `subflows/tap_player.yaml`. A plain `tapOn` waits for the screen to settle, and while the position ticks it waits out its whole timeout, which can be longer than the state being checked lasts.
- **`centerElement` on something that can't reach the middle**, such as a title in the last row, keeps swiping before Maestro settles for it being visible. Give that `scrollUntilVisible` a generous timeout.
