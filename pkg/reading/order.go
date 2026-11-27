package reading

// MovedInReorder returns the ids that moved when before was put in the order
// after, both holding the same ids: those outside a longest run kept in its
// relative order. Dragging one Book to a new place moves only that Book,
// though every Book between its old and new places shifts by one. It keys
// "the latest move of each Book wins" (ADR 0024) for a reorder that sends
// the whole order, as the web's does.
func MovedInReorder(before, after []int) map[int]bool {
	place := make(map[int]int, len(before))
	for i, id := range before {
		place[id] = i
	}
	// The ids of after that were also in before, by their old places.
	var ids, places []int
	for _, id := range after {
		if p, ok := place[id]; ok {
			ids = append(ids, id)
			places = append(places, p)
		}
	}
	// Longest increasing run of old places, by patience sorting: tails[k] is
	// the index in places ending the best run of length k+1, and prev links
	// each index to the one before it in its run.
	tails := []int{}
	prev := make([]int, len(places))
	for i, p := range places {
		lo, hi := 0, len(tails)
		for lo < hi {
			mid := (lo + hi) / 2
			if places[tails[mid]] < p {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		if lo > 0 {
			prev[i] = tails[lo-1]
		} else {
			prev[i] = -1
		}
		if lo == len(tails) {
			tails = append(tails, i)
		} else {
			tails[lo] = i
		}
	}
	kept := make(map[int]bool, len(tails))
	if len(tails) > 0 {
		for i := tails[len(tails)-1]; i >= 0; i = prev[i] {
			kept[ids[i]] = true
		}
	}
	moved := map[int]bool{}
	for _, id := range ids {
		if !kept[id] {
			moved[id] = true
		}
	}
	return moved
}
