# FeedRewind

A web app that can take an arbitrary blog, discover the full list of posts from it and serve an RSS feed that starts at the beginning and populates on a specified schedule.

If video works better for you, here's an [overview](https://youtu.be/Gyodk7r9F60) that gives a sense of the product and the engineering behind it.

The code is published for demo purposes. You can crawl blogs, create free accounts and have feeds come out as expected when running locally. Like in a typical SaaS, full functionality depends on integrations with other services which need more involved setup.

## Architecture

Hosted app has a typical PaaS structure with a `web` and a `worker` role. About a third of the code is a [crawler](https://github.com/ilidemi/feedrewind/tree/master/crawler) that can scan an arbitrary blog and extract chronologically ordered list of posts from it in a close to minimal number of requests. When the crawler runs in a worker, the progress is communicated to web instances via Postgres listen/notify and streamed to the user via a websocket. When multiple users request the same blog at the same time, they get assigned to the same crawling job and get to watch the same progress bar.

Iterating on crawler heuristics is aided by an [admin tool](https://github.com/ilidemi/feedrewind/blob/master/cmd/crawl/crawl.go) that runs it in parallel on a known set of blogs (not included in the repo), compares the results with manually vetted ground truth and produces a report [like this one](https://html-preview.github.io/?url=https://github.com/ilidemi/feedrewind/blob/master/cmd/crawl/example_report.html).

Users are expected to come from any timezone and still receive their feeds in the morning - the list of timezones is vendored from Go standard library and augmented with human-friendly names from [vvo/tzdb](https://github.com/vvo/tzdb), making sure the two sources match.

For the sake of compatibility with the previous Ruby codebase, worker infrastructure is handwritten to exactly match the DB schema and the locking contract as Ruby's [delayed_job](https://github.com/collectiveidea/delayed_job). Fuzzy date parsing is [ported](https://github.com/ilidemi/feedrewind/tree/master/crawler/rubydate) from Ruby runtime, as no appropriate Go replacement existed at the moment.

Dependencies: Go 1.22+, Postgres 12+

Dev dependencies: Tailwind CSS 3.0.23+ (vendored for Windows and Linux), golangci-lint 1.61+ (as a pre-commit hook)

## Running locally

Build:
```
go build
```

Run tests:
```
go test ./... -tags testing
```

Make sure that `config/demo.json` has the right DB credentials.

Initialize the DB:
```
createdb feedrewind_development
psql -d feedrewind_development -f db/structure.sql
go run . demo-seed-db
```

Run (in separate shells):
```
go run . web
```
```
go run . worker
```

Navigate to `http://localhost:3000` and click around, making sure to use a free account and RSS delivery (not email).

Supported in the demo:
- Crawling blogs
- User accounts and settings
- RSS delivery

Not supported in the demo:
- Crawling Tumblr (needs API key)
- Sending emails (needs Postmark key)
- Payments (needs Stripe key)
- Various maintenance jobs (needs a seeded DB and AWS key)
- Sending out events to Amplitude and Slack (needs API keys)
- End to end tests (need all of the above)
- Incident management is based on logs and set up outside of the code
