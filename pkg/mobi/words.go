package mobi

import (
	"bytes"
	"context"
	"io"
	"os"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/wordcount"
)

// WordCount counts the words in the text of the MOBI or AZW3 file at path,
// the book ReadContent reads from it. It returns ErrDRMProtected for a file
// whose text is encrypted.
func WordCount(ctx context.Context, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return countWords(ctx, f, stat.Size())
}

func countWords(ctx context.Context, r io.ReaderAt, size int64) (int, error) {
	content, err := ReadContent(ctx, r, size)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, doc := range content.Documents {
		n, err := wordcount.HTML(bytes.NewReader(doc.Content))
		if err != nil {
			return 0, errors.Wrapf(err, "count words in %s", doc.Name)
		}
		total += n
	}
	return total, nil
}
