package clients

import (
	"context"
	"fmt"
	"strconv"
	"time"

	awsSqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/francescopepe/go-queue-worker/internal/client"
	"github.com/francescopepe/go-queue-worker/internal/messages"
)

type SqsClientConfiguration struct {
	// The AWS Sqs Service Client
	Svc *awsSqs.Client

	// The AWS ReceiveMessageInput
	ReceiveMessageInput *awsSqs.ReceiveMessageInput

	// Defines the interval within which the message must be processed.
	// If empty, it tries to set the value from the ReceiveMessageInput's
	// VisibilityTimeout.
	// If ReceiveMessageInput's VisibilityTimeout is empty it retrieves the
	// default value set on the queue. This action will fail if the client
	// does not have the permission to retrieve the SQS queue's attributes.
	MessageCtxTimeout time.Duration
}

type sqsClient struct {
	svc                 *awsSqs.Client
	receiveMessageInput *awsSqs.ReceiveMessageInput
	messageCtxTimeout   time.Duration
}

func (c sqsClient) ReceiveMessages() ([]messages.Message, error) {
	out, err := c.svc.ReceiveMessage(context.Background(), c.receiveMessageInput)
	if err != nil {
		return nil, fmt.Errorf("unable to receive messages: %w", err)
	}

	msgs := make([]messages.Message, len(out.Messages))
	for i, msg := range out.Messages {
		msgs[i] = c.createMessage(msg)
	}

	return msgs, nil
}

func (c sqsClient) DeleteMessages(messages []messages.Message) error {
	deleteEntries := make([]types.DeleteMessageBatchRequestEntry, 0, len(messages))
	for _, message := range messages {
		deleteEntries = append(deleteEntries, types.DeleteMessageBatchRequestEntry{
			Id:            message.Msg.(types.Message).MessageId,
			ReceiptHandle: message.Msg.(types.Message).ReceiptHandle,
		})
	}

	_, err := c.svc.DeleteMessageBatch(context.Background(), &awsSqs.DeleteMessageBatchInput{
		Entries:  deleteEntries,
		QueueUrl: c.receiveMessageInput.QueueUrl,
	})

	return err
}

func (c sqsClient) createMessage(sqsMessage types.Message) messages.Message {
	msg := messages.Message{
		Msg: sqsMessage,
	}

	// Set a context with timeout
	msg.Ctx, msg.CancelCtx = context.WithTimeout(context.Background(), c.messageCtxTimeout)

	return msg
}

func NewSqsClient(ctx context.Context, config SqsClientConfiguration) (sqsClient, error) {
	messageTimeout := config.MessageCtxTimeout
	if messageTimeout == 0 {
		var err error
		messageTimeout, err = retrieveVisibilityTimeout(ctx, config.Svc, config.ReceiveMessageInput.QueueUrl)
		if err != nil {
			return sqsClient{}, fmt.Errorf("unable to retrieve visibility timeout: %w", err)
		}
	}

	return sqsClient{
		svc:                 config.Svc,
		receiveMessageInput: config.ReceiveMessageInput,
		messageCtxTimeout:   messageTimeout,
	}, nil
}

func retrieveVisibilityTimeout(ctx context.Context, svc *awsSqs.Client, queue *string) (time.Duration, error) {
	out, err := svc.GetQueueAttributes(ctx, &awsSqs.GetQueueAttributesInput{
		QueueUrl:       queue,
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameVisibilityTimeout},
	})
	if err != nil {
		return 0, fmt.Errorf("unable to get attributes: %w", err)
	}

	timeout, err := strconv.Atoi(out.Attributes[string(types.QueueAttributeNameVisibilityTimeout)])
	if err != nil {
		return 0, fmt.Errorf("unable to parse timeout: %w", err)
	}

	return time.Second * time.Duration(timeout), nil
}

// Interface guards
var (
	_ client.Client = (*sqsClient)(nil)
)
