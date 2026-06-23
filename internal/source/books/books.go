package books

import (
	"context"
	"net/url"
	"strings"

	"github.com/gocolly/colly/v2"
	"github.com/my-go-crawler/internal/source"
	"github.com/my-go-crawler/pkg"
	"golang.org/x/time/rate"
)

func init() { source.Register("books", &booksSource{}) }

const baseURL = "https://books.toscrape.com/catalogue"

type book struct {
	URL          string
	Title        string
	Description  string
	Price        string
	Tax          string
	Availability string
	Rating       string
	UPC          string
	ProductType  string
	NumReviews   string
	Category     string
	ImageURL     string
}

type booksSource struct{}

func (b *booksSource) SeedURLs() []string {
	return []string{
		baseURL + "/page-1.html",
		baseURL + "/page-2.html",
		baseURL + "/page-3.html",
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

func (b *booksSource) Parse(ctx context.Context, bookURL string) (*source.Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := rate.NewLimiter(rate.Inf, 1).Wait(ctx); err != nil {
		return nil, err
	}
	bk, err := scrapeBook(ctx, bookURL)
	if err != nil {
		return nil, err
	}
	return &source.Record{
		Source: "books",
		Data: map[string]any{
			"title":        bk.Title,
			"price":        bk.Price,
			"rating":       bk.Rating,
			"category":     bk.Category,
			"upc":          bk.UPC,
			"availability": bk.Availability,
			"description":  bk.Description,
			"url":          bk.URL,
		},
	}, nil
}

// scrapeBook fetches and parses a single book page. Separated so tests can
// inject a custom rate limiter via parseWithLimiter.
func scrapeBook(_ context.Context, bookURL string) (*book, error) {
	c := colly.NewCollector()
	bk := &book{URL: bookURL}
	tableData := map[string]string{}

	c.OnHTML("div.product_main h1", func(e *colly.HTMLElement) {
		bk.Title = e.Text
	})
	c.OnHTML("div.product_main p.price_color", func(e *colly.HTMLElement) {
		bk.Price = e.Text
	})
	c.OnHTML(".product_main .availability", func(e *colly.HTMLElement) {
		bk.Availability = strings.TrimSpace(e.Text)
	})
	c.OnHTML("p.star-rating", func(e *colly.HTMLElement) {
		for _, class := range strings.Fields(e.Attr("class")) {
			if class != "star-rating" {
				bk.Rating = class
			}
		}
	})
	c.OnHTML("#product_description ~ p", func(e *colly.HTMLElement) {
		bk.Description = e.Text
	})
	c.OnHTML(".breadcrumb li:nth-child(3) a", func(e *colly.HTMLElement) {
		bk.Category = e.Text
	})
	c.OnHTML("#product_gallery .thumbnail", func(e *colly.HTMLElement) {
		bk.ImageURL = e.Attr("src")
	})
	c.OnHTML("table.table tr", func(e *colly.HTMLElement) {
		tableData[e.ChildText("th")] = e.ChildText("td")
	})

	if err := c.Visit(bookURL); err != nil {
		return nil, err
	}

	bk.UPC = tableData["UPC"]
	bk.ProductType = tableData["Product Type"]
	bk.Tax = tableData["Tax"]
	bk.NumReviews = tableData["Number of reviews"]

	pkg.DebugJson(bk)
	return bk, nil
}
