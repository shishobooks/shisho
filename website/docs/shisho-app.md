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

## Browsing and Searching Offline

Everything in the libraries you can access is on the device: books with their descriptions, series, authors and narrators, genres, tags, and files, plus your [lists](./lists.md) and the lists shared with you. The **Library** tab browses it by series, people, genres, and tags, and the **Lists** tab shows your lists. **Filter** narrows the library by reading status, a range of ratings, file type, genres, tags, and language, as on the web, and **Sort** orders it by several fields, each ascending or descending. Sort starts from the sort you [saved for the library on the web](./gallery-sort.md); a sort you pick in the app stays on that device, and **reset to default** goes back to the saved one. Each book shows your [reading status](./reading-tracking.md#reading-status) and rating, which you change on the web, and tapping a file opens its page, as on the web, with its edition details, chapters, and where it sits on disk. Tapping an audiobook's chapter there plays from that chapter. At the bottom of a book, the **Series**, **Author**, and **Lists** tabs show the books around it, as on the web. A list shows who it's shared with to its owner and anyone who can manage it, as on the web.

Search, from the magnifying glass at the top of the **Library** and **Lists** tabs, finds books by title, author, narrator, series, or an identifier such as an ISBN, and finds series, people, genres, and tags, including by their [aliases](./metadata.md#aliases-and-resource-merges). It searches the copy on the device, so it works in airplane mode, and it matches words and ranks results the way the web's search does. Unlike the web, it doesn't search the names of files and folders on the server.

Edits on the web reach the app the next time it syncs. When an admin changes which libraries a user can access, or a list is shared or unshared, the app follows on its next sync: a library you lose leaves the device, and one you gain downloads in the background. A user whose role can no longer read books keeps nothing from the server on the device.

Covers download as you browse. To have every cover on the device before going offline, turn on **Account > Download all covers on Wi-Fi**.

## Accounts

The app can be signed in to several servers, or several accounts on one server, at once. **Account > Add Server** signs in to another server, or as someone else on the same one, and tapping an account in that list switches to it. Each account keeps its own copy of its libraries, so switching shows it at once, even offline. Signing out of one account switches to another that's still signed in.

## Listening

Books with an M4B audiobook have a **Listen** button that plays it from your server. Before you start, it shows the audiobook's length and size. A book with more than one audiobook, such as an abridged and an unabridged edition, has a listen button on each in its list of files, and **Listen** plays the one you listened to last. Playback goes on with the phone locked, and the lock screen and Control Center show the chapter, its length, and your speed, with controls to play, pause, and skip. Headphone and car controls skip too; chapters change in the app. Calls and other apps' audio pause playback and resume it afterwards, and unplugging or disconnecting headphones pauses it.

The chapters are the ones Shisho has for the file, so [editing them](./metadata.md#chapters) changes them in the app too. Tap the chapter's name under the title, or **Chapters** under the player, to list them and go to one. Tap the time left under the position bar, which is at your current speed, to switch the bar between the chapter and the whole book.

Speed moves in steps of 0.1, from 0.5x to 4x, and is kept for each audiobook file on that phone, since it usually suits the narrator; a new one starts at 1x. The **…** menu at the top of the player sets how far skip back and skip forward go, separately, from 5 to 90 seconds, 30 by default; the lock screen, headphones, and car use the same lengths. **Rewind After a Pause**, on by default and turned off there too, goes back a few seconds when you resume after a minute or more, further the longer the pause. The AirPlay button beside the menu plays to a speaker or other output.

**Sleep** stops playback after a set time, counting only time spent playing, or at the end of the chapter. The last seconds fade out. Shake the phone while the timer runs to start its time over.

The app saves your place every few seconds while playing and when you pause, and sends it, with your listening time, the next time it reaches your server. Listening done offline, such as on a plane, counts at the time you listened, not when it synced: [Reading History](./reading-tracking.md#reading-history) and [Reading Stats](./reading-stats.md) show it on the right day, under the phone's Device. Listening starts and finishes a book's Read-through by the same rules as the [web player](./reading-tracking.md#starting-and-finishing-automatically).

Opening an audiobook takes you to your latest place from any device, unless you've also listened on this phone since it last synced. Then it stays where the phone was and offers the other place, without jumping. The offer names the [Device](./users-and-permissions.md#devices) the place came from, or says it's from the web. A place that arrives while you listen is only offered too.

If the server turns down a change the app made offline, for example listening to a book that has since been deleted, the app drops it and says so in the library.

### Bookmarks

**Bookmark** under the player adds a [bookmark](./reading-and-playback.md#bookmarks) at the current moment; tap **Add Note** in the message that follows to write a note. The **Bookmarks** tab beside **Chapters**, also in the **…** menu, lists them in time order: tap one to go to its moment, or use its buttons to edit its note or delete it. Bookmarks work offline and sync with your account, so ones you add on the web show in the app after it syncs, and ones you add in the app show on the web. If a note was edited in two places, the later edit wins, and a deleted bookmark stays deleted.

## Reading

Books with an EPUB, KePub, MOBI, or AZW3 file have a **Read** button, which shows the file's size before you start, and the chapter you're in and the pages left in the book once you've read on the phone. Each of those files has a read button in the book's list of files; **Read** opens the one you read last. A book you open is downloaded from your server the first time and kept on the phone for a while, so it opens again without a connection. iOS may clear it to free space, and the next open downloads it again.

A tap on the left or right third of the page turns it, and a tap in the middle shows or hides the controls. A tap on a link follows it, and in the middle of the page so does a tap near one; a tap near a footnote reference opens it anywhere on the page. A tap on a picture in the middle of the page opens it on its own: pinch or double-tap to zoom, and drag down to close it. A footnote opens in a card beside its reference, with **Go to Note** to read it in place. Press and hold to select text and use **Copy**, **Look Up**, and the rest of the system's menu; while text is selected, dragging moves the selection instead of turning the page.

In **Scrolled** layout, swipe left or right to go to the next or previous chapter, or pull past the end of a chapter and let go to go on to the next, or past its start for the one before.

The controls have:

- **Contents**, the book's chapters. For EPUB and KePub books these are the chapters Shisho has for the file, so [editing them](./metadata.md#chapters) changes them in the app too; a file Shisho has no chapters for uses the table of contents inside it. MOBI and AZW3 books use the table of contents inside the file.
- A bar across the bottom to move through the book, with a mark where each chapter starts, and at its ends, [back and forward](./reading-and-playback.md#going-back-after-a-jump) to where you were before a jump. In the app, going to another device's place is a jump too.
- **Search**, which finds a word or phrase anywhere in the book, as [search in the Shisho reader](./reading-and-playback.md#searching-inside-a-book) does, and lists each result in its sentence under its chapter. A single character searches when you tap **Search** on the keyboard. Choose a result to go to it: every match on the page is shaded, and the one you chose more strongly. The bar over the page then counts the results and steps to the previous or next one, and its search button shows the list again. With a keyboard, Command-F opens search, Command-G and Shift-Command-G step through the results, and Escape closes it. A search counts as one [jump](./reading-and-playback.md#going-back-after-a-jump), however many results you step through.
- **Settings**: the Shisho reader's [display settings](./reading-and-playback.md#display-settings), with the same options and steps: font, font size, force font size, line spacing, margins, justify text, theme, layout, and pages. Each change applies right away and keeps your place. Settings are kept on each phone or tablet, so an iPad can read at a different size than a phone, and they apply to every book and account on it. The theme starts on **System**, which is Light or Dark to match the app's appearance. Fixed-layout books always turn page by page, whatever the layout setting, as in the Shisho reader.

To read in your own font, open **Font** in the settings and tap **Import Font** to choose a TTF, OTF, or WOFF2 file from Files. The font stays on the phone or tablet, never on your server, and every account on it can use it; tap the remove button beside its name to delete it. The included fonts are the Shisho reader's own.

The screen stays on while a book is open, past the auto-lock time.

Under the page, with or without the controls, are the chapter and how far through the book you are, and in Paginated layout, the pages left in the chapter. Tap the pages left to count the pages left in the book instead. Where the contents don't say where the chapter ends, the count is the book's. Pages past the chapter showing are estimated from the pages read so far, so the count settles as you read.

The app saves your place on every page turn, and a book opens at the same place in the app and the [Shisho reader](./reading-and-playback.md#epub-mobi-and-azw3-reader). Reading time is counted as the [Shisho reader counts it](./reading-tracking.md#reading-time-from-devices), and reading starts and finishes a book's Read-through by the same [rules](./reading-tracking.md#starting-and-finishing-automatically). Places and reading time reach your server when the app syncs, such as when you close the book or open the app; reading done offline counts at the time you read. Opening a book follows the same rule as listening: your latest place from any device, unless you've also read on this phone since it last synced, when the other place is offered instead.

## Diagnostics

**Account > Export Diagnostics** shows the app's log of syncs, playback problems, and books that wouldn't open, with the app, iOS, and server versions, and shares it as a text file. Server addresses and sign-in details are always removed. Books and files are named by id unless you turn on **Include titles**. Read it before you send it anywhere.

## Paths the App Uses

The app calls these paths on your server. A reverse proxy has to pass them, as [HTTPS and Reverse Proxies](./deployment-and-maintenance.md#https-and-reverse-proxies) describes. If an authentication proxy sits in front of Shisho, let them through without the proxy's login, as you would for [OPDS](./opds.md) or [Kobo Sync](./kobo-sync.md): Shisho authenticates them itself.

| Path | Used for |
|------|----------|
| `/mobile/*` | Signing in, syncing the library, your lists, and your reading and listening, covers, downloading books to read, and playing audiobooks |

## Local Network and Certificates

The app connects over HTTPS. It also connects over plain HTTP to a server on your local network: a private IP address such as `192.168.1.20`, a Tailscale address (`100.64.0.0` to `100.127.255.255`), a `.local` or `.localhost` name, or a name without dots. Typing an address without `http://` or `https://` picks HTTP for those and HTTPS for anything else. The app refuses plain HTTP to any other address, since your password would cross the internet unencrypted. The first time it connects to the local network, iOS asks to allow it.

A server with a certificate from your own certificate authority works once the authority's profile is installed on the iPhone or iPad and turned on under **Settings > General > About > Certificate Trust Settings**. The app trusts what iOS trusts.

## Troubleshooting

### An Audiobook Won't Play

**Symptom:** Now Playing says the audiobook couldn't be played.

**Likely cause:** The phone can't reach the server, or the file is missing from the library on disk.

**Verify:** Play the same book in the web player.

**Fix:** Reconnect and press play again. A file the web player can't play either needs a [rescan](./libraries.md) or restoring on disk.

### A Book Won't Open

**Symptom:** The reader says the book couldn't be downloaded.

**Likely cause:** The phone can't reach the server to download a book it hasn't kept yet, or the file is missing from the library on disk.

**Verify:** Open the same book in the web reader.

**Fix:** Reconnect and tap **Try Again**. A file the web reader can't open either needs a [rescan](./libraries.md) or restoring on disk.

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
