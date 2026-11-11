# Server Stats

**Administration > Server Stats** shows what your server holds, how much disk it uses and has left, and whether people are using it. It requires Config Read, which only the Admin role has by default. Every figure is counted when the page opens, so refresh it to see changes.

## Overview

The totals across every library: books, files, library size, users, and [Devices](./users-and-permissions.md#devices). The file count includes [supplement files](./supplement-files.md). The user count includes deactivated users.

## Reading

Reading totals for the current year and for all time, across every user:

- **Reading** and **Listening** are the time recorded in reading sessions. Listening is time spent in an audiobook; everything else counts as reading. A session counts in the year it started. Time from KOReader and Kobo is estimated, as [Reading Time From Devices](./reading-tracking.md#reading-time-from-devices) explains.
- **Read-throughs finished** counts every finished read, so a reread counts again. A read is in the current year when its end date is. A finished read with no end date counts only toward all time.
- **Users who read in the last 30 days** counts users with any reading session in that time.

"The current year" is the year in the server's time zone, not each reader's: the `TZ` environment variable, or the host's time zone when it isn't set. The Docker image uses UTC unless you set `TZ`.

Server Stats never shows reading by user or by book. Admins can't see who read what here; each user's reading stays on their own pages.

Read-throughs of a book that was later deleted still count until their user deletes them from [Reading History](./reading-tracking.md#reading-history-page). See [Merging and Deleting Books](./reading-tracking.md#merging-and-deleting-books).

## Libraries

Each library's books, main files by format, supplement files, series, people, genres, tags, and publishers, with a total row when there are two or more libraries. **People** counts everyone credited as an author or narrator in that library.

Only the libraries in your [library access](./users-and-permissions.md#library-access) list get a row. The **All libraries** row, like the overview, covers every library on the server.

**Size** is the size of every file Shisho recorded at its last scan, supplements included. It comes from the database rather than reading the disk, so it can differ from your filesystem after files change outside Shisho, until the next scan.

## Library Growth

The number of books on the server over time, by when each was added to Shisho, in the server's time zone. Hover over the chart to see the total and how many were added in a month. The chart always spans at least a year, and groups by quarter past three years and by year past nine. A library's first scan adds every book it finds at once, so the line usually jumps at the start.

## Disk Space

Free and used space on each volume that holds a library folder, the directory containing the database, or `CACHE_DIR`. When several of these share a volume, it's listed once with each path under it. **Free** is the space Shisho can still write to. On a filesystem that reserves space for the system, such as ext4, the reserved space is in neither figure, so used and free add up to less than the total.

Library folders are listed only for libraries you can access, and only with Libraries Read, matching what the library pages show. A volume whose folders are all hidden from you still shows its space.

A path that can't be read is left out, such as a library folder on a drive that isn't mounted, or a network share that doesn't answer within a few seconds. Each ZFS dataset or btrfs subvolume is listed as its own volume, so datasets sharing a pool show the same free space more than once.

## Server Data

The database size includes its write-ahead log (the `-wal` file next to it), so it matches the space the database takes on disk. Each cache's size is the same figure **Administration > Cache** shows; see [Maintain Server Caches](./deployment-and-maintenance.md#maintain-server-caches) to change limits or clear one.
