package parser_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/my-go-crawler/internal/parser"
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

func TestParse_ReturnsBook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(minimalBookHTML))
	defer server.Close()
	fmt.Println(server.URL, "server.URL")
	// t.Logf("test server url %s", server.URL)

	limiter := rate.NewLimiter(rate.Inf, 1) // unlimited for functional test
	p := parser.NewParser(limiter)

	b, err := p.Parse(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.Title != "Test Book" {
		t.Errorf("Title = %q, want %q", b.Title, "Test Book")
	}
	if b.Price != "£10.00" {
		t.Errorf("Price = %q, want %q", b.Price, "£10.00")
	}
	if b.Rating != "Three" {
		t.Errorf("Rating = %q, want %q", b.Rating, "Three")
	}
	if b.Category != "Fiction" {
		t.Errorf("Category = %q, want %q", b.Category, "Fiction")
	}
	if b.UPC != "abc123" {
		t.Errorf("UPC = %q, want %q", b.UPC, "abc123")
	}
}

func TestParse_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(minimalBookHTML))
	limiter := rate.NewLimiter(2, 1)
	p := parser.NewParser(limiter)

	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := p.Parse(context.Background(), server.URL); err != nil {
			t.Fatalf("parse %d failed: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	// 3 requests at 2 req/s: first is free (burst), then 2 wait ~500ms each → ≥ 1s total
	if elapsed < time.Second {
		t.Errorf("rate limit not enforced: elapsed %v, want >= 1s", elapsed)
	}
}

func TestParse_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(minimalBookHTML))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	limiter := rate.NewLimiter(rate.Every(10*time.Second), 1) // very slow — forces limiter.Wait to check ctx
	limiter.Allow()
	p := parser.NewParser(limiter)

	start := time.Now()
	b, err := p.Parse(ctx, server.URL)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected context timeout error, got nil")
	}
	if b != nil {
		t.Errorf("expected nil book, got %+v", b)
	}
	// must have returned quickly, not waited the full 10s
	if elapsed > time.Second {
		t.Errorf("Parse blocked too long (%v) — context was not respected", elapsed)
	}
	t.Logf("returned in %v with: %v", elapsed, err)
}
