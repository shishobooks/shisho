# Shisho App

:::note[In Development]
The Shisho app is under development and isn't available to install yet. This page describes what the server supports for it, and grows as the app does.
:::

The Shisho app is Shisho's own app for iPhone and iPad. It keeps a copy of every library you can access on the device, so browsing works without a connection, and it syncs with your server whenever it can reach it.

## Server Requirements

The app needs Shisho 0.10.0 or newer. A server that's too old, or an app that's too old for its server, gets a message naming the versions involved; anything already on the device keeps working while the two sides catch up.

The app can't sign in to a server in [Demo Mode](./demo.md), including the public demo. It says so when you try.

## Sign In

In the app, enter the server's address and the same username and password you use on the web. The address is the one you open in a browser, such as `https://shisho.example.com`. Your password manager can fill the username and password.

Each sign-in adds a **Shisho app** [Device](./users-and-permissions.md#devices) to your account, named after the iPhone or iPad. The Device list shows the app's version and when it last connected.

A user who must change their password signs in on the web first: the app refuses the sign-in until the password is changed. The role needs Books Read.

## Staying Signed In

The app stays signed in until one of these happens:

- You sign out in the app, under **Account > Sign Out**.
- The Device is deleted from **Devices**, by you or by an admin revoking it.
- The user is deactivated.

Changing your password doesn't sign the app out. A signed-out app asks you to sign in again and keeps what's on the device until you do.

## Listening

Books with an M4B audiobook have a **Listen** button that plays it from your server. Playback goes on with the phone locked, and the lock screen and Control Center show the chapter, its length, and your speed, with controls to play, pause, and skip. The chapters are the ones Shisho has for the file, so [editing them](./metadata.md#chapters) changes them in the app too. Speed moves in steps of 0.1 and is kept for each book on that phone; a new book starts at 1x.

The app saves your place every few seconds while playing and when you pause, and sends it, with your listening time, the next time it reaches your server. Listening done offline, such as on a plane, counts at the time you listened, not when it synced: [Reading History](./reading-tracking.md#reading-history) and [Reading Stats](./reading-stats.md) show it on the right day, under the phone's Device. Listening starts and finishes a book's Read-through by the same rules as the [web player](./reading-tracking.md#starting-and-finishing-automatically).

Opening a book takes you to your latest place from any device, unless you've also listened on this phone since it last synced. Then it stays where the phone was and offers the other place, without jumping. A place that arrives while you listen is only offered too.

If the server turns down a change the app made offline, for example listening to a book that has since been deleted, the app drops it and says so in the library.

## Diagnostics

**Account > Export Diagnostics** shows the app's log of syncs and playback problems, with the app, iOS, and server versions, and shares it as a text file. Server addresses and sign-in details are always removed. Books and files are named by id unless you turn on **Include titles**. Read it before you send it anywhere.

## Paths the App Uses

The app calls these paths on your server. A reverse proxy has to pass them, as [HTTPS and Reverse Proxies](./deployment-and-maintenance.md#https-and-reverse-proxies) describes. If an authentication proxy sits in front of Shisho, let them through without the proxy's login, as you would for [OPDS](./opds.md) or [Kobo Sync](./kobo-sync.md): Shisho authenticates them itself.

| Path | Used for |
|------|----------|
| `/mobile/*` | Signing in, syncing the library and your listening, covers, and playing audiobooks |

## Local Network and Certificates

The app connects over HTTPS. It also connects over plain HTTP to a server on your local network: a private IP address such as `192.168.1.20`, a Tailscale address (`100.64.0.0` to `100.127.255.255`), a `.local` or `.localhost` name, or a name without dots. Typing an address without `http://` or `https://` picks HTTP for those and HTTPS for anything else. The app refuses plain HTTP to any other address, since your password would cross the internet unencrypted. The first time it connects to the local network, iOS asks to allow it.

A server with a certificate from your own certificate authority works once the authority's profile is installed on the iPhone or iPad and turned on under **Settings > General > About > Certificate Trust Settings**. The app trusts what iOS trusts.

## Troubleshooting

### An Audiobook Won't Play

**Symptom:** Now Playing says the audiobook couldn't be played.

**Likely cause:** The phone can't reach the server, or the file is missing from the library on disk.

**Verify:** Play the same book in the web player.

**Fix:** Reconnect and press play again. A file the web player can't play either needs a [rescan](./libraries.md) or restoring on disk.

### The Server Is Too Old for the App

**Symptom:** Signing in says the server's Shisho is too old for the app.

**Likely cause:** The server runs a release from before Shisho 0.10.0.

**Verify:** Check the image tag or release the server runs.

**Fix:** [Update Shisho](./deployment-and-maintenance.md#update-shisho-safely).

### The App Can't Reach the Server

**Symptom:** Signing in says it couldn't reach the server.

**Likely cause:** The address is wrong, the phone isn't on the same network as a local server, or iOS was told not to allow the app on the local network.

**Verify:** Open the same address in the phone's browser.

**Fix:** Correct the address, or turn on Shisho under **Settings > Privacy & Security > Local Network**.

### Signing In Fails Behind an Authentication Proxy

**Symptom:** Signing in or syncing fails away from home, where an authentication proxy guards the server, while the web app works after the proxy's login. The app says something in front of the server turned it away, or that the server's Shisho is too old.

**Likely cause:** The proxy answers `/mobile/*` with its own login page or refusal.

**Verify:** While signed out of the proxy, open `https://your-server/mobile/server` in a browser. Shisho answers with a short JSON document; a login page means the proxy caught it.

**Fix:** Exempt `/mobile/*` from the proxy's login, as in [Paths the App Uses](#paths-the-app-uses).
