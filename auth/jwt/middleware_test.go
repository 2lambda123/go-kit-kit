package jwt

import (
	"testing"

	jwt "github.com/dgrijalva/jwt-go"

	"golang.org/x/net/context"
)

var (
	key           = "test_signing_key"
	method        = jwt.SigningMethodHS256
	invalidMethod = jwt.SigningMethodRS256
	claims        = jwt.MapClaims{"user": "go-kit"}
	signedKey     = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyIjoiZ28ta2l0In0.MMefQU5pwDeoWBSdyagqNlr1tDGddGUOMGiIWmMlFvk"
	invalidKey    = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.e30.vKVCKto-Wn6rgz3vBdaZaCBGfCBDTXOENSo_X2Gq7qA"
)

func TestSigner(t *testing.T) {
	e := func(ctx context.Context, i interface{}) (interface{}, error) { return ctx, nil }

	signer := NewSigner(key, method, claims)(e)
	ctx := context.Background()
	ctx1, err := signer(ctx, struct{}{})
	if err != nil {
		t.Fatalf("Signer returned error: %s", err)
	}

	token, ok := ctx1.(context.Context).Value(JWTTokenContextKey).(string)
	if !ok {
		t.Fatal("Token did not exist in context")
	}

	if token != signedKey {
		t.Fatalf("JWT tokens did not match: expecting %s got %s", signedKey, token)
	}
}

func TestJWTParser(t *testing.T) {
	e := func(ctx context.Context, i interface{}) (interface{}, error) { return ctx, nil }

	keyfunc := func(token *jwt.Token) (interface{}, error) { return []byte(key), nil }
	badKeyfunc := func(token *jwt.Token) (interface{}, error) { return []byte("bad"), nil }

	parser := NewParser(keyfunc, method)(e)

	// No Token is passed into the parser
	_, err := parser(context.Background(), struct{}{})
	if err == nil {
		t.Error("Parser should have returned an error")
	}

	// Invalid Token is passed into the parser
	ctx := context.WithValue(context.Background(), JWTTokenContextKey, invalidKey)
	_, err = parser(ctx, struct{}{})
	if err == nil {
		t.Error("Parser should have returned an error")
	}

	// Invalid Method is used in the parser
	badParser := NewParser(keyfunc, invalidMethod)(e)
	ctx = context.WithValue(context.Background(), JWTTokenContextKey, signedKey)
	_, err = badParser(ctx, struct{}{})
	if err == nil {
		t.Error("Parser should have returned an error")
	}

	// Invalid key is used in the parser
	badParser = NewParser(badKeyfunc, method)(e)
	ctx = context.WithValue(context.Background(), JWTTokenContextKey, signedKey)
	_, err = badParser(ctx, struct{}{})
	if err == nil {
		t.Error("Parser should have returned an error")
	}

	// Correct token is passed into the parser
	ctx = context.WithValue(context.Background(), JWTTokenContextKey, signedKey)
	ctx1, err := parser(ctx, struct{}{})
	if err != nil {
		t.Fatalf("Parser returned error: %s", err)
	}

	cl, ok := ctx1.(context.Context).Value(JWTClaimsContextKey).(jwt.MapClaims)
	if !ok {
		t.Fatal("Claims were not passed into context correctly")
	}

	if cl["user"] != claims["user"] {
		t.Fatalf("JWT Claims.user did not match: expecting %s got %s", claims["user"], cl["user"])
	}
}