# Reading Stats

**Reading Stats**, in the user menu, sums up your own reading for one year or for all time. It's private: other users, admins included, never see it.

It opens on the current year. The range picker offers every year you have reading in, plus **All time**. **Manage reading history** opens [Reading History](./reading-tracking.md#reading-history-page), where you can fix dates or delete a read that shouldn't count.

Everything here comes from your [reading history](./reading-tracking.md#reading-history): the Read-throughs you start and end, and the time each reader records as you read.

## Summary

| Figure | What it counts |
|--------|----------------|
| **Books finished** | Reads you finished in the range, counted on the day each ended. A book you finished twice counts twice, and the note under the figure says how many were rereads. Reads of a book since deleted from Shisho are never counted as rereads. |
| **Did Not Finish** | Reads you abandoned in the range, counted on the day each ended. |
| **Reading time** | Time spent reading ebooks, comics, and PDFs. |
| **Listening time** | Time spent listening to audiobooks, kept apart from reading time. |
| **Median days to finish** | The middle value of how many days your finished reads took, from start date to end date, counting both. A book started and finished on the same day took 1 day. Reads missing either date are left out. |
| **Average Rating** | The average of your current ratings of the books you finished in the range, each book once. |

A read with no end date has no year, so it counts only under **All time**.

## Time

Reading and listening time includes every session in the range, whether or not it belongs to a read, so time spent looking something up in a book you already finished counts too.

Time is split into days in your time zone, under **Preferences > Reading > Time zone**, or in UTC while it's unset. A session that runs past midnight is split between the two days. Changing your time zone moves time to the days it fell on in the new zone, but never moves a read's start or end date.

**Kobo and KOReader time is estimated.** Neither device times your reading the way the Shisho reader does; [Reading Time From Devices](./reading-tracking.md#reading-time-from-devices) explains how Shisho estimates it.

## Heatmap and Calendar

**Reading by day in** a year, such as **Reading by day in 2026**, is a heatmap with one square per day. A day you read or listened is shaded, and the more time you spent, the darker it is. Shades are relative to your own year, so the darkest squares are always your busiest days. Point at a square to see its date and time. Under **All time**, it shows the year the calendar is in.

Under it, a calendar shows one month, with arrows to move between the months of the year. It opens on the current month for this year or **All time**, and on the last month you read in for a past year. Under **All time**, the arrows go back to the first year you read in.

- Each day has a bar for every book you read that day. Reading a book on consecutive days makes one continuous bar across them.
- Each book keeps the same colour in every month, so you can follow it from day to day.
- A check marks the day you finished a read. A read you logged by hand, with no recorded time, shows only its check.
- A book deleted from Shisho still shows, by the title it had, but in a new colour, since nothing links its reads to the book any longer.

Select a day to list that day's sessions: the book, when the session started and ended in your time zone, how long it ran that day, how far through the book it took you, and where you read it. A session that ran past midnight is listed on both days, with the time spent on each. Books you finished that day are listed too.

You can delete a session that wasn't really reading, such as a book left open overnight. Deleting a session can't be undone: its time leaves your totals, the heatmap, the calendar, and the time shown under [Finished](#finished). To delete one, select the trash can beside it in a day's list and confirm.

## Charts and Breakdowns

- **Books finished by month** and **Reading and listening by month**: books finished and time spent in each month of the year. Under **All time**, they become **by year**.
- **Genres**, **Tags**, and **Authors**: how many books you finished under each, most first. A book with several genres counts once under each. These use the book's current genres, tags, and authors.
- **Kind**: how many books you finished as ebooks, comics, or audiobooks, from the files you read them in. A finished read with no recorded time counts under the kinds of its book's files, and as **Unknown** when those don't say either.
- **Ratings**: how many of the books you finished have each rating.
- **Where you read**: reading and listening time in the Shisho reader and on each of your devices, by name. Time from a device you've since removed, or recorded before Shisho kept track of which device it came from, shows as **Kobo** or **KOReader**.

Each breakdown shows its first few rows. **Show all** lists every one in a scrolling list with a search box, so even a library with hundreds of tags stays easy to browse.

Select a row in **Genres**, **Tags**, **Authors**, **Kind**, or **Ratings** to list only those books under [Finished](#finished), such as the fantasy you read this year. Rows from different breakdowns combine, so a genre and a rating list the books that match both. Select the row again, or remove its chip above the list, to clear it.

## Finished

Every book you finished in the range, latest first, 50 to a page, with the date you finished it, how many days it took, the time recorded for that read, and your rating. A book you read with no recorded time shows no time. A book you finished twice is listed for each read.

A book that was deleted from Shisho shows as **Removed from library**, and one in a library you no longer have access to shows as **No access to this library**. Neither links to the book. Both still count toward your totals and time, but a deleted book has no genres, tags, authors, or rating left to count in the breakdowns.
