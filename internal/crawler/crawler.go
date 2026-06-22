package crawler

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gocolly/colly/v2"
)

const (
	MAIN_URL = "https://books.toscrape.com/catalogue"
)

func GetAllLinksBookByPage(ctx context.Context, pageIndex int, urlChan chan<- string) error {
	pageUrl := fmt.Sprintf("%s/page-%d.html", MAIN_URL, pageIndex)

	c := colly.NewCollector()
	c.OnHTML("article.product_pod h3 a", func(e *colly.HTMLElement) {
		href := e.Attr("href")

		base, _ := url.Parse(e.Request.URL.String())
		rel, _ := url.Parse(href)
		detailURL := base.ResolveReference(rel)
		select {
		case urlChan <- detailURL.String():
		case <-ctx.Done():
		default:
		}
	})

	if err := c.Visit(pageUrl); err != nil {
		return err
	}
	return ctx.Err()
}
