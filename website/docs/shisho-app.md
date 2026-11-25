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

Signing out in the app first sends changes it hasn't sent yet, such as listening done offline, when it can reach the server. Offline, it signs out at once and keeps those changes to send when you sign in again.

## Setting Up a Server Again

The app tells a server apart from one set up again from scratch at the same address, such as a fresh install with a new database. An app still signed in finds out at its next sync: it sends that server nothing and asks you to sign in again. Signing in to that server again makes a new account in the app. The account the app had for the same user on the earlier server is removed with its copy of the libraries, its [downloads](#downloads), and any changes made in the app that hadn't reached that server yet, since they belong to a server that's gone. The app says how many changes were removed.

Restoring a [backup](./deployment-and-maintenance.md#restore-a-backup) taken since the app first signed in keeps the server's identity, so the app carries on as the same account and sends what it hasn't sent. A backup from a Shisho release before the app could tell servers apart has no identity yet, so an app that has synced with the server since it was upgraded treats the restored server as one set up again.

## Browsing and Searching Offline

Everything in the libraries you can access is on the device: books with their descriptions, series, authors and narrators, genres, tags, and files, plus your [lists](./lists.md) and the lists shared with you. The **Library** tab browses it by series, people, genres, and tags, and the **Lists** tab shows your lists. **Filter** narrows the library by reading status, a range of ratings, file type, genres, tags, and language, as on the web, or to only the books [downloaded](#downloads) to the phone, and **Sort** orders it by several fields, each ascending or descending. Sort starts from the sort you [saved for the library on the web](./gallery-sort.md); a sort you pick in the app stays on that device, and **reset to default** goes back to the saved one. Each book shows your [reading status](./reading-tracking.md#reading-status) and rating, which you change on the web, and tapping a file opens its page, as on the web, with its edition details, chapters, and where it sits on disk. Tapping an audiobook's chapter there plays from that chapter. At the bottom of a book, the **Series**, **Author**, and **Lists** tabs show the books around it, as on the web. A list shows who it's shared with to its owner and anyone who can manage it, as on the web.

Search, from the magnifying glass at the top of the **Library** and **Lists** tabs, finds books by title, author, narrator, series, or an identifier such as an ISBN, and finds series, people, genres, and tags, including by their [aliases](./metadata.md#aliases-and-resource-merges). It searches the copy on the device, so it works in airplane mode, and it matches words and ranks results the way the web's search does. Unlike the web, it doesn't search the names of files and folders on the server.

Edits on the web reach the app the next time it syncs. When an admin changes which libraries a user can access, or a list is shared or unshared, the app follows on its next sync: a library you lose leaves the device, and one you gain downloads in the background. A user whose role can no longer read books keeps nothing from the server on the device.

Covers download as you browse. To have every cover on the device before going offline, including the covers of files that have their own, such as an audiobook's, turn on **Download all covers on Wi-Fi** in **Download Settings**, the gear at the top of the **Downloads** tab. It applies to every account on the phone.

### When the Server Can't Be Reached

The app knows when the phone has no network, and when it has one but your server doesn't answer, as with a home server when you're away from home. A bar above the tabs says **Offline** or **Can't reach your server**, and everything on the phone keeps working: browsing, searching, and your downloads. Books with nothing downloaded look dimmed in the library, and listening to or reading something that isn't downloaded shows that it needs a connection, with its button dimmed, rather than waiting for a server that won't answer. When the server can be reached again, the bar goes, and a book you tried to open opens.

## Accounts

The app can be signed in to several servers, or several accounts on one server, at once. **Account > Add Server** signs in to another server, or as someone else on the same one, and tapping an account in that list switches to it. Each account keeps its own copy of its libraries, so switching shows it at once, even offline. Signing out of one account switches to another that's still signed in.

## Listening

Books with an M4B audiobook have a **Listen** button that streams it from your server, or plays it from the phone once it's [downloaded](#downloads). Before you start, it shows the audiobook's length and size. A book with more than one audiobook, such as an abridged and an unabridged edition, has a listen button on each in its list of files, and **Listen** plays the one you listened to last. Playback goes on with the phone locked, and the lock screen and Control Center show the chapter, its length, and your speed, with controls to play, pause, and skip. Headphone and car controls skip too; chapters change in the app and in [CarPlay](#carplay). Calls and other apps' audio pause playback and resume it afterwards, and unplugging or disconnecting headphones pauses it.

The chapters are the ones Shisho has for the file, so [editing them](./metadata.md#chapters) changes them in the app too. Tap the chapter's name under the title, or **Chapters** under the player, to list them and go to one. Tap the time left under the position bar, which is at your current speed, to switch the bar between the chapter and the whole book.

Speed moves in steps of 0.1, from 0.5x to 4x, and is kept for each audiobook file on that phone, since it usually suits the narrator; a new one starts at 1x. The **…** menu at the top of the player sets how far skip back and skip forward go, separately, from 5 to 90 seconds, 30 by default; the lock screen, headphones, and car use the same lengths. **Rewind After a Pause**, on by default and turned off there too, goes back a few seconds when you resume after a minute or more, further the longer the pause. The AirPlay button beside the menu plays to a speaker or other output.

**Sleep** stops playback after a set time, counting only time spent playing, or at the end of the chapter. The last seconds fade out. Shake the phone while the timer runs to start its time over.

The app saves your place every few seconds while playing and when you pause, and sends it, with your listening time, the next time it reaches your server. Listening done offline, such as on a plane, counts at the time you listened, not when it synced: [Reading History](./reading-tracking.md#reading-history) and [Reading Stats](./reading-stats.md) show it on the right day, under the phone's Device. Listening starts and finishes a book's Read-through by the same rules as the [web player](./reading-tracking.md#starting-and-finishing-automatically).

Opening an audiobook takes you to your latest place from any device, unless you've also listened on this phone since it last synced. Then it stays where the phone was and offers the other place, without jumping. The offer names the [Device](./users-and-permissions.md#devices) the place came from, or says it's from the web. A place that arrives while you listen is only offered too.

If the server turns down a change the app made offline, for example listening to a book that has since been deleted, the app drops it and says so in the library.

### Bookmarks

**Bookmark** under the player adds a [bookmark](./reading-and-playback.md#bookmarks) at the current moment; tap **Add Note** in the message that follows to write a note. The **Bookmarks** tab beside **Chapters**, also in the **…** menu, lists them in time order: tap one to go to its moment, or use its buttons to edit its note or delete it. Bookmarks work offline and sync with your account, so ones you add on the web show in the app after it syncs, and ones you add in the app show on the web. If a note was edited in two places, the later edit wins, and a deleted bookmark stays deleted.

### CarPlay

In a car with CarPlay, Shisho lists **Continue Listening**, the audiobooks you're partway through with the one you listened to last first, then **Downloads**, the audiobooks on the phone, and each library that has audiobooks. Continue Listening leaves out books whose [status](./reading-tracking.md#reading-status) is Read, Paused, or Did Not Finish. When a car shows only a few rows, the libraries move under one **Libraries** row. A library lists its audiobooks in the same order as the library in the app. Pick one to play it and open Now Playing, which has previous and next chapter buttons beside skip back and skip forward at your skip lengths.

CarPlay doesn't need the app open on the phone, and it works offline for downloaded audiobooks; others need a connection to your server, as on the phone. Listening in the car saves your place and counts in your reading history and stats, as on the phone.

## Reading

Books with an EPUB, KePub, MOBI, AZW3, CBZ, or PDF file have a **Read** button, which shows an ebook's [word count](./supported-formats.md#word-count) or a comic's or PDF's page count, and the file's size, before you start, and once you've started, the chapter you're in and the pages left in the book. Each of those files has a read button in the book's list of files; **Read** opens the one you read last. The reader needs the whole book before it can show a page, so opening a book that isn't on the phone [downloads](#downloads) it. It stays on the phone, and opens without a connection, until you remove it.

A tap on the left or right third of the page turns it, and a tap in the middle shows or hides the controls. A tap on a link follows it, and in the middle of the page so does a tap near one; a tap near a footnote reference opens it anywhere on the page. A tap on a picture in the middle of the page opens it on its own: pinch or double-tap to zoom, and drag down to close it. A footnote opens in a card beside its reference, with **Go to Note** to read it in place. Press and hold to select text and use **Copy**, **Look Up**, and the rest of the system's menu; while text is selected, dragging moves the selection instead of turning the page.

In **Scrolled** layout, swipe left or right to go to the next or previous chapter, or pull past the end of a chapter and let go to go on to the next, or past its start for the one before.

The controls have:

- **Contents**, the book's chapters. For EPUB and KePub books these are the chapters Shisho has for the file, so [editing them](./metadata.md#chapters) changes them in the app too; a file Shisho has no chapters for uses the table of contents inside it. MOBI and AZW3 books use the table of contents inside the file. A book with no chapters at all has no **Contents** button.
- A bar across the bottom to move through the book, with a mark where each chapter starts, and at its ends, [back and forward](./reading-and-playback.md#going-back-after-a-jump) to where you were before a jump. In the app, going to another device's place is a jump too.
- **Search**, which finds a word or phrase anywhere in the book, as [search in the Shisho reader](./reading-and-playback.md#searching-inside-a-book) does, and lists each result in its sentence under its chapter. A single character searches when you tap **Search** on the keyboard. Choose a result to go to it: every match on the page is shaded, and the one you chose more strongly. The bar over the page then counts the results and steps to the previous or next one, and its search button shows the list again. With a keyboard, Command-F opens search, Command-G and Shift-Command-G step through the results, and Escape closes it. A search counts as one [jump](./reading-and-playback.md#going-back-after-a-jump), however many results you step through.
- **Settings**: the Shisho reader's [display settings](./reading-and-playback.md#display-settings), with the same options and steps: font, font size, force font size, line spacing, margins, justify text, theme, layout, and pages. Each change applies right away and keeps your place. Settings are kept on each phone or tablet, so an iPad can read at a different size than a phone, and they apply to every book and account on it. The theme starts on **System**, which is Light or Dark to match the app's appearance. Fixed-layout books always turn page by page, whatever the layout setting, as in the Shisho reader.

To read in your own font, open **Font** in the settings and tap **Import Font** to choose a TTF, OTF, or WOFF2 file from Files. The font stays on the phone or tablet, never on your server, and every account on it can use it; tap the remove button beside its name to delete it. The included fonts are the Shisho reader's own.

The screen stays on while a book is open, past the auto-lock time.

Under the page, with or without the controls, are the chapter and how far through the book you are, and in Paginated layout, the pages left in the chapter. Tap the pages left to count the pages left in the book instead. Where the contents don't say where the chapter ends, the count is the book's. Pages past the chapter showing are estimated from the pages read so far, so the count settles as you read.

The app saves your place on every page turn, and a book opens at the same place in the app and the [Shisho reader](./reading-and-playback.md#epub-mobi-and-azw3-reader). Reading time is counted as the [Shisho reader counts it](./reading-tracking.md#reading-time-from-devices), and reading starts and finishes a book's Read-through by the same [rules](./reading-tracking.md#starting-and-finishing-automatically). Places and reading time reach your server when the app syncs, such as when you close the book or open the app; reading done offline counts at the time you read. Opening a book follows the same rule as listening: your latest place from any device, unless you've also read on this phone since it last synced, when the other place is offered instead.

### Comics and PDFs

CBZ and PDF files open in a page reader of their own, which turns pages the same way: tap the left or right third, or swipe, and tap the middle for the controls. Pinch, or double-tap anywhere on the page, to zoom into a panel. A tap waits a moment to see whether a second follows, but once a page has turned, taps on the sides turn at once, so tapping through pages stays quick. A zoomed page pans to its edge before a swipe turns it, a tap anywhere shows the controls, and a double-tap zooms back out. Turned sideways, pages show two at a time, with the cover and any wide page (two printed pages scanned as one) on their own. In the **Scrolled** flow, pages fill the screen's width; zoomed, the column scrolls down as before and pans sideways.

The pages come in the same order as in the [Shisho reader](./reading-and-playback.md#cbz-and-pdf-readers), so a page number means the same page in both, and the app keeps your place as a page. A comic's or PDF's file page lists its pages in a grid, as on the web; tap one to open the reader there. The grid shows each page's picture, from your server until the file is [downloaded](#downloads) and from the phone after that; without a connection, a file that isn't downloaded shows page numbers for the pages it hadn't shown yet. Opening a page from the grid doesn't count as reading, so looking at one leaves your place where it was; turning on from it does, and starts a [Read-through](./reading-tracking.md#starting-and-finishing-automatically) there, so you can skip the pages before the story. Seeing the last page, including in a spread after turning the phone sideways, finishes the book.

The controls have:

- **Contents**, the chapters Shisho has for the file, such as a PDF's outline, each going to the page it starts on. [Editing them](./metadata.md#chapters) changes them in the app too. A file with no chapters has no **Contents** button.
- A bar across the bottom to move through the book, which shows the page you'd land on while you drag it, with [back and forward](./reading-and-playback.md#going-back-after-a-jump) at its ends.
- **Settings**: the **Paginated** or **Scrolled** flow, and **Follow Reading Direction**. As in the Shisho reader, a book opens scrolled when the pages after its cover are tall strips, as a webtoon's are, until you pick a flow for it. The flow you pick is kept for the book in your account, so the Shisho reader and your other devices use it too, and one picked in the Shisho reader reaches the app. A file whose [Reading Direction](./metadata.md#reading-direction) is right to left, such as manga, turns the other way, and so does the bar; turn off **Follow Reading Direction** to have every book turn left to right on that phone or tablet.

Under the page, with the controls hidden, are the chapter, the pages left in it, and how far through the book you are, as in the ebook reader, in either flow. Tap the pages left to count the pages left in the book instead; the choice holds in both readers. The bar with the controls shows the page you're on.

## Downloads

Each file in a book's list of files has a download button, so you can keep a book's audiobook without its ebook, or the other way round. The download button at the top of a series or a list downloads the main files of every book in it in one tap; supplements download one at a time. A file type this version of the app doesn't support shows as not supported, with its download button turned off.

Downloads go on in the background when you switch to another app or lock the phone. One that's interrupted, by losing the connection or by the app closing, carries on from where it stopped instead of starting over. Downloads go on for the account you're using; another account's pause until you switch back to it. A downloaded audiobook plays, and a downloaded book opens, without a connection.

The app downloads each file as it is in your library. Titles, authors, covers, and chapters always come from the app's copy of your library, so an edit on the web shows in the app at its next sync without downloading anything again.

The **Downloads** tab lists what's downloaded and what's on its way, with the space they take and what's left on the phone. Each download shows whether you've started it, how far you are, or that you've finished it, to help you choose what to remove. Tap a download's remove button, or **Remove All Downloads**, to free the space; tapping a download opens its book. Books you've opened in the reader are listed too, since opening one downloads it.

Two settings in **Download Settings**, the gear at the top of the **Downloads** tab, apply to every account on the phone, and both start off:

- **Download on Wi-Fi only**: downloads wait while the phone is on cellular or a network iOS treats as limited, such as a personal hotspot, and carry on from where they stopped once it's on Wi-Fi. A book you open in the reader still downloads on any network, since the reader needs the whole book to show a page, just as an audiobook streams on any network.
- **Remove when finished**: a download is removed once you listen to or read it to the end. A book in the reader is removed when you close it, unless you went back from the end. Supplements are never removed this way.

A downloaded file that's deleted from your library, or is in a library you lose access to, is removed from the phone at the next sync. One whose file in your library is replaced by a copy of a different size downloads again at the next sync, and the copy already on the phone plays or opens until the new one arrives.

### Sharing a File

**Share**, on a file's page, sends the file out of the app through the share sheet, such as to **Files** or another app. The copy carries the metadata Shisho has for the book now, as a download from the web does. Your server prepares that copy before sending it, which can take a while for a large audiobook the first time; it's kept in the server's [download cache](./deployment-and-maintenance.md#maintain-server-caches), so sharing it again is quick. A file Shisho can't add its metadata to, such as a supplement, is shared as it is in your library. Sharing needs a connection to your server.

## Diagnostics

**Account > Export Diagnostics** shows the app's log of syncs, playback problems, books that wouldn't open, and downloads that failed, with the app, iOS, and server versions, and shares it as a text file. Server addresses and sign-in details are always removed. Books and files are named by id unless you turn on **Include titles**. Read it before you send it anywhere.

## Paths the App Uses

The app calls these paths on your server. A reverse proxy has to pass them, as [HTTPS and Reverse Proxies](./deployment-and-maintenance.md#https-and-reverse-proxies) describes. If an authentication proxy sits in front of Shisho, let them through without the proxy's login, as you would for [OPDS](./opds.md) or [Kobo Sync](./kobo-sync.md): Shisho authenticates them itself.

| Path | Used for |
|------|----------|
| `/mobile/*` | Signing in, syncing the library, your lists, and your reading and listening, covers, comic and PDF page thumbnails, downloads, sharing files, and playing audiobooks |

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

### A Download Doesn't Start

**Symptom:** A download says it's waiting for Wi-Fi or for a connection, or that it kept stopping.

**Likely cause:** **Download on Wi-Fi only** is on and the phone is on cellular or a limited network, the phone has no connection, or it can't reach your server.

**Verify:** Check the **Downloads** tab, which says why each download is waiting, and open the server's address in the phone's browser.

**Fix:** Join Wi-Fi, turn off **Download on Wi-Fi only**, or reconnect. A download that failed shows why under the file; tap its download button to try again.

### Sharing a File Fails

**Symptom:** Sharing a large audiobook says the server couldn't be reached, or answers with an error, after a minute or two.

**Likely cause:** A reverse proxy in front of Shisho gave up waiting while the server prepared the file. Shisho finishes preparing it anyway.

**Verify:** Check the proxy's timeout for responses, which is often 60 or 100 seconds.

**Fix:** Share it again once the server has had time to finish, which is quick, or raise the proxy's timeout for `/mobile/*`.

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
