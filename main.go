package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/my-go-crawler/internal/book"
	"github.com/my-go-crawler/internal/crawler"
	"github.com/my-go-crawler/internal/export"
	"github.com/my-go-crawler/internal/parser"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

const (
	crawlWorkers = 3
	parseWorkers = 5
)

func main() {
	pageIndex := []int{1, 2, 3}
	pageChan := make(chan int, len(pageIndex))
	urlChan := make(chan string, 100)
	bookChan := make(chan *book.Book, 50)
	g, ctx := errgroup.WithContext(context.Background())

	for _, page := range pageIndex {
		pageChan <- page
	}
	close(pageChan)

	// Stage 1: crawl pages → collect book URLs
	var crawlWg sync.WaitGroup
	for i := 0; i < crawlWorkers; i++ {
		crawlWg.Add(1)
		g.Go(func() error {
			defer crawlWg.Done()
			for page := range pageChan {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				if err := crawler.GetAllLinksBookByPage(ctx, page, urlChan); err != nil {
					return err
				}
			}
			return nil
		})
	}
	go func() { crawlWg.Wait(); close(urlChan) }()

	// Stage 2: parse book detail pages
	limiter := rate.NewLimiter(rate.Every(200*time.Microsecond), 5)

	var parseWg sync.WaitGroup
	for i := 0; i < parseWorkers; i++ {
		parseWg.Add(1)
		g.Go(func() error {
			defer parseWg.Done()
			p := parser.NewParser(limiter)
			for url := range urlChan {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				b, err := p.Parse(ctx, url)
				if err != nil {
					return err
				}
				select {
				case bookChan <- b:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
	}
	go func() { parseWg.Wait(); close(bookChan) }()

	// Stage 3: stream results directly to file as they arrive
	g.Go(func() error {
		exporter := export.NewExport()
		n, err := exporter.StreamJSON(ctx, bookChan, "books.json")
		if err != nil {
			return err
		}
		fmt.Printf("Wrote %d books to books.json\n", n)
		return nil
	})

	if err := g.Wait(); err != nil {
		fmt.Fprintln(os.Stderr, "pipeline error:", err)
		os.Exit(1)
	}
}
