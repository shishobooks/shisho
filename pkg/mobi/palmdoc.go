package mobi

// PalmDOC compression, the inverse of palmDOCDecompress: a back-reference
// for a run of 3 to 10 bytes found up to 2047 bytes back, a space and the
// letter after it in one byte, a byte that needs no escaping as itself, and
// a literal run of up to 8 bytes for the rest.

const (
	palmDOCMaxDistance = 2047
	palmDOCMinMatch    = 3
	palmDOCMaxMatch    = 10
	// palmDOCChain caps how many earlier places with the same three bytes
	// are tried for each position, which bounds the work on repetitive text.
	palmDOCChain = 32
)

// palmDOCCompress compresses one text record.
func palmDOCCompress(in []byte) []byte {
	out := make([]byte, 0, len(in))
	// head maps a hash of three bytes to the last position they were seen
	// at, plus one; prev links each position to the one before it with
	// the same hash.
	var head [1 << 12]int
	prev := make([]int, len(in))
	hash := func(i int) int {
		return (int(in[i])<<8 ^ int(in[i+1])<<4 ^ int(in[i+2])) & (len(head) - 1)
	}
	insert := func(i int) {
		if i+palmDOCMinMatch <= len(in) {
			h := hash(i)
			prev[i] = head[h]
			head[h] = i + 1
		}
	}

	for i := 0; i < len(in); {
		if dist, length := palmDOCMatch(in, i, head[:], prev, hash); length >= palmDOCMinMatch {
			pair := 0x8000 | dist<<3 | (length - palmDOCMinMatch)
			out = append(out, byte(pair>>8), byte(pair))
			for j := i; j < i+length; j++ {
				insert(j)
			}
			i += length
			continue
		}
		c := in[i]
		switch {
		case c == ' ' && i+1 < len(in) && in[i+1] >= 0x40 && in[i+1] <= 0x7F:
			out = append(out, in[i+1]^0x80)
			insert(i)
			insert(i + 1)
			i += 2
		case c == 0 || c >= 0x09 && c <= 0x7F:
			out = append(out, c)
			insert(i)
			i++
		default:
			// A literal run takes the bytes that can't stand for themselves,
			// up to 8 of them.
			n := 1
			for n < 8 && i+n < len(in) && needsLiteral(in[i+n]) {
				n++
			}
			out = append(out, byte(n))
			out = append(out, in[i:i+n]...)
			for j := i; j < i+n; j++ {
				insert(j)
			}
			i += n
		}
	}
	return out
}

func needsLiteral(c byte) bool {
	return c >= 0x01 && c <= 0x08 || c >= 0x80
}

// palmDOCMatch returns the distance and length of the longest earlier run
// matching the bytes at i, or a length of 0 when there is none long enough.
func palmDOCMatch(in []byte, i int, head []int, prev []int, hash func(int) int) (int, int) {
	if i+palmDOCMinMatch > len(in) {
		return 0, 0
	}
	limit := min(palmDOCMaxMatch, len(in)-i)
	bestDist, bestLen := 0, 0
	for p, tries := head[hash(i)], 0; p > 0 && tries < palmDOCChain; p, tries = prev[p-1], tries+1 {
		j := p - 1
		dist := i - j
		if dist > palmDOCMaxDistance {
			break
		}
		n := 0
		for n < limit && in[j+n] == in[i+n] {
			n++
		}
		if n > bestLen {
			bestDist, bestLen = dist, n
			if n == limit {
				break
			}
		}
	}
	if bestLen < palmDOCMinMatch {
		return 0, 0
	}
	return bestDist, bestLen
}
