package jsonrpc_test

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-kit/kit/transport/http/jsonrpc"
)

type TestResponse struct {
	Body   io.ReadCloser
	String string
}

func TestCanCallBeforeFunc(t *testing.T) {
	called := false
	u, _ := url.Parse("http://senseye.io/jsonrpc")
	sut := jsonrpc.NewClient(
		u,
		"add",
		jsonrpc.ClientBefore(func(ctx context.Context, req *http.Request) context.Context {
			called = true
			return ctx
		}),
	)

	sut.Endpoint()(context.TODO(), "foo")

	if !called {
		t.Fatal("Expected client before func to be called. Wasn't.")
	}
}

func TestClientHappyPath(t *testing.T) {
	var (
		afterCalledKey    = "AC"
		beforeHeaderKey   = "BF"
		beforeHeaderValue = "beforeFuncWozEre"
		testbody          = `{"jsonrpc":"2.0", "result":5}`
		requestBody       []byte
		beforeFunc        = func(ctx context.Context, r *http.Request) context.Context {
			r.Header.Add(beforeHeaderKey, beforeHeaderValue)
			return ctx
		}
		encode = func(ctx context.Context, req interface{}) (json.RawMessage, error) {
			return json.Marshal(req)
		}
		afterFunc = func(ctx context.Context, r *http.Response) context.Context {
			return context.WithValue(ctx, afterCalledKey, true)
		}
		decode = func(ctx context.Context, res json.RawMessage) (interface{}, error) {
			if ac := ctx.Value(afterCalledKey); ac == nil {
				t.Fatal("after not called")
			}
			var result int
			err := json.Unmarshal(res, &result)
			if err != nil {
				return nil, err
			}
			return result, nil
		}
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(beforeHeaderKey) != beforeHeaderValue {
			t.Fatal("Header not set by before func.")
		}

		b, err := ioutil.ReadAll(r.Body)
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}
		requestBody = b

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testbody))
	}))

	sut := jsonrpc.NewClient(
		mustParse(server.URL),
		"add",
		jsonrpc.ClientRequestEncoder(encode),
		jsonrpc.ClientResponseDecoder(decode),
		jsonrpc.ClientBefore(beforeFunc),
		jsonrpc.ClientAfter(afterFunc),
	)

	type addRequest struct {
		A int
		B int
	}

	in := addRequest{2, 2}

	result, err := sut.Endpoint()(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	ri, ok := result.(int)
	if !ok {
		t.Fatalf("result is not int: (%T)%+v", result, result)
	}
	if ri != 5 {
		t.Fatalf("want=5, got=%d", ri)
	}

	var requestAtServer jsonrpc.Request
	err = json.Unmarshal(requestBody, &requestAtServer)
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := requestAtServer.ID.Int(); id != 0 {
		t.Fatalf("Request ID at server: want=0, got=%d", id)
	}

	var paramsAtServer addRequest
	err = json.Unmarshal(requestAtServer.Params, &paramsAtServer)
	if err != nil {
		t.Fatal(err)
	}

	if paramsAtServer != in {
		t.Fatalf("want=%+v, got=%+v", in, paramsAtServer)
	}
}

func TestCanUseDefaults(t *testing.T) {
	var (
		testbody    = `{"jsonrpc":"2.0", "result":"boogaloo"}`
		requestBody []byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadAll(r.Body)
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}
		requestBody = b

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testbody))
	}))

	sut := jsonrpc.NewClient(
		mustParse(server.URL),
		"add",
	)

	type addRequest struct {
		A int
		B int
	}

	in := addRequest{2, 2}

	result, err := sut.Endpoint()(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	rs, ok := result.(string)
	if !ok {
		t.Fatalf("result is not string: (%T)%+v", result, result)
	}
	if rs != "boogaloo" {
		t.Fatalf("want=boogaloo, got=%d", rs)
	}

	var requestAtServer jsonrpc.Request
	err = json.Unmarshal(requestBody, &requestAtServer)
	if err != nil {
		t.Fatal(err)
	}
	var paramsAtServer addRequest
	err = json.Unmarshal(requestAtServer.Params, &paramsAtServer)
	if err != nil {
		t.Fatal(err)
	}

	if paramsAtServer != in {
		t.Fatalf("want=%+v, got=%+v", in, paramsAtServer)
	}
}

func TestDefaultAutoIncrementer(t *testing.T) {
	sut := jsonrpc.NewAutoIncrementID(0)
	var want uint64
	for ; want < 100; want++ {
		got := sut.Generate()
		if got != want {
			t.Fatalf("want=%d, got=%d", want, got)
		}
	}
}

func mustParse(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
