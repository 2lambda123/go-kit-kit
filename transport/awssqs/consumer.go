package awssqs

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/transport"
)

// Consumer wraps an endpoint and provides a handler for sqs messages.
type Consumer struct {
	sqsClient             Client
	e                     endpoint.Endpoint
	dec                   DecodeRequestFunc
	enc                   EncodeResponseFunc
	wantRep               WantReplyFunc
	queueURL              string
	visibilityTimeout     int64
	visibilityTimeoutFunc VisibilityTimeoutFunc
	before                []ConsumerRequestFunc
	after                 []ConsumerResponseFunc
	errorEncoder          ErrorEncoder
	finalizer             []ConsumerFinalizerFunc
	errorHandler          transport.ErrorHandler
}

// NewConsumer constructs a new Consumer, which provides a Consume method
// and message handlers that wrap the provided endpoint.
func NewConsumer(
	sqsClient Client,
	e endpoint.Endpoint,
	dec DecodeRequestFunc,
	enc EncodeResponseFunc,
	queueURL string,
	options ...ConsumerOption,
) *Consumer {
	s := &Consumer{
		sqsClient:             sqsClient,
		e:                     e,
		dec:                   dec,
		enc:                   enc,
		wantRep:               DoNotRespond,
		queueURL:              queueURL,
		visibilityTimeout:     int64(30),
		visibilityTimeoutFunc: DoNotExtendVisibilityTimeout,
		errorEncoder:          DefaultErrorEncoder,
		errorHandler:          transport.NewLogErrorHandler(log.NewNopLogger()),
	}
	for _, option := range options {
		option(s)
	}
	return s
}

// ConsumerOption sets an optional parameter for consumers.
type ConsumerOption func(*Consumer)

// ConsumerBefore functions are executed on the publisher request object before the
// request is decoded.
func ConsumerBefore(before ...ConsumerRequestFunc) ConsumerOption {
	return func(c *Consumer) { c.before = append(c.before, before...) }
}

// ConsumerAfter functions are executed on the consumer reply after the
// endpoint is invoked, but before anything is published to the reply.
func ConsumerAfter(after ...ConsumerResponseFunc) ConsumerOption {
	return func(c *Consumer) { c.after = append(c.after, after...) }
}

// ConsumerErrorEncoder is used to encode errors to the consumer reply
// whenever they're encountered in the processing of a request. Clients can
// use this to provide custom error formatting. By default,
// errors will be published with the DefaultErrorEncoder.
func ConsumerErrorEncoder(ee ErrorEncoder) ConsumerOption {
	return func(c *Consumer) { c.errorEncoder = ee }
}

// ConsumerVisbilityTimeOutFunc is used to extend the visibility timeout
// for messages while the consumer processes them.
// VisibilityTimeoutFunc will need to check that the provided context is not done.
// By default, visibility timeout are not extended.
func ConsumerVisbilityTimeOutFunc(vtFunc VisibilityTimeoutFunc) ConsumerOption {
	return func(c *Consumer) { c.visibilityTimeoutFunc = vtFunc }
}

// ConsumerVisibilityTimeout overrides the default value for the consumer's
// visibilityTimeout field.
func ConsumerVisibilityTimeout(visibilityTimeout int64) ConsumerOption {
	return func(c *Consumer) { c.visibilityTimeout = visibilityTimeout }
}

// ConsumerWantReplyFunc overrides the default value for the consumer's
// wantRep field.
func ConsumerWantReplyFunc(replyFunc WantReplyFunc) ConsumerOption {
	return func(c *Consumer) { c.wantRep = replyFunc }
}

// ConsumerErrorHandler is used to handle non-terminal errors. By default, non-terminal errors
// are ignored. This is intended as a diagnostic measure. Finer-grained control
// of error handling, including logging in more detail, should be performed in a
// custom ConsumerErrorEncoder which has access to the context.
func ConsumerErrorHandler(errorHandler transport.ErrorHandler) ConsumerOption {
	return func(c *Consumer) { c.errorHandler = errorHandler }
}

// ConsumerFinalizer is executed once all the received SQS messages are done being processed.
// By default, no finalizer is registered.
func ConsumerFinalizer(f ...ConsumerFinalizerFunc) ConsumerOption {
	return func(c *Consumer) { c.finalizer = f }
}

// Consume calls ReceiveMessageWithContext and handles messages having an
// sqs.ReceiveMessageInput as parameter allows each user to have his own receive configuration.
// That said, this method overrides the queueURL for the provided ReceiveMessageInput to ensure
// the messages are retrieved from the consumer's configured queue.
func (c Consumer) Consume(ctx context.Context, receiveMsgInput *sqs.ReceiveMessageInput) error {
	receiveMsgInput.QueueUrl = &c.queueURL
	out, err := c.sqsClient.ReceiveMessageWithContext(ctx, receiveMsgInput)
	if err != nil {
		return err
	}
	return c.HandleMessages(ctx, out.Messages)
}

// HandleMessages handles the consumed messages.
func (c Consumer) HandleMessages(ctx context.Context, msgs []*sqs.Message) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Copy received messages slice in leftMsgs slice
	// leftMsgs will be used by the consumer's visibilityTimeoutFunc to extend the
	// visibility timeout for the messages that have not been processed yet.
	leftMsgs := []*sqs.Message{}
	leftMsgs = append(leftMsgs, msgs...)

	visibilityTimeoutCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go c.visibilityTimeoutFunc(visibilityTimeoutCtx, c.sqsClient, c.queueURL, c.visibilityTimeout, &leftMsgs)

	if len(c.finalizer) > 0 {
		defer func() {
			for _, f := range c.finalizer {
				f(ctx, &msgs)
			}
		}()
	}

	for _, f := range c.before {
		ctx = f(ctx, &msgs)
	}

	for _, msg := range msgs {
		if err := c.HandleSingleMessage(ctx, msg, &leftMsgs); err != nil {
			return err
		}
	}
	return nil
}

// HandleSingleMessage handles a single sqs message.
func (c Consumer) HandleSingleMessage(ctx context.Context, msg *sqs.Message, leftMsgs *[]*sqs.Message) error {
	req, err := c.dec(ctx, msg)
	if err != nil {
		c.errorHandler.Handle(ctx, err)
		c.errorEncoder(ctx, err, msg, c.sqsClient)
		return err
	}

	response, err := c.e(ctx, req)
	if err != nil {
		c.errorHandler.Handle(ctx, err)
		c.errorEncoder(ctx, err, msg, c.sqsClient)
		return err
	}

	responseMsg := sqs.SendMessageInput{}
	for _, f := range c.after {
		ctx = f(ctx, msg, &responseMsg, leftMsgs)
	}

	if !c.wantRep(ctx, msg) {
		return nil
	}

	if err := c.enc(ctx, &responseMsg, response); err != nil {
		c.errorHandler.Handle(ctx, err)
		c.errorEncoder(ctx, err, msg, c.sqsClient)
		return err
	}

	if _, err := c.sqsClient.SendMessageWithContext(ctx, &responseMsg); err != nil {
		c.errorHandler.Handle(ctx, err)
		c.errorEncoder(ctx, err, msg, c.sqsClient)
		return err
	}
	return nil
}

// ErrorEncoder is responsible for encoding an error to the consumer's reply.
// Users are encouraged to use custom ErrorEncoders to encode errors to
// their replies, and will likely want to pass and check for their own error
// types.
type ErrorEncoder func(ctx context.Context, err error, req *sqs.Message, sqsClient Client)

// ConsumerFinalizerFunc can be used to perform work at the end of a request
// from a publisher, after the response has been written to the publisher. The
// principal intended use is for request logging.
// Can also be used to delete messages once fully proccessed.
type ConsumerFinalizerFunc func(ctx context.Context, msg *[]*sqs.Message)

// VisibilityTimeoutFunc encapsulates logic to extend messages visibility timeout.
// this can be used to provide custom visibility timeout extension such as doubling it everytime
// it gets close to being reached.
// VisibilityTimeoutFunc will need to check that the provided context is not done and return once it is.
type VisibilityTimeoutFunc func(context.Context, Client, string, int64, *[]*sqs.Message) error

// WantReplyFunc encapsulates logic to check whether message awaits response or not
// for example check for a given message attribute value.
type WantReplyFunc func(context.Context, *sqs.Message) bool

// DefaultErrorEncoder simply ignores the message. It does not reply.
func DefaultErrorEncoder(context.Context, error, *sqs.Message, Client) {
}

// DoNotExtendVisibilityTimeout is the default value for the consumer's visibilityTimeoutFunc.
// It returns no error and does nothing
func DoNotExtendVisibilityTimeout(context.Context, Client, string, int64, *[]*sqs.Message) error {
	return nil
}

// DoNotRespond is a WantReplyFunc and is the default value for consumer's wantRep field.
// It indicates that the message do not expect a response.
func DoNotRespond(context.Context, *sqs.Message) bool {
	return false
}

// EncodeJSONResponse marshals response as json and loads it into an sqs.SendMessageInput MessageBody.
func EncodeJSONResponse(_ context.Context, input *sqs.SendMessageInput, response interface{}) error {
	payload, err := json.Marshal(response)
	if err != nil {
		return err
	}
	input.MessageBody = aws.String(string(payload))
	return nil
}

// Client is consumer contract for the Producer and Consumer.
// It models methods of the AWS *sqs.SQS type.
type Client interface {
	SendMessageWithContext(ctx context.Context, input *sqs.SendMessageInput, opts ...request.Option) (*sqs.SendMessageOutput, error)
	ReceiveMessageWithContext(ctx context.Context, input *sqs.ReceiveMessageInput, opts ...request.Option) (*sqs.ReceiveMessageOutput, error)
	ChangeMessageVisibilityWithContext(ctx aws.Context, input *sqs.ChangeMessageVisibilityInput, opts ...request.Option) (*sqs.ChangeMessageVisibilityOutput, error)
}