# The Shisho App

The Shisho app for iPhone and iPad (`ios/`), and one suite of Maestro flows (`harness/flows/`) that the Android app will run unchanged. Each platform is written natively with no shared code (ADR 0021); `android/` holds only the generated API types and their decode tests until the Android app joins it. The decisions are in ADRs 0020 to 0026; the server side is in `docs/agents/backend/change-feed.md`.

## Rules every change needs

- **Every screen reads the account's own SQLite database; the network only updates it.** A screen never shows a server response directly, so everything shown works offline. Each account has its own database under its folder, owned by native code. On iOS it's GRDB: screens observe it with `observing(...)` (`ios/Shisho/UI/Observed.swift`) rather than refetching after a sync.
- **A schema change is a new GRDB migration**, appended to `CatalogStore.migrator`. Never edit one that has shipped: devices that already ran it won't run it again. An unreleased one is edited in place; debug builds rebuild a database whose migrations changed.
- **Swift 6 language mode, so data races are compile errors.** Make code provably safe (value types, `let`s of `Sendable` types, actors, `@MainActor`) rather than reaching for an escape hatch; `mise lint:swiftconcurrency` rejects one without a reason.
- **Never write an API type by hand.** They're generated from the Go structs by `mise tygo` (`tools/apptypes`) into gitignored folders: Swift in `ios/ShishoAPI/Sources/ShishoAPI/Generated/`, in namespaces named for the Go packages (`Shishoapp.FeedBook`), and Kotlin in `android/api/src/main/kotlin/generated/`, one package per Go package. Change the Go struct and run `mise tygo`.
- **Decode leniently and keep data on refusals** (ADR 0020). Unknown kinds and fields are skipped, an unknown enum value is its own case, and a 426 or an unreachable server leaves everything on the device working.
- **One HTTP layer per account carries every request** (`ios/Shisho/Core/APIClient.swift`). It skips the URL cache, so capability headers are only read from live responses.
- **Every control has an accessibility identifier and a VoiceOver label.** Flows find controls by identifier or visible text, never by position, so the Android app can run them unchanged: give the same control the same identifier on both platforms.
- **Strings go in the String Catalog** (`ios/Shisho/Resources/Localizable.xcstrings`). Command-line builds don't update it; run `mise ios:strings` after adding or removing strings.
- **The look comes from the web app**: its colors, from `app/index.css`, are in the asset catalog, and its typeface, Geist, is bundled. Use `Theme` and `Font.geist`, never system colors for brand surfaces.

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
- App code → `mise ios:test` and `mise ios:build`.
- Anything a flow covers → `mise ios:harness`, which needs ffmpeg. `-- -keep` keeps the server log and Maestro's screenshots. A flow that changes the world around the app, such as taking the server offline, calls the harness's control endpoint (`tools/appharness/ios.go`).
- The too-old paths → start the server with `SHISHO_DEV_APP_PROTOCOLS` set to the range it should accept: `2-2` makes an app on protocol 1 too old, and `0-0` makes the server too old for it.
