package parser

import (
	"context"
	"strings"

	"github.com/my-go-crawler/internal/book"
	"github.com/my-go-crawler/pkg"

	"github.com/gocolly/colly/v2"
	"golang.org/x/time/rate"
)

type Parser interface {
	Parse(ctx context.Context, url string) (*book.Book, error)
}

type parser struct {
	limiter *rate.Limiter
}

func NewParser(limiter *rate.Limiter) Parser {
	return &parser{limiter: limiter}
}

func (p *parser) Parse(ctx context.Context, url string) (*book.Book, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := p.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	c := colly.NewCollector()
	b := &book.Book{URL: url}
	tableData := map[string]string{}

	c.OnHTML("div.product_main h1", func(e *colly.HTMLElement) {
		b.Title = e.Text
	})
	c.OnHTML("div.product_main p.price_color", func(e *colly.HTMLElement) {
		b.Price = e.Text
	})
	c.OnHTML(".product_main .availability", func(e *colly.HTMLElement) {
		b.Availability = strings.TrimSpace(e.Text)
	})
	c.OnHTML("p.star-rating", func(e *colly.HTMLElement) {
		for _, class := range strings.Fields(e.Attr("class")) {
			if class != "star-rating" {
				b.Rating = class // One, Two, Three, Four, Five
			}
		}
	})
	c.OnHTML("#product_description ~ p", func(e *colly.HTMLElement) {
		b.Description = e.Text
	})
	c.OnHTML(".breadcrumb li:nth-child(3) a", func(e *colly.HTMLElement) {
		b.Category = e.Text
	})
	c.OnHTML("#product_gallery .thumbnail", func(e *colly.HTMLElement) {
		b.ImageURL = e.Attr("src")
	})
	c.OnHTML("table.table tr", func(e *colly.HTMLElement) {
		tableData[e.ChildText("th")] = e.ChildText("td")
	})

	err := c.Visit(url)

	b.UPC = tableData["UPC"]
	b.ProductType = tableData["Product Type"]
	b.Tax = tableData["Tax"]
	b.NumReviews = tableData["Number of reviews"]

	pkg.DebugJson(b)

	return b, err
}
