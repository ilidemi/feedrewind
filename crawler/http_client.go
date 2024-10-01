package crawler

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type HttpResponse struct {
	Code             string
	MaybeContentType *string
	MaybeLocation    *string
	Body             []byte
}

func newHttpResponse(code string) *HttpResponse {
	return &HttpResponse{
		Code:             code,
		MaybeContentType: nil,
		MaybeLocation:    nil,
		Body:             nil,
	}
}

type HttpClient interface {
	Request(
		uri *url.URL, shouldThrottle bool, maybeRobotsClient *RobotsClient, logger Logger,
	) (*HttpResponse, error)
	GetRetryDelay(attemptsMade int) float64
}

type CancellationFunc func() error

type HttpClientImpl struct {
	CancellationFunc CancellationFunc
	EnableThrottling bool
	PrevTimestamp    time.Time
	Client           *http.Client
}

func NewHttpClientImplFunc(cancellationFunc CancellationFunc, enableThrottling bool) *HttpClientImpl {
	var client http.Client
	client.Timeout = time.Minute
	return &HttpClientImpl{
		CancellationFunc: cancellationFunc,
		EnableThrottling: enableThrottling,
		PrevTimestamp:    time.Time{},
		Client:           &client,
	}
}

func NewHttpClientImplCtx(ctx context.Context, enableThrottling bool) *HttpClientImpl {
	var client http.Client
	client.Timeout = time.Minute
	return &HttpClientImpl{
		CancellationFunc: func() error {
			return ctx.Err()
		},
		EnableThrottling: enableThrottling,
		PrevTimestamp:    time.Time{},
		Client:           &client,
	}
}

const maxContentLength = 20 * 1024 * 1024

const codeSSLError = "SSLError"
const codeResponseBodyTooBig = "ResponseBodyTooBig"

func (c *HttpClientImpl) Request(
	uri *url.URL, shouldThrottle bool, maybeRobotsClient *RobotsClient, logger Logger,
) (*HttpResponse, error) {
	if err := c.CancellationFunc(); err != nil {
		return nil, err
	}

	if c.EnableThrottling && shouldThrottle && maybeRobotsClient != nil {
		maybeRobotsClient.Throttle()
	}

	req, err := http.NewRequest(http.MethodGet, uri.String(), nil)
	if err != nil {
		logger.Info("HTTP new request error: %v", err)
		return newHttpResponse("Error"), nil
	}
	req.Header.Add("User-Agent", "Mozilla/5.0 (compatible; FeedRewindBot/1.0; +https://feedrewind.com/bot)")
	resp, err := c.Client.Do(req)
	var hostnameError x509.HostnameError
	var unknownAuthorityError x509.UnknownAuthorityError
	if errors.As(err, &hostnameError) || errors.As(err, &unknownAuthorityError) {
		return newHttpResponse(codeSSLError), nil
	} else if os.IsTimeout(err) {
		return newHttpResponse("Timeout"), nil
	} else if err != nil {
		logger.Info("HTTP request error: %v", err)
		return newHttpResponse("Error"), nil
	}
	defer resp.Body.Close()

	if resp.ContentLength > maxContentLength {
		return newHttpResponse(codeResponseBodyTooBig), nil
	}

	var body []byte
	var buf [1024 * 1024]byte
	for {
		n, err := resp.Body.Read(buf[:])
		if n > 0 {
			body = append(body, buf[:n]...)
			if len(body) > maxContentLength {
				return newHttpResponse(codeResponseBodyTooBig), nil
			}
		}
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			logger.Info("HTTP read body error: %v", err)
			return newHttpResponse("Error"), nil
		}
	}

	var maybeContentType *string
	if contentTypes, ok := resp.Header["Content-Type"]; ok {
		contentType := strings.Clone(contentTypes[0])
		maybeContentType = &contentType
	}

	var maybeLocation *string
	if locations, ok := resp.Header["Location"]; ok {
		location := strings.Clone(locations[0])
		maybeLocation = &location
	}

	return &HttpResponse{
		Code:             fmt.Sprint(resp.StatusCode),
		MaybeContentType: maybeContentType,
		MaybeLocation:    maybeLocation,
		Body:             body,
	}, nil

}

func (c *HttpClientImpl) GetRetryDelay(attemptsMade int) float64 {
	switch attemptsMade {
	case 0:
		return 1
	case 1:
		return 5
	default:
		return 15
	}
}
