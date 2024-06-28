package crawler

import (
	"feedrewind/config"
	"feedrewind/oops"
	"net/url"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

type PuppeteerFindLoadMoreButton func(*rod.Page) (*rod.Element, error)

type PuppeteerClient interface {
	Fetch(
		uri *url.URL, feedEntryCurisTitlesMap CanonicalUriMap[*LinkTitle], crawlCtx *CrawlContext,
		logger Logger, findLoadMoreButton PuppeteerFindLoadMoreButton, extendedScrollTime bool,
	) (string, error)
}

type PuppeteerClientImpl struct {
	Launcher *launcher.Launcher
}

func NewPuppeteerClientImpl() *PuppeteerClientImpl {
	launcher := launcher.New()
	if config.Cfg.IsHeroku {
		launcher = launcher.Bin("chrome")
	}
	return &PuppeteerClientImpl{
		Launcher: launcher,
	}
}

const defaultMaxScrollTime = 30 * time.Second
const extendedMaxScrollTime = 2 * time.Minute

func (c *PuppeteerClientImpl) Fetch(
	uri *url.URL, feedEntryCurisTitlesMap CanonicalUriMap[*LinkTitle], crawlCtx *CrawlContext,
	logger Logger, findLoadMoreButton PuppeteerFindLoadMoreButton, extendedScrollTime bool,
) (string, error) {
	progressLogger := crawlCtx.ProgressLogger
	logger.Info("Puppeteer start: %s", uri)
	progressLogger.LogAndSavePuppeteerStart()
	isInitialRequest := true
	puppeteerStart := time.Now()

	browserUrl, err := c.Launcher.Launch()
	if err != nil {
		return "", oops.Wrap(err)
	}
	browser := rod.New().ControlURL(browserUrl)
	err = browser.Connect()
	if err != nil {
		return "", oops.Wrap(err)
	}
	defer func() {
		if err := browser.Close(); err != nil {
			logger.Error("Browser close error: %v", err)
		}
	}()
	maxScrollTime := defaultMaxScrollTime
	if extendedScrollTime {
		maxScrollTime = extendedMaxScrollTime
	}

	errorsCount := 0
	for {
		content, err := func() (string, error) {
			page, err := browser.Page(proto.TargetCreateTarget{}) //nolint:exhaustruct
			if err != nil {
				return "", oops.Wrap(err)
			}
			page = page.Timeout(maxScrollTime + 1*time.Minute)

			hijackRouter := page.HijackRequests()
			err = hijackRouter.Add("*", proto.NetworkResourceTypeImage, func(h *rod.Hijack) {
				h.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			})
			if err != nil {
				return "", oops.Wrap(err)
			}
			err = hijackRouter.Add("*", proto.NetworkResourceTypeFont, func(h *rod.Hijack) {
				h.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			})
			if err != nil {
				return "", oops.Wrap(err)
			}
			go hijackRouter.Run()
			defer func() {
				if err := hijackRouter.Stop(); err != nil {
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
				page.WaitRequestIdle(500*time.Millisecond, []string{".+"}, nil, nil)()
			}
			progressLogger.LogAndSavePuppeteer()
			if err != nil {
				return "", oops.Wrap(err)
			}

			initialContent, err := page.HTML()
			if err != nil {
				return "", oops.Wrap(err)
			}
			initialDocument, err := parseHtml(initialContent, logger)
			if err != nil {
				return "", oops.Wrap(err)
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
							return "", err
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
						return "", err
					}
				}
				content, err = page.HTML()
				if err != nil {
					return "", err
				}
			} else {
				logger.Info("Puppeteer didn't find any feed links on initial load")
				content = initialContent
			}

			scrollablePage.Mutex.Lock()
			finishedRequests := scrollablePage.FinishedRequests
			scrollablePage.Mutex.Unlock()
			crawlCtx.PuppeteerRequestsMade += finishedRequests
			logger.Info("Puppeteer done (%v, %d req)", time.Since(puppeteerStart), finishedRequests)

			return content, nil
		}()
		if err != nil {
			errorsCount++
			logger.Info("Recovered Puppeteer error (%d): %v", errorsCount, err)
			progressLogger.LogAndSavePuppeteer()
			if errorsCount >= 3 {
				return "", oops.Wrapf(err, "Puppeteer error")
			}
			continue
		}

		return content, nil
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
		_, err := p.Page.Evaluate(&evalOptions)
		if err != nil {
			return oops.Wrap(err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}
