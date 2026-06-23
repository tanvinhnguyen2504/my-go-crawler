package source

import "context"

type Record struct {
	Source string
	Data   map[string]any
}

type SourceParser interface {
	SeedURLs() []string
	Crawl(ctx context.Context, seedURL string) ([]string, error)
	Parse(ctx context.Context, url string) (*Record, error)
}

var Registry = map[string]SourceParser{}

func Register(name string, p SourceParser) {
	Registry[name] = p
}
