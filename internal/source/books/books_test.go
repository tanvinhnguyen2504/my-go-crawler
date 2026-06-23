package books

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func minimalBookHTML(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, `<html><body>
		<div class="product_main">
			<h1>Test Book</h1>
			<p class="price_color">£10.00</p>
			<p class="availability">In stock</p>
			<p class="star-rating Three"></p>
		</div>
		<ul class="breadcrumb">
			<li><a>Home</a></li>
			<li><a>Books</a></li>
			<li><a>Fiction</a></li>
		</ul>
		<article>
			<div id="product_description"></div>
			<p>A great book about testing.</p>
		</article>
		<div id="product_gallery">
			<img class="thumbnail" src="/media/cover.jpg">
		</div>
		<table class="table">
			<tr><th>UPC</th><td>abc123</td></tr>
			<tr><th>Product Type</th><td>Books</td></tr>
			<tr><th>Tax</th><td>£0.00</td></tr>
			<tr><th>Number of reviews</th><td>0</td></tr>
		</table>
		</body></html>`)
}

// parseWithLimiter is a testable variant that accepts a custom rate limiter.
func parseWithLimiter(ctx context.Context, limiter *rate.Limiter, bookURL string) (*book, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return scrapeBook(ctx, bookURL)
}

func TestParse_ReturnsBook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(minimalBookHTML))
	defer server.Close()

	src := &booksSource{}
	record, err := src.Parse(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.Data["title"] != "Test Book" {
		t.Errorf("title = %q, want %q", record.Data["title"], "Test Book")
	}
	if record.Data["price"] != "£10.00" {
		t.Errorf("price = %q, want %q", record.Data["price"], "£10.00")
	}
	if record.Data["upc"] != "abc123" {
		t.Errorf("upc = %q, want %q", record.Data["upc"], "abc123")
	}
}

func TestParse_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(minimalBookHTML))
	defer server.Close()

	limiter := rate.NewLimiter(2, 1)
	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := parseWithLimiter(context.Background(), limiter, server.URL); err != nil {
			t.Fatalf("parse %d failed: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	if elapsed < time.Second {
		t.Errorf("rate limit not enforced: elapsed %v, want >= 1s", elapsed)
	}
}

func TestParse_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(minimalBookHTML))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	limiter := rate.NewLimiter(rate.Every(10*time.Second), 1)
	limiter.Allow() // consume the burst token so next call blocks

	start := time.Now()
	_, err := parseWithLimiter(ctx, limiter, server.URL)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected context timeout error, got nil")
	}
	if elapsed > time.Second {
		t.Errorf("blocked too long (%v) — context was not respected", elapsed)
	}
	t.Logf("returned in %v with: %v", elapsed, err)
}
