package crawler

import (
	"context"
	"errors"
	"feedrewind/config"
	"feedrewind/oops"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

type PuppeteerFindLoadMoreButton func(*rod.Page) (*rod.Element, error)

type PuppeteerPage struct {
	Content               string
	MaybeTopScreenshot    []byte
	MaybeBottomScreenshot []byte
}

type PuppeteerClient interface {
	Fetch(
		uri *url.URL, feedEntryCurisTitlesMap CanonicalUriMap[MaybeLinkTitle], crawlCtx *CrawlContext,
		logger Logger, findLoadMoreButton PuppeteerFindLoadMoreButton, extendedScrollTime bool,
	) (*PuppeteerPage, error)
}

type PuppeteerClientImpl struct {
}

func NewPuppeteerClientImpl() *PuppeteerClientImpl {
	if maxBrowserCount == 0 {
		panic("Set max browser count before invoking puppeteer")
	}
	return &PuppeteerClientImpl{}
}

var maxBrowserCount int
var browserLimitCh chan struct{}

func SetMaxBrowserCount(count int) {
	maxBrowserCount = count
	browserLimitCh = make(chan struct{}, count)
	for range count {
		browserLimitCh <- struct{}{}
	}
}

const defaultMaxScrollTime = 30 * time.Second
const extendedMaxScrollTime = 90 * time.Second

func (c *PuppeteerClientImpl) Fetch(
	uri *url.URL, feedEntryCurisTitlesMap CanonicalUriMap[MaybeLinkTitle], crawlCtx *CrawlContext,
	logger Logger, findLoadMoreButton PuppeteerFindLoadMoreButton, extendedScrollTime bool,
) (result *PuppeteerPage, retErr error) {
	progressLogger := crawlCtx.ProgressLogger
	logger.Info("Puppeteer start: %s", uri)
	progressLogger.LogAndSavePuppeteerStart()
	isInitialRequest := true
	puppeteerStart := time.Now()

	select {
	case <-browserLimitCh:
	default:
		logger.Warn("Out of browser instances (%d)", maxBrowserCount)
		<-browserLimitCh
	}
	defer func() {
		browserLimitCh <- struct{}{}
	}()
	browserAcquiredTime := time.Now()
	logger.Info("Browser acquired in %v", browserAcquiredTime.Sub(puppeteerStart))

	launcher := launcher.New()
	if config.Cfg.IsHeroku {
		launcher = launcher.Bin("chrome").NoSandbox(true)
	}
	defer launcher.Kill()
	browserUrl, err := launcher.Launch()
	if err != nil {
		return nil, oops.Wrap(err)
	}
	browser := rod.New().ControlURL(browserUrl)
	err = browser.Connect()
	if err != nil {
		return nil, oops.Wrap(err)
	}
	logger.Info("Connected to the browser")
	maxScrollTime := defaultMaxScrollTime
	if extendedScrollTime {
		maxScrollTime = extendedMaxScrollTime
	}
	maxInitialWaitTime := 15 * time.Second

	errorsCount := 0
	for {
		var rawPage *rod.Page
		result, err := func() (*PuppeteerPage, error) {
			var err error
			rawPage, err = browser.Page(proto.TargetCreateTarget{}) //nolint:exhaustruct
			if err != nil {
				return nil, oops.Wrap(err)
			}
			page := rawPage.Timeout(maxInitialWaitTime + maxScrollTime + 10*time.Second)

			hijackRouter := page.HijackRequests()
			err = hijackRouter.Add("*", proto.NetworkResourceTypeImage, func(h *rod.Hijack) {
				h.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			})
			if err != nil {
				return nil, oops.Wrap(err)
			}
			err = hijackRouter.Add("*", proto.NetworkResourceTypeFont, func(h *rod.Hijack) {
				h.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			})
			if err != nil {
				return nil, oops.Wrap(err)
			}
			go hijackRouter.Run()
			defer func() {
				if err := hijackRouter.Stop(); err != nil {
					if errors.Is(err, context.DeadlineExceeded) {
						return
					}
					var opError1, opError2 *net.OpError
					if errors.As(err, &opError1) && errors.As(retErr, &opError2) {
						return
					}
					logger.Warn("Hijack stop error: %v", err)
				}
			}()
			scrollablePage := newScrollablePage(page)

			if isInitialRequest {
				isInitialRequest = false
			} else {
				progressLogger.LogAndSavePuppeteerStart()
			}
			err = page.Navigate(uri.String())
			if err == nil {
				logger.Info("Waiting till idle")
				waitRequestIdleStart := time.Now()
				page.Timeout(maxInitialWaitTime).
					WaitRequestIdle(500*time.Millisecond, []string{".+"}, nil, nil)()
				logger.Info("Waiting till idle took %v", time.Since(waitRequestIdleStart).Round(time.Second))
			}
			progressLogger.LogAndSavePuppeteer()
			if err != nil {
				return nil, oops.Wrap(err)
			}

			initialContent, err := page.HTML()
			if err != nil {
				return nil, oops.Wrap(err)
			}
			initialDocument, err := parseHtml(initialContent, logger)
			if err != nil {
				return nil, oops.Wrap(err)
			}
			initialLinks := extractLinks(
				initialDocument, uri, nil, map[string]*Link{}, logger, includeXPathNone,
			)
			isScrollingAllowed := false
			for _, link := range initialLinks {
				if feedEntryCurisTitlesMap.Contains(link.Curi) {
					isScrollingAllowed = true
					break
				}
			}

			var content string
			var maybeTopScreenshot, maybeBottomScreenshot []byte
			if isScrollingAllowed {
				if findLoadMoreButton != nil {
					loadMoreButton, err := findLoadMoreButton(page)
					if err != nil {
						logger.Info("Find load more button error: %v", err)
					}
					for loadMoreButton != nil {
						logger.Info("Clicking load more button")
						progressLogger.LogAndSavePuppeteerStart()
						err := scrollablePage.waitAndScroll(logger, loadMoreButton, maxScrollTime)
						progressLogger.LogAndSavePuppeteer()
						if err != nil {
							return nil, err
						}
						loadMoreButton, err = findLoadMoreButton(page)
						if err != nil {
							logger.Info("Find load more button error: %v", err)
						}
					}
				} else {
					logger.Info("Scrolling")
					progressLogger.LogAndSavePuppeteerStart()
					err := scrollablePage.waitAndScroll(logger, nil, maxScrollTime)
					progressLogger.LogAndSavePuppeteer()
					if err != nil {
						return nil, err
					}
				}
				content, err = page.HTML()
				if err != nil {
					return nil, oops.Wrap(err)
				}
				//nolint:exhaustruct
				maybeBottomScreenshot, err = page.Screenshot(false, &proto.PageCaptureScreenshot{
					Format:           proto.PageCaptureScreenshotFormatPng,
					OptimizeForSpeed: true,
				})
				if err != nil {
					maybeBottomScreenshot = nil
					logger.Warn("Couldn't capture bottom screenshot: %v", err)
				}
				var evalOptions rod.EvalOptions
				evalOptions.JS = "() => window.scroll(0, 0)"
				_, err := page.Timeout(3 * time.Second).Evaluate(&evalOptions)
				if err != nil {
					logger.Warn("Couldn't scroll up: %v", err)
				} else {
					//nolint:exhaustruct
					maybeTopScreenshot, err = page.Screenshot(false, &proto.PageCaptureScreenshot{
						Format:           proto.PageCaptureScreenshotFormatPng,
						OptimizeForSpeed: true,
					})
					if err != nil {
						maybeTopScreenshot = nil
						logger.Warn("Couldn't capture top screenshot: %v", err)
					}
				}
			} else {
				logger.Info("Puppeteer didn't find any feed links on initial load")
				content = initialContent
			}

			scrollablePage.Mutex.Lock()
			finishedRequests := scrollablePage.FinishedRequests
			scrollablePage.Mutex.Unlock()
			crawlCtx.PuppeteerRequestsMade += finishedRequests
			logger.Info(
				"Puppeteer done (%v, %v wait, %d req)",
				time.Since(puppeteerStart), browserAcquiredTime.Sub(puppeteerStart), finishedRequests,
			)

			return &PuppeteerPage{
				Content:               content,
				MaybeTopScreenshot:    maybeTopScreenshot,
				MaybeBottomScreenshot: maybeBottomScreenshot,
			}, nil
		}()
		if err != nil {
			if opError := (&net.OpError{}); errors.As(err, &opError) { //nolint:exhaustruct
				logger.Error("Unrecoverable Puppeteer error: %v", err)
				return nil, err
			}
			errorsCount++
			logger.Info("Recovered Puppeteer error (%d): %v", errorsCount, err)
			progressLogger.LogAndSavePuppeteer()
			if errorsCount >= 3 {
				return nil, oops.Wrapf(err, "Puppeteer error")
			}
			continue
		}

		return result, nil
	}
}

type scrollablePage struct {
	Page             *rod.Page
	LastEventTime    time.Time
	OngoingRequests  int
	FinishedRequests int
	Mutex            sync.Mutex
}

func newScrollablePage(page *rod.Page) *scrollablePage {
	result := &scrollablePage{
		Page:             page,
		LastEventTime:    time.Now(),
		OngoingRequests:  0,
		FinishedRequests: 0,
		Mutex:            sync.Mutex{},
	}

	go page.EachEvent(
		func(e *proto.NetworkRequestWillBeSent) {
			result.Mutex.Lock()
			result.LastEventTime = time.Now()
			result.OngoingRequests++
			result.Mutex.Unlock()
		}, func(e *proto.NetworkLoadingFinished) {
			result.Mutex.Lock()
			result.LastEventTime = time.Now()
			result.OngoingRequests--
			result.FinishedRequests++
			result.Mutex.Unlock()
		}, func(e *proto.NetworkLoadingFailed) {
			result.Mutex.Lock()
			result.LastEventTime = time.Now()
			result.OngoingRequests--
			result.Mutex.Unlock()
		},
	)()

	return result
}

func (p *scrollablePage) waitAndScroll(
	logger Logger, maybeFindMoreButton *rod.Element, maxScrollTime time.Duration,
) error {
	startTime := time.Now()

	if maybeFindMoreButton != nil {
		err := maybeFindMoreButton.Click(proto.InputMouseButtonLeft, 1)
		if err != nil {
			logger.Info("Error while clicking: %v", err)
			content, contentErr := p.Page.HTML()
			if contentErr != nil {
				logger.Info("Page content:")
				logger.Info(content)
			} else {
				logger.Info("Error while getting page content: %v", contentErr)
			}
			return oops.Wrap(err)
		}
	}

	for {
		now := time.Now()
		if now.Sub(startTime) >= maxScrollTime {
			logger.Warn("Stopping the scroll early after %v", maxScrollTime)
			break
		}

		p.Mutex.Lock()
		lastEventTime := p.LastEventTime
		finishedRequests := p.FinishedRequests
		ongoingRequests := p.OngoingRequests
		p.Mutex.Unlock()

		underSecondPassed := now.Sub(startTime) < time.Second
		requestRecentlyInFlight := ongoingRequests > 0 || now.Sub(lastEventTime) < time.Second
		requestStuckInFlight := now.Sub(lastEventTime) >= 10*time.Second
		if !(underSecondPassed || (requestRecentlyInFlight && !requestStuckInFlight)) {
			break
		}

		logger.Info(
			"Wait and scroll - finished: %d ongoing: %d time: %v",
			finishedRequests, ongoingRequests, now.Sub(startTime),
		)
		var evalOptions rod.EvalOptions
		evalOptions.JS = "() => window.scrollBy(0, document.body.scrollHeight)"
		_, err := p.Page.Timeout(3 * time.Second).Evaluate(&evalOptions)
		if err != nil {
			return oops.Wrap(err)
		}
		time.Sleep(time.Second)
	}

	return nil
}
