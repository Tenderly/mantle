package client

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/go-resty/resty/v2"
	"github.com/tenderly/mantle/l2geth/rollup"
)

var errDtlHTTPError = errors.New("dtl http error")

type DtlClient interface {
	GetEnqueueByIndex(index uint64) (string, error)
}

type Client struct {
	client *resty.Client
}

func NewDtlClient(url string) *Client {
	client := resty.New()
	client.SetHostURL(url)
	client.OnAfterResponse(func(c *resty.Client, r *resty.Response) error {
		statusCode := r.StatusCode()
		if statusCode >= 400 {
			method := r.Request.Method
			url := r.Request.URL
			return fmt.Errorf("%d cannot %s %s: %w", statusCode, method, url, errDtlHTTPError)
		}
		return nil
	})
	return &Client{
		client: client,
	}
}

func (c *Client) GetEnqueueByIndex(index uint64) (string, error) {
	str := strconv.FormatUint(index, 10)
	response, err := c.client.R().
		SetPathParams(map[string]string{
			"index": str,
		}).
		SetResult(&rollup.Enqueue{}).
		Get("/enqueue/index/{index}")
	if err != nil {
		return "", fmt.Errorf("cannot fetch enqueue: %w", err)
	}
	enqueue, ok := response.Result().(*rollup.Enqueue)
	if !ok {
		return "", fmt.Errorf("cannot fetch enqueue %d", index)
	}
	if enqueue == nil || enqueue.Origin == nil {
		return "", fmt.Errorf("cannot deserialize enqueue %d", index)
	}
	return enqueue.Origin.String(), nil
}
