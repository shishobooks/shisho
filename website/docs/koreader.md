# KOReader

[KOReader](https://koreader.rocks/) can browse your library over OPDS and keep its reading position in Shisho through its own Progress sync. Reading in KOReader then shows up in [your reading history](./reading-tracking.md): a book you start becomes **Currently Reading**, a paused one resumes, and reaching its last pages marks it **Read**.

## Add a KOReader Device

Add one Device for each copy of KOReader, even when it runs on a Kobo: Kobo's own reader and KOReader are separate Devices.

1. Open the user menu, select **Devices**, and click **Add Device**.
2. Name it, choose **KOReader**, and click **Add Device**.

The Device's page shows everything to enter below. A new KOReader Device follows your default [Delivery Rules](./users-and-permissions.md#delivery-rules-of-a-device) until you change its rules on its page. Deliver EPUBs as EPUB: KOReader renders a KePub the same as an EPUB, so converting gains nothing and costs a wait on each first download.

## Browse Over OPDS

In KOReader, open **OPDS catalog** from the top menu's search tab, add a catalog with the plus button, and enter the **OPDS URL**, **Username**, and **Password** from the Device's page. The [OPDS Catalog](./opds.md) page covers what the catalog offers.

## Turn On Progress Sync

KOReader shows its **Progress sync** menu only with a book open, so open any book, then open the tools menu and select **Progress sync**. You set this up once: the settings apply to every book in KOReader.

1. Choose **Custom sync server** and enter the **Custom sync server** URL from the Device's page.
2. Choose **Register / Login**, enter your Shisho username and the Device's password, and tap **Login**. **Register** does not work here: it shows a message pointing you to the Devices page, because the Device is your account.
3. Turn on **Automatically keep documents in sync**. KOReader's setting is off by default, and without it, positions move only when you choose **Push progress from this device now** or **Pull progress from other devices now**.
4. Keep **Document matching method** on **Binary**, KOReader's default. Shisho does not support **Filename** matching.

### Automatic Sync and Battery

With automatic sync on, KOReader pulls your position when a book opens and when the device wakes, and pushes it when a book closes and when the device sleeps. **Periodically sync every # pages**, below it in the menu, also pushes while you read. Each sync needs a network connection:

- On an e-reader where KOReader manages Wi-Fi, such as a Kobo, KOReader turns Wi-Fi on for each sync and off again when the device sleeps. Turning Wi-Fi on that often uses more battery than leaving sync off. KOReader enables automatic sync only when **Action when Wi-Fi is off**, in its network settings, is set to **turn on**.
- A push KOReader can't send, because Wi-Fi is off or out of range, waits in a queue and goes out the next time KOReader connects.

KOReader doesn't tell Shisho when you read, so Shisho dates each position when the push arrives. The longer a push waits, the less accurate your reading dates are. A book you start or finish offline is dated the day KOReader next pushes it, and its [last read](./reading-tracking.md#last-read) is the time of that push. Until then, the Shisho reader and your other devices still have your old place.

You can leave automatic sync off to save battery. KOReader then pushes only when you choose **Push progress from this device now** in the **Progress sync** menu, and it sends only the open book's position. Every book you read in the meantime keeps its place in KOReader and syncs the next time you open it and push. You give up accurate dates for the battery you save, so a Read-through can start or end on the day you pushed rather than the day you read. To correct those dates, use **Edit dates** in [Reading History](./reading-tracking.md#reading-history).

Regenerating the Device's credential stops the old one working for sync as well as OPDS. Log in again in KOReader with the new one.

## Which Books Sync

KOReader identifies a book by a hash of the file's bytes, so a book syncs when its file is one Shisho produced or holds:

- **Downloaded from Shisho**, over OPDS or from the web interface, individually or in a bulk download. A copy keeps syncing after you edit the book's metadata, without downloading it again.
- **Copied over by hand**, as long as it is an unmodified copy of the file in your library.

A book in a library you can't access, or outside the Device's [scope](./users-and-permissions.md#scope), syncs as if Shisho didn't have it.

A copy that differs from those by even one byte does not sync, such as one edited elsewhere. KOReader also remembers a book's hash, so a file replaced in place on the device keeps reporting the hash of the copy it replaced. Delete the book and its metadata from KOReader, then download it again.

A copy downloaded from an older version of Shisho that didn't record these hashes shows as no progress too. Download it again.

## Moving Between Readers

A KOReader Device never moves itself. KOReader ignores a position it pushed, so pulling on the same device leaves you where you are, and KOReader says the latest progress is coming from this device.

For comics and PDFs, the page carries between KOReader and other readers: the web reader opens on the page KOReader pushed, and KOReader offers to jump to the page you reached in the web reader or on another KOReader Device. For EPUB and other reflowable formats, a position saved by another reader or another KOReader Device shows as no progress in KOReader.

## Finishing a Book

Pushing a position 99% or more through a book marks it **Read**, as a streaming service counts an episode watched once the credits start. The last pages are usually back matter, and KOReader's continuous view never reports exactly 100%. In continuous view KOReader reports the top of the screen, so the last screen of a very short book can stay below 99%; use **Mark as read** in Shisho for those. Going back before that point within 15 minutes undoes the finish, as it does in the [Shisho reader](./reading-tracking.md#starting-and-finishing-automatically).

## Troubleshooting

### Login Fails

KOReader shows the message from Shisho. Check that the username is your Shisho username and the password is the credential of a KOReader Device, not your account password or the credential of another type of Device. If you regenerated the credential, log in with the new one. A deactivated user, or a role without Books Read, cannot log in.

### KOReader Says "No Progress Found"

Either Shisho has no position yet for that file, or it doesn't recognize the file; see [Which Books Sync](#which-books-sync). Shisho logs each hash it doesn't recognize, under **koreader document not recognized**, at the info level. To see which book it was, turn on **Send document metadata** under **Progress sync** in KOReader: a push then logs the title too.
