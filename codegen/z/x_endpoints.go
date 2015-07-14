// Do not edit! Generated by gokit-generate

package z

import (
	"fmt"

	"github.com/go-kit/kit/endpoint"
	"golang.org/x/net/context"
)

type XEndpoints struct {
	x X
}

func MakeXEndpoints(x X) XEndpoints {
	return XEndpoints{x}
}

type XClient struct {
	f func(string) endpoint.Endpoint
}

func MakeXClient(f func(string) endpoint.Endpoint) XClient {
	return XClient{f}
}

func (x XEndpoints) Y(ctx context.Context, request interface{}) (interface{}, error) {
	select {
	default:
	case <-ctx.Done():
		return nil, endpoint.ErrContextCanceled
	}
	req, ok := request.(XYRequest)
	if !ok {
		return nil, endpoint.ErrBadCast
	}
	var err error
	var resp XYResponse
	resp.Int64 = x.x.Y(ctx, req.P1, req.Int, req.Int1, req.Int64)
	return resp, err
}

func (x XClient) Y(ctx context.Context, p1 struct {
	F fmt.Stringer
	G uint32
}, int1 int, int11 int, int641 int64) (int642 int64) {

	var err error
	var req XYRequest
	ctx, req.P1, req.Int, req.Int1, req.Int64 = ctx, p1, int1, int11, int641
	var raw interface{}
	raw, err = x.f("Y")(ctx, req)
	if err != nil {
		panic(err)
	}
	resp, ok := raw.(XYResponse)
	if !ok {
		err = endpoint.ErrBadCast
		panic(err)
	}

	int642 = resp.Int64
	return

}

func (x XEndpoints) Z(ctx context.Context, request interface{}) (interface{}, error) {
	select {
	default:
	case <-ctx.Done():
		return nil, endpoint.ErrContextCanceled
	}
	req, ok := request.(XZRequest)
	if !ok {
		return nil, endpoint.ErrBadCast
	}
	var err error
	var resp XZResponse
	resp.R, err = x.x.Z(ctx, req.A, req.B)
	return resp, err
}

func (x XClient) Z(ctx context.Context, a int, b int) (r int, err error) {

	var req XZRequest
	ctx, req.A, req.B = ctx, a, b
	var raw interface{}
	raw, err = x.f("Z")(ctx, req)
	if err != nil {
		return
	}
	resp, ok := raw.(XZResponse)
	if !ok {
		err = endpoint.ErrBadCast
		return
	}

	r, err = resp.R, err
	return

}