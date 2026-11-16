# The Shisho app is written natively for each platform, and one black-box test suite keeps the two in step

The **Shisho app** is written twice: SwiftUI on iOS and Jetpack Compose on Android, each with its own local database, sync, download, and playback code. No code is shared between the two apps. One suite of end-to-end flows drives both apps through their accessibility identifiers. It runs the same scenario on each and checks the outcome on the device and on a Shisho server, so the two apps behave the same without sharing code. API types for both languages are generated from the Go structs, as the web app's TypeScript types are (ADR 0004).

The app exists for what the web app can't do well: audio that keeps playing and saving positions in the background, CarPlay and Android Auto, offline reading and listening, and a native feel. All of that is platform code under any framework, so a cross-platform layer would only have drawn the remaining screens.

## Considered options

- **React Native (Expo) with native modules.** A spike built the same slice (sign in, a Library read from local SQLite, an M4B player with lock screen controls, native position saving with server sync, CarPlay) in Expo and natively. In the Expo build, playback, background audio, CarPlay, Android Auto, and position saving all had to be native modules, and TypeScript drew three screens. The native modules and Expo's own SQLite opened the same database through two separate SQLite copies, which risks corrupting it; fixing that moves the database behind native code too. The Expo builds were about 15 times larger and started about 30% slower on Android.
- **A shared Kotlin Multiplatform core with native UI.** Rejected to avoid Swift and Kotlin interop and the build friction it brings. The test suite covers the risk a shared core would remove, which is the two apps' sync logic drifting apart.

## Consequences

- **Every behavior that matters is covered by a flow that runs on both apps.** That includes positions, sessions, offline changes, and Downloads. In the spike, the suite caught a deliberate difference in how often one build saved positions.
- **A feature isn't done until it works on both platforms and passes the same flows**, even though iOS is built first.
- **The type generator is part of the API contract**, alongside tygo. A request or response shape changes in Go, and the web app and both native apps regenerate from it.
- **Generated decoders are lenient**, as ADR 0020 requires. They ignore fields they don't know, every enum has an unknown case that keeps the raw value, and every field the server may omit is optional. Built-in File Types can grow (ADR 0019), so the UI handles the unknown case: a File Type the app doesn't know is shown as not supported in this version, and can't be downloaded.
