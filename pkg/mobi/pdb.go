package mobi

import (
	"encoding/binary"
	"io"

	"github.com/pkg/errors"
)

const (
	// Header records hold the MOBI header, EXTH, and padding; Calibre's are
	// about 9 KB. Cover images are rarely more than a few megabytes.
	maxHeaderRecord = 1 << 20
	maxImageRecord  = 64 << 20

	pdbHeaderSize    = 78
	pdbRecordEntry   = 8
	pdbTypeOffset    = 60
	pdbUniqueIDSeed  = 68
	pdbRecordsOffset = 76
)

// errNotMOBI is returned for a file that is not a Palm database of type
// BOOKMOBI, such as a Topaz or KFX file, or a file with the wrong extension.
var errNotMOBI = errors.New("not a MOBI file")

// pdb is a Palm database: a header and a list of records, each starting at
// the offset the record list gives and ending where the next one starts.
type pdb struct {
	r       io.ReaderAt
	offsets []int64 // one per record, plus the file size
}

func readPDB(r io.ReaderAt, size int64) (*pdb, error) {
	header := make([]byte, pdbHeaderSize)
	if _, err := r.ReadAt(header, 0); err != nil {
		return nil, errors.Wrap(errNotMOBI, "file is too short for a Palm database header")
	}
	if string(header[pdbTypeOffset:pdbTypeOffset+8]) != "BOOKMOBI" {
		return nil, errors.WithStack(errNotMOBI)
	}

	count := int(binary.BigEndian.Uint16(header[pdbRecordsOffset:]))
	if count == 0 {
		return nil, errors.New("MOBI file has no records")
	}
	list := make([]byte, count*pdbRecordEntry)
	if _, err := r.ReadAt(list, pdbHeaderSize); err != nil {
		return nil, errors.Wrap(err, "read record list")
	}

	offsets := make([]int64, count+1)
	for i := range count {
		offsets[i] = int64(binary.BigEndian.Uint32(list[i*pdbRecordEntry:]))
	}
	offsets[count] = size
	listEnd := int64(pdbHeaderSize + len(list))
	for i := range count {
		if offsets[i] < listEnd || offsets[i] > offsets[i+1] {
			return nil, errors.Errorf("record %d has an invalid offset", i)
		}
	}
	return &pdb{r: r, offsets: offsets}, nil
}

func (p *pdb) count() int {
	return len(p.offsets) - 1
}

// record reads record i, refusing one larger than limit bytes so a crafted
// record list cannot make the parser allocate the whole file.
func (p *pdb) record(i int, limit int64) ([]byte, error) {
	if i < 0 || i >= p.count() {
		return nil, errors.Errorf("record %d is out of range", i)
	}
	size := p.offsets[i+1] - p.offsets[i]
	if size > limit {
		return nil, errors.Errorf("record %d is too large", i)
	}
	data := make([]byte, size)
	if _, err := p.r.ReadAt(data, p.offsets[i]); err != nil {
		return nil, errors.Wrapf(err, "read record %d", i)
	}
	return data, nil
}
