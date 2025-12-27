# Blog Search - URL Extraction and Content Pipeline

A flexible, multi-stage pipeline system for extracting URLs from various sources (sitemaps, RSS feeds, paginated HTML pages) and processing content for blog/article search.

## Available Commands

### 1. `extract` - Test URL Extraction from HTML Files

Test if extractors work on HTML files before running the full pipeline.

```bash
go run . extract <html-file-path> <extractor-type>
```

**Extractor types:**
- `se-radio` - For se-radio.net pages
- `data-engineering-podcast` - For dataengineeringpodcast.com pages
- `generic` - Generic extractor for common HTML patterns

**Example:**
```bash
go run . extract html-page-examples/shopify.html generic
```

---

### 2. `pipeline` - Generic Pipeline System (Recommended)

The main pipeline system that supports multiple source types.

#### **Sitemap Pipeline:**
```bash
go run . pipeline sitemap <sitemap-url> [url-fetcher-workers] [content-workers] [-url-filter=<path>]
```

**Example:**
```bash
MONGO_URI="mongodb://admin:password@localhost:27017" \
go run . pipeline sitemap https://engineering.fb.com/post-sitemap.xml

# With filter to only process blog URLs
go run . pipeline sitemap https://www.cncf.io/sitemap.xml -url-filter=/blog
```

#### **RSS Pipeline:**
```bash
go run . pipeline rss <rss-url> [url-fetcher-workers] [content-workers] [-url-filter=<path>]
```

**Example:**
```bash
MONGO_URI="mongodb://admin:password@localhost:27017" \
go run . pipeline rss https://dropbox.tech/feed.xml
```

#### **Pagination Pipeline:**
```bash
go run . pipeline paginate <base-url> <page-pattern> [extractor-type] [pages-per-batch] [page-gen-workers] [html-fetcher-workers] [content-workers] [-url-filter=<path>]
```

**Parameters:**
- `base-url`: Base URL of the website (e.g., `https://se-radio.net`)
- `page-pattern`: URL pattern with `%d` placeholder (e.g., `/page/%d` or `/?currentPage=%d`)
- `extractor-type`: `se-radio`, `data-engineering-podcast`, or `generic` (default: `se-radio`)
- `pages-per-batch`: Pages processed per batch (default: 10)
- `page-gen-workers`: Workers for generating page URLs (default: 1)
- `html-fetcher-workers`: Workers for extracting URLs from pages (default: 3)
- `content-workers`: Workers for fetching and saving content (default: 5)

**Example:**
```bash
MONGO_URI="mongodb://admin:password@localhost:27017" \
go run . pipeline paginate https://se-radio.net /page/%d se-radio

# With generic extractor
go run . pipeline paginate https://www.shopify.com/blog /page/%d generic
```

---

### 3. `paginate` - Legacy Pagination (Old System)

```bash
go run . paginate [baseURLPattern] [pagesPerBatch] [urlFetcherWorkers] [contentWorkers]
```

**Note:** This is the old two-level worker system. Use `pipeline paginate` instead.

---

### 4. `replicate` - MongoDB to Postgres Replication

```bash
MONGO_URI="mongodb://admin:password@localhost:27017" \
POSTGRES_DSN="postgres://user:pass@localhost:5432/blogsearch?sslmode=disable" \
go run . replicate
```

---

## How the Pipeline Works

### Architecture Overview

The pipeline is a multi-stage processing system that extracts URLs and processes content:

```
[URL Generator/Fetcher] → [Step 1] → [Step 2] → ... → [Content Consumer] → [MongoDB]
```

### Pipeline Components

1. **Pipeline Steps** - Extract URLs from previous step's URLs
   - **First Step:** Can use `URLGenerator` (generates URLs) or `URLFetcher` (extracts from base URL)
   - **Subsequent Steps:** Use `URLFetcher` to extract URLs from each URL from previous step

2. **Content Consumer** - Final stage that:
   - Receives URLs from the last step
   - Fetches HTML content
   - Extracts article text and title
   - Saves to MongoDB

### Pipeline Types

#### **1. Sitemap Pipeline** (1 step)
```
Base URL → [Sitemap Fetcher] → [Content Consumer]
```
- Fetches sitemap XML
- Extracts all URLs
- Processes each URL

#### **2. RSS Pipeline** (1 step)
```
Base URL → [RSS Fetcher] → [Content Consumer]
```
- Fetches RSS feed
- Extracts article URLs
- Processes each URL

#### **3. Pagination Pipeline** (2 steps)
```
[Page Generator] → [HTML Page Fetcher] → [Content Consumer]
```
- **Step 1 (Generator):** Generates page URLs (e.g., `/page/1`, `/page/2`)
  - Uses HTTP HEAD requests to check if pages exist
  - Checks content every 10 pages for empty markers (e.g., "0 episodes found")
- **Step 2 (Fetcher):** Extracts article URLs from each page
  - Uses site-specific or generic extractor
- **Content Consumer:** Processes all extracted article URLs

### How It Works Internally

1. **Channel-Based Communication:**
   - Each step communicates via Go channels
   - Buffered channels allow parallel processing

2. **Worker Pools:**
   - Each step has configurable worker counts
   - Workers process URLs concurrently

3. **Flow:**
   ```
   Step 1 generates/fetches URLs → sends to channel
   Step 2 workers read from channel → extract URLs → send to next channel
   Content workers read from channel → fetch content → save to DB
   ```

4. **URL Filters:**
   - Applied at fetcher level
   - Can filter by path (e.g., only `/blog` URLs)
   - Multiple filters can be chained

5. **Error Handling:**
   - Errors are logged but don't stop the pipeline
   - Each URL is processed independently
   - Failed URLs are skipped, others continue

### Example: Pagination Pipeline Flow

```
1. PageRangeGenerator generates: ["/page/1", "/page/2", "/page/3"]
2. HTML Page Fetcher workers process each page:
   - Page 1 → extracts ["/article/1", "/article/2"]
   - Page 2 → extracts ["/article/3", "/article/4"]
3. Content Consumer workers process all articles:
   - Fetches HTML, extracts content, saves to MongoDB
```

## URL Extractors

### Site-Specific Extractors

- **`se-radio`** - Extracts URLs from se-radio.net pages
  - Looks for: `div.col-12.megaphone-order-1.col-lg-8` → `article.megaphone-item` → `h2.entry-title a`

- **`data-engineering-podcast`** - Extracts URLs from dataengineeringpodcast.com
  - Looks for: `a.episodeLink` with href starting with `/episodepage/`

### Generic Extractor

The `generic` extractor works for many websites using common HTML patterns:

1. Links within `<article>` tags
2. Links within `<main>` content area
3. Links with common article-related classes (`entry-title`, `post-title`, `article-link`, etc.)
4. All links excluding navigation/footer/header (fallback)

**Features:**
- Converts relative URLs to absolute using base URL detection
- Checks `<base>` tag, canonical links, and `og:url` meta tags
- Filters out common non-content links (tags, categories, archives, etc.)

## URL Filters

Use the `-url-filter` flag to filter URLs by path segment:

```bash
# Only process URLs containing "/blog"
go run . pipeline sitemap https://example.com/sitemap.xml -url-filter=/blog
```

## Environment Variables

- `MONGO_URI` - MongoDB connection string (default: `mongodb://admin:password@localhost:27017`)
- `POSTGRES_DSN` - Postgres connection string (for replication)

## Testing

Run all tests:
```bash
go test ./...
```

Run tests for a specific package:
```bash
go test ./pkg/pipeline/...
```

Run tests with verbose output:
```bash
go test ./... -v
```

## Project Structure

```
blog-search/
├── main.go                    # Command-line interface
├── pkg/
│   ├── pipeline/              # Generic pipeline system
│   │   ├── pipeline.go        # Core pipeline orchestration
│   │   ├── fetchers.go        # URL fetchers and generators
│   │   ├── builders.go        # Pipeline builders
│   │   └── processor.go       # Content processing
│   ├── sites/                 # Site-specific extractors
│   │   ├── seradio.go
│   │   ├── dataengineeringpodcast.go
│   │   └── generic.go         # Generic extractor
│   ├── urls/                  # URL fetching and filtering
│   ├── content/               # Content extraction
│   ├── db/                    # Database clients
│   └── httpclient/            # HTTP client configurations
└── html-page-examples/        # HTML test files
```

## Features

- **Multi-stage pipelines** - Support for complex URL extraction workflows
- **Parallel processing** - Configurable worker pools for each stage
- **Flexible extractors** - Site-specific and generic extractors
- **URL filtering** - Filter URLs by path or other criteria
- **Error resilience** - Failed URLs don't stop the pipeline
- **Comprehensive logging** - Detailed logs for debugging


