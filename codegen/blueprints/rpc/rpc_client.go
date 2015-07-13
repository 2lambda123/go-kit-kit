package rpc

import (
	"net/rpc"

	"golang.org/x/net/context"

	"github.com/go-kit/kit/endpoint"
)

// NewRPCClient takes a net/rpc Client that should point to an instance of an
// addsvc. It returns an endpoint that wraps and invokes that Client.
func NewRPCClient(c *rpc.Client) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		var (
			errs      = make(chan error, 1)
			responses = make(chan interface{}, 1)
		)
		go func() {
			var response ResponseT
			if err := c.Call("addsvc.Add", request, &response); err != nil {
				errs <- err
				return
			}
			responses <- response
		}()
		select {
		case <-ctx.Done():
			return nil, context.DeadlineExceeded
		case err := <-errs:
			return nil, err
		case response := <-responses:
			return response, nil
		}
	}
}
