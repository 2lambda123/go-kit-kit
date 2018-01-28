package twirp

import (
	"context"

	"github.com/go-kit/kit/endpoint"
)

// rpcFn is a simple type to represent an RPC method on the Twirp client.
type rpcFn func(context.Context, interface{}) (interface{}, error)

// Client wraps a Twirp client and provides a method that implements endpoint.Endpoint.
type Client struct {
	rpcFn  rpcFn
	enc    EncodeRequestFunc
	dec    DecodeResponseFunc
	before []ClientRequestFunc
	after  []ClientResponseFunc
}

// NewClient constructs a usable Client for a single remote method.
func NewClient(
	rpcFn rpcFn,
	enc EncodeRequestFunc,
	dec DecodeResponseFunc,
	options ...ClientOption,
) *Client {
	c := &Client{
		rpcFn:  rpcFn,
		enc:    enc,
		dec:    dec,
		before: []ClientRequestFunc{},
		after:  []ClientResponseFunc{},
	}
	for _, option := range options {
		option(c)
	}
	return c
}

// ClientOption sets an optional parameter for clients.
type ClientOption func(*Client)

// ClientBefore sets the ClientRequestFunc that are applied to the outgoing HTTP
// request before it's invoked.
func ClientBefore(before ...ClientRequestFunc) ClientOption {
	return func(c *Client) { c.before = append(c.before, before...) }
}

// ClientAfter sets the ClientResponseFuncs applied to the incoming HTTP
// request prior to it being decoded. This is useful for obtaining anything off
// of the response and adding onto the context prior to decoding.
func ClientAfter(after ...ClientResponseFunc) ClientOption {
	return func(c *Client) { c.after = append(c.after, after...) }
}

// Endpoint returns a usable endpoint that invokes the remote endpoint.
func (c Client) Endpoint() endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Encode
		var (
			req interface{}
			err error
		)
		req, err = c.enc(ctx, request)
		if err != nil {
			return nil, err
		}

		// Process ClientRequestFunctions
		for _, f := range c.before {
			ctx, err = f(ctx)
			if err != nil {
				return nil, err
			}
		}

		// Call the actual RPC method
		resp, err := c.rpcFn(ctx, req)
		if err != nil {
			return nil, err
		}

		// Process ClientResponseFunctions
		for _, f := range c.after {
			ctx, err = f(ctx)
			if err != nil {
				return nil, err
			}
		}

		// Decode
		response, err := c.dec(ctx, resp)
		if err != nil {
			return nil, err
		}

		return response, nil
	}
}
