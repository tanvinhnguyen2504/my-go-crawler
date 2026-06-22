package export

import (
	"bufio"
	"context"
	"encoding/json"
	"os"

	"github.com/my-go-crawler/internal/book"
)

type Export interface {
	StreamJSON(ctx context.Context, bookChan <-chan *book.Book, path string) (int, error)
}

type export struct{}

func NewExport() Export {
	return &export{}
}

func (e *export) StreamJSON(ctx context.Context, bookChan <-chan *book.Book, path string) (int, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	w.WriteString("[")
	count := 0

	for {
		select {
		case <-ctx.Done():
			w.WriteString("\n")
			return count, ctx.Err()
		case b, ok := <-bookChan:
			if !ok {
				w.WriteString("\n")
				return count, nil
			}
			data, _ := json.MarshalIndent(b, "  ", "  ")
			if count > 0 {
				w.WriteString(",")
			}
			w.WriteString("\n  ")
			w.Write(data)
			count++
		}
	}
}
