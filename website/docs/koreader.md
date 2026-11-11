# KOReader

[KOReader](https://koreader.rocks/) can browse your library over OPDS and sync your reading with Shisho. Reading in KOReader then shows up in [your reading history](./reading-tracking.md): a book you start becomes **Currently Reading**, a paused one resumes, and reaching its last pages marks it **Read**.

The **KOReader plugin**, which every Shisho server serves, is the recommended way to connect. It signs KOReader in to Shisho with a pairing code, adds your library to KOReader's OPDS catalogs, keeps itself up to date from Shisho, and syncs your reading:

- **Reading time** comes from KOReader's own statistics, page by page, so it matches what KOReader's statistics screens show.
- **Offline reading** keeps its real dates, however long the e-reader stays offline, and nothing is lost in the meantime.
- **Your place** moves between KOReader, the Shisho reader, and your other devices.

Without the plugin, KOReader's built-in [progress sync](#turn-on-progress-sync) still syncs your place, but Shisho can only estimate your reading time from it, and reading done offline is dated when the e-reader next connects.

## Add a KOReader Device

Add one Device for each copy of KOReader, even when it runs on a Kobo: Kobo's own reader and KOReader are separate Devices.

1. Open the user menu, select **Devices**, and click **Add Device**.
2. Name it, choose **KOReader**, and click **Add Device**.

The Device's page shows everything to enter below. A new KOReader Device follows your default [Delivery Rules](./users-and-permissions.md#delivery-rules-of-a-device) until you change its rules on its page. Deliver EPUBs as EPUB: KOReader renders a KePub the same as an EPUB, so converting gains nothing and costs a wait on each first download.

## Install the KOReader Plugin

KOReader can't install a plugin by itself, so the first install is by hand. Every later update comes from Shisho.

1. On the Device's page, click **Download Plugin**. The zip it downloads signs itself in to this server for the next 30 minutes.
2. Connect the e-reader to your computer with USB, or on Android open a file manager.
3. Unzip the download into KOReader's plugins folder, so a `shisho.koplugin` folder sits inside it. The folder is:

   | Platform   | Plugins folder                                   |
   | ---------- | ------------------------------------------------ |
   | Kobo       | `.adds/koreader/plugins`                         |
   | Kindle     | `koreader/plugins`                               |
   | PocketBook | `applications/koreader/plugins`                  |
   | Android    | `koreader/plugins` in internal storage           |
   | Linux      | `~/.config/koreader/plugins`                     |
   | macOS      | `~/Library/Application Support/koreader/plugins` |

4. Eject the e-reader and restart KOReader.

Each Shisho release also attaches the plugin as `shisho.koplugin.zip` on its [GitHub release](https://github.com/shishobooks/shisho/releases). That zip knows no server, so after installing it, sign in with **Add a server** in the [plugin's menu](#sign-in-with-the-plugin).

## Sign In with the Plugin

With Wi-Fi on, a plugin downloaded from the Device's page signs itself in on its first launch, as long as that's within 30 minutes of the download. If Wi-Fi is off, it signs in the next time Wi-Fi comes on.

Later than that, KOReader opens **Sign in to Shisho** with the server's URL filled in. On the Device's page, click **Pair** and enter the code it shows as the **Pairing code**. A pairing code lasts 30 minutes, and clicking **Pair** or **Download Plugin** again replaces it. To sign in without the Device's page, choose **Use password** and enter your Shisho username and the Device's password. A server URL typed without `http://` or `https://` is tried with `https://` first.

The plugin's menu is **Shisho** in the top menu's tools tab, in the file browser and in an open book. **Add a server** signs in to another Shisho server, each as a Device of its own on that server. Under each server, **Sign in again** signs in afresh and **Remove server** removes the plugin's sign-in for it, leaving KOReader's books and OPDS catalogs alone.

Signing in adds the server's catalog to KOReader's OPDS catalogs as **Shisho** with the server's address, signed in with the Device's password, so you can browse and download without typing a password. If the catalog list already has an entry for the server's OPDS URL, such as one you added by hand, the plugin updates that entry instead of adding another, and keeps its name.

The Device's page shows when the Device last connected through the plugin, and which version of the plugin it reported.

Regenerating the Device's credential stops the plugin's sign-in working too. The server shows **Needs login** in the plugin's menu: choose **Sign in again** under it, and the OPDS catalog entry is updated with the new password.

## Update the Plugin

Each time Wi-Fi comes on, the plugin asks every server it's signed in to which version of the plugin it serves, and offers to install a newer one, then restarts KOReader into it. If you cancel, it doesn't offer that build again on its own; **Check for plugin update** offers it again. Before the plugin signs in to a server, it checks for nothing.

- **Several servers:** it offers an update only from a server with a strictly newer version that every server it's signed in to accepts. When a newer plugin wouldn't work with one of your servers, **Check for plugin update** names the server to upgrade first.
- **Builds of one version:** a different build of the version installed, such as one from a development server, is offered only by the server the plugin was last installed from.
- **On purpose:** **Reinstall plugin from this server** under a server installs the plugin that server serves, newer or older, and makes it the server the plugin was installed from. It's there when you're signed in to more than one server, or when the server serves a plugin an update would never install, such as an older version.
- **Signed out:** a server that shows **Needs login** offers no update until you sign in to it again.

When a server no longer accepts the installed plugin, it shows **Update required** in the plugin's menu, and the plugin offers the update that server serves. If you'd rather not wait for Wi-Fi to come on again, choose **Check for plugin update**.

## Sync Reading with the Plugin

Once signed in, the plugin syncs on its own whenever Wi-Fi comes on, for any reason, so syncing never costs a Wi-Fi connection of its own. It never turns Wi-Fi on by itself unless you ask it to:

- **Sync now** in the plugin's menu turns Wi-Fi on if needed and sends everything at once. Use it before you switch to another device.
- **Settings > Sync on sleep**, off by default, turns Wi-Fi on briefly each time the e-reader goes to sleep, so Shisho stays current at the cost of some battery.

A sync sends your reading time and your place in each book you read since the last one. Everything waits on the e-reader while it's offline, for days, weeks, or months, and a restart or a flat battery loses nothing that KOReader had already saved. A sync cut off halfway picks up where it stopped without sending anything twice. **Sync now** says when it finishes or fails; a sync that runs on its own stays quiet.

If you're signed in to several servers, each gets the reading for every book it has, and one server being down doesn't hold up the others.

### Reading Time

The plugin sends the time KOReader's statistics recorded on each page, so your reading time in Shisho is measured, not estimated. [Reading Time From Devices](./reading-tracking.md#reading-time-from-devices) covers how it becomes reading sessions and which Read-through each belongs to. Reading a book you've never read, or one on To Read, starts its Read-through on the day you read it, even when that reading reaches Shisho days later.

Deleting a Read-through in Shisho stays deleted: the plugin's next sync doesn't bring back its reading, even though KOReader still has it. Deleting or resetting statistics in KOReader changes nothing in Shisho.

Reading time needs KOReader's statistics, which are on by default: **Enabled** under **Tools > Reading statistics**. With statistics turned off, the plugin's menu and **Sync now** say that reading time won't sync, and your place still does.

### The E-Reader's Clock

Shisho dates your reading by the e-reader's clock, corrected when it syncs: if the clock is more than five minutes off, Shisho shifts everything in that sync by the difference. Times before the Device was added to Shisho, or before 2000, as after a clock resets when the battery runs flat, are ignored and logged. A time later than when the sync arrived is taken as the time it arrived. Days follow [your time zone](./reading-tracking.md#your-time-zone) in Shisho, not the e-reader's.

### Your Place

The plugin sends your place in each book with the time you were there, so a place from offline reading that's older than one Shisho already has keeps its reading time but doesn't move you back.

When you open a book with Wi-Fi already on, and after each sync, the plugin asks Shisho for your latest place in the open book. A place saved anywhere else is offered by name: **Shisho web reader** for the Shisho reader, or the name of the other KOReader or Kobo Device. Opening a book never turns Wi-Fi on. Under **Settings** in the plugin's menu, **Newer position from another reader** and **Older position from another reader** set what happens: **Ask** asks before going there, **Go there** goes without asking, and **Ignore** stays put. By default a newer place asks and an older one is ignored. [Moving Between Readers](./reading-tracking.md#moving-between-readers) covers what carries for each format.

### Switching Between Two KOReader Devices

Sign in to Shisho with the plugin on both, each as its own KOReader Device. Before you put one down, choose **Sync now** on it. Then open the book on the other with Wi-Fi on, and it offers to jump to where you stopped.

Leave KOReader's own statistics cloud sync (**Cloud sync** under **Reading statistics**) unset on both. It copies one device's statistics into the other's, and the plugin would then send that reading again as the other device's, counting it twice.

### Turn Off Progress Sync

Once a Device syncs through the plugin, Shisho ignores KOReader's progress sync from it, since the plugin reports the same reading with its real time, and the Device's page says to turn progress sync off. In KOReader, open a book, open **Progress sync** from the tools menu, and turn off **Automatically keep documents in sync**.

### Needs Login

Regenerating the Device's credential, or deleting the Device, marks the server **Needs login** in the plugin's menu. Everything you read stays queued on the e-reader until you choose **Sign in again**, and then arrives with nothing lost.

## Browse Over OPDS

The KOReader plugin adds the catalog for you when it signs in. To add it by hand instead, open **OPDS catalog** from the top menu's search tab, add a catalog with the plus button, and enter the **OPDS URL**, **Username**, and **Password** from the Device's page. The [OPDS Catalog](./opds.md) page covers what the catalog offers.

## Turn On Progress Sync

Progress sync is KOReader's built-in way to sync your place, for when you don't use the plugin. Don't use both for one Device: once the Device syncs through the plugin, Shisho ignores progress sync from it.

KOReader shows its **Progress sync** menu only with a book open, so open any book, then open the tools menu and select **Progress sync**. You set this up once: the settings apply to every book in KOReader.

1. Choose **Custom sync server** and enter the **Custom sync server** URL from the Device's page.
2. Choose **Register / Login**, enter your Shisho username and the Device's password, and tap **Login**. **Register** does not work here: it shows a message pointing you to the Devices page, because the Device is your account.
3. Turn on **Automatically keep documents in sync**, which is off by default. [Automatic Sync and Battery](#automatic-sync-and-battery) covers its battery cost and what happens if you leave it off.
4. Keep **Document matching method** on **Binary**, KOReader's default. Shisho does not support **Filename** matching.

### Automatic Sync and Battery

With automatic sync on, KOReader pulls your position when a book opens and when the device wakes, and pushes it when a book closes and when the device sleeps. **Periodically sync every # pages**, below it in the menu, also pushes while you read. Each sync needs a network connection:

- On an e-reader where KOReader manages Wi-Fi, such as a Kobo, KOReader turns Wi-Fi on to pull when a book opens or the device wakes, and to push before the device sleeps, then turns it off again. Turning Wi-Fi on that often uses more battery than leaving sync off. KOReader enables automatic sync only when **Settings > Network > Action when Wi-Fi is off** is set to **Turn on**.
- A push KOReader can't send, because Wi-Fi is off or out of range, waits in a queue and goes out the next time KOReader connects with a book open. Several queued pushes sent together update your place in the order KOReader sends them, and add no [reading time](./reading-tracking.md#reading-time-from-devices).

KOReader doesn't tell Shisho when you read, so Shisho dates each position when the push arrives. The longer a push waits, the less accurate your reading dates are. A book you start or finish offline is dated the day KOReader next pushes it, and its [last read](./reading-tracking.md#last-read) is the time of that push. Until then, the Shisho reader and your other devices still have your old place.

You can leave automatic sync off to save battery. KOReader then pushes only when you choose **Push progress from this device now** in the **Progress sync** menu, and it sends only the open book's position. Every book you read in the meantime keeps its place in KOReader and syncs the next time you open it and push. You give up accurate dates for the battery you save, so a Read-through can start or end on the day you pushed rather than the day you read. To correct those dates, use **Edit dates** in [Reading History](./reading-tracking.md#reading-history).

Regenerating the Device's credential stops the old one working for sync as well as OPDS. Log in again in KOReader with the new one.

## Which Books Sync

This applies to the plugin and to progress sync alike. KOReader identifies a book by a hash of the file's bytes, so a book syncs when its file is one Shisho produced or holds:

- **Downloaded from Shisho**, over OPDS or from the web interface, individually or in a bulk download. A copy keeps syncing after you edit the book's metadata, without downloading it again.
- **Copied over by hand**, as long as it is an unmodified copy of the file in your library.

A book in a library you can't access, or outside the Device's [scope](./users-and-permissions.md#scope), syncs as if Shisho didn't have it.

A copy that differs from those by even one byte does not sync, such as one edited elsewhere. KOReader also remembers a book's hash, so a file replaced in place on the device keeps reporting the hash of the copy it replaced. Delete the book and its metadata from KOReader, then download it again.

A copy downloaded from an older version of Shisho that didn't record these hashes shows as no progress too. Download it again.

## Moving Between Readers with Progress Sync

With the plugin, see [Your Place](#your-place) instead. A KOReader Device never moves itself. KOReader ignores a position it pushed, so pulling on the same device leaves you where you are, and KOReader says the latest progress is coming from this device.

KOReader offers to jump to a position saved anywhere else, and names where it came from: **Shisho web reader** for the Shisho reader, or the name of the other KOReader or Kobo Device. Under **Sync behavior** in the **Progress sync** menu, **Sync to a newer state** and **Sync to an older state** set whether it asks first: on **Prompt** KOReader asks before jumping, and on **Auto** it jumps. [Moving Between Readers](./reading-tracking.md#moving-between-readers) covers what carries for each format.

## Finishing a Book

Pushing a position 99% or more through a book marks it **Read**, as a streaming service counts an episode watched once the credits start. The last pages are usually back matter, and KOReader's continuous view never reports exactly 100%. In continuous view KOReader reports the top of the screen, so the last screen of a very short book can stay below 99%; use **Mark as read** in Shisho for those. Going back before that point in KOReader within 15 minutes undoes the finish, as it does in the [Shisho reader](./reading-tracking.md#starting-and-finishing-automatically). A push from KOReader can't undo a finish made in another reader, since it may be one KOReader queued from before.

## Troubleshooting

### Login Fails

KOReader shows the message from Shisho. Check that the username is your Shisho username and the password is the credential of a KOReader Device, not your account password or the credential of another type of Device. If you regenerated the credential, log in with the new one. A deactivated user, or a role without Books Read, cannot log in.

### The Plugin Says the Pairing Code Is Wrong or Has Expired

The code was mistyped, has expired, or was replaced. A code lasts 30 minutes, and clicking **Pair** or **Download Plugin** on the Device's page replaces the code before it. Check that the code you entered is the one the Device's page shows now, without the countdown having run out. If it isn't, click **Pair** and enter the new code. Only a KOReader Device's code signs the plugin in.

### Reading Time Doesn't Sync

The plugin reads your reading time from KOReader's statistics, so check that **Enabled** under **Tools > Reading statistics** is on. Your place syncs either way. A server marked **Needs login** in the plugin's menu gets nothing until you sign in to it again; what you read in the meantime stays queued.

### KOReader Says "No Progress Found"

Either Shisho has no position yet for that file, or it doesn't recognize the file; see [Which Books Sync](#which-books-sync). Shisho logs each hash it doesn't recognize, under **koreader document not recognized**, at the info level. To see which book it was, turn on **Send document metadata** under **Progress sync** in KOReader: a push then logs the title too.
