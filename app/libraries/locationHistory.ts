// A copy of a LocationHistory, from its save().
export interface LocationHistorySnapshot {
  back: string[];
  forward: string[];
  group: object | undefined;
}

/**
 * The places a reader can go back and forward to in a Book, as KOReader's
 * "Go back to previous location" keeps them (its readerlink.lua). The Shisho
 * app's readers follow the same rules (#920, #926), so a change here changes
 * what those tickets promise too. The rules:
 *
 * - A jump (a link, a footnote's note, the chapter menu or a chapter mark,
 *   the progress bar, a search result) records where it left from, unless
 *   it didn't move the page at all; the caller checks that, since only the
 *   renderer knows. Any move counts, however short: the page just before
 *   this one, or a short scroll up.
 * - Turning pages records nothing, so going back after paging around where a
 *   jump landed still returns to where the jump left from.
 * - Jumps in one group, such as the results of one search, record only the
 *   first, so stepping through results is one entry, not one per result.
 * - Going back saves the place it left for going forward again, so forward
 *   returns to where the reader had paged to. A new jump forgets the way
 *   forward, as a browser does.
 * - Going back or forward skips the place the reader is already at, since
 *   going there would do nothing: the same CFI, which in a paginated book
 *   means the same page.
 *
 * Places are CFIs of what the screen shows, as the renderer reports it. The
 * history lasts while the Book is open in the reader.
 */
export class LocationHistory {
  #back: string[] = [];
  #forward: string[] = [];
  // The group of the last jump recorded, until another jump or a move back
  // or forward.
  #group: object | undefined;

  get canGoBack() {
    return this.#back.length > 0;
  }

  get canGoForward() {
    return this.#forward.length > 0;
  }

  /** A copy of the history, for `restore` if a move doesn't happen. */
  save(): LocationHistorySnapshot {
    return {
      back: [...this.#back],
      forward: [...this.#forward],
      group: this.#group,
    };
  }

  restore(saved: LocationHistorySnapshot) {
    this.#back = [...saved.back];
    this.#forward = [...saved.forward];
    this.#group = saved.group;
  }

  /** Records a jump that left the screen at `origin`. */
  recordJump(origin: string, group?: object) {
    if (group !== undefined && group === this.#group) return;
    this.#back.push(origin);
    this.#forward = [];
    this.#group = group;
  }

  /**
   * Takes the place to go back to from `current`, or null for none, keeping
   * `current` for going forward again.
   */
  back(current: string): string | null {
    return this.#move(this.#back, this.#forward, current);
  }

  /**
   * Takes the place to go forward to from `current`, or null for none,
   * keeping `current` for going back again.
   */
  forward(current: string): string | null {
    return this.#move(this.#forward, this.#back, current);
  }

  // Going to the place the reader is already at would do nothing, so it's
  // skipped.
  #move(from: string[], to: string[], current: string): string | null {
    let target = from.pop();
    while (target !== undefined && target === current) target = from.pop();
    if (target === undefined) return null;
    to.push(current);
    this.#group = undefined;
    return target;
  }
}
