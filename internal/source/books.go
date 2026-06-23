package source

import (
	"context"
	"net/url"

	"github.com/gocolly/colly/v2"
	"github.com/my-go-crawler/internal/crawler"
	"github.com/my-go-crawler/internal/parser"
	"golang.org/x/time/rate"
)

func init() { Register("books", &booksSource{}) }

type booksSource struct{}

func (b *booksSource) SeedURLs() []string {
	return []string{
		crawler.MAIN_URL + "/page-1.html",
		crawler.MAIN_URL + "/page-2.html",
		crawler.MAIN_URL + "/page-3.html",
	}
}

func (b *booksSource) Crawl(ctx context.Context, seedURL string) ([]string, error) {
	var urls []string
	c := colly.NewCollector()
	c.OnHTML("article.product_pod h3 a", func(e *colly.HTMLElement) {
		base, _ := url.Parse(e.Request.URL.String())
		rel, _ := url.Parse(e.Attr("href"))
		urls = append(urls, base.ResolveReference(rel).String())
	})
	if err := c.Visit(seedURL); err != nil {
		return nil, err
	}
	return urls, ctx.Err()
}

func (b *booksSource) Parse(ctx context.Context, bookURL string) (*Record, error) {
	p := parser.NewParser(rate.NewLimiter(rate.Inf, 1))
	book, err := p.Parse(ctx, bookURL)
	if err != nil {
		return nil, err
	}
	return &Record{
		Source: "books",
		Data: map[string]any{
			"title":        book.Title,
			"price":        book.Price,
			"rating":       book.Rating,
			"category":     book.Category,
			"upc":          book.UPC,
			"availability": book.Availability,
			"description":  book.Description,
			"url":          book.URL,
		},
	}, nil
}
