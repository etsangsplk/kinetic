package kinetic

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	//"github.com/aws/aws-sdk-go/service/firehose"
	"github.com/aws/aws-sdk-go/service/firehose/firehoseiface"
	"github.com/aws/aws-sdk-go/service/kinesis"
	"github.com/aws/aws-sdk-go/service/kinesis/kinesisiface"

	"github.com/rewardStyle/kinetic/logging"
)

var (
	// GetShards errors
	ErrNilDescribeStreamResponse = errors.New("DescribeStream returned a nil response")
	ErrNilStreamDescription      = errors.New("DescribeStream returned a nil StreamDescription")
)

type kineticConfig struct {
	logLevel aws.LogLevelType
}

type Kinetic struct {
	*kineticConfig
	session *session.Session

	fclient  firehoseiface.FirehoseAPI
	kclient  kinesisiface.KinesisAPI
	clientMu sync.Mutex
}

func New(config *Config) (*Kinetic, error) {
	session, err := config.GetSession()
	if err != nil {
		return nil, err
	}
	return &Kinetic{
		kineticConfig: config.kineticConfig,
		session:       session,
	}, nil
}

func (k *Kinetic) Log(args ...interface{}) {
	if k.logLevel.Matches(logging.LogDebug) {
		k.session.Config.Logger.Log(args...)
	}
}

func (k *Kinetic) ensureKinesisClient() {
	k.clientMu.Lock()
	defer k.clientMu.Unlock()
	if k.kclient == nil {
		k.kclient = kinesis.New(k.session)
	}
}

func (k *Kinetic) CreateStream(stream string, shards int) error {
	k.ensureKinesisClient()
	_, err := k.kclient.CreateStream(&kinesis.CreateStreamInput{
		StreamName: aws.String(stream),
		ShardCount: aws.Int64(int64(shards)),
	})
	if err != nil {
		k.Log("Error creating kinesis stream:", err)
	}
	return err
}

func (k *Kinetic) WaitUntilStreamExists(ctx context.Context, stream string, opts ...request.WaiterOption) error {
	k.ensureKinesisClient()
	return k.kclient.WaitUntilStreamExistsWithContext(ctx, &kinesis.DescribeStreamInput{
		StreamName: aws.String(stream), // Required
	}, opts...)
}

func (k *Kinetic) DeleteStream(stream string) error {
	k.ensureKinesisClient()
	_, err := k.kclient.DeleteStream(&kinesis.DeleteStreamInput{
		StreamName: aws.String(stream),
	})
	if err != nil {
		k.Log("Error deleting kinesis stream:", err)
	}
	return err
}

func (k *Kinetic) WaitUntilStreamDeleted(ctx context.Context, stream string, opts ...request.WaiterOption) error {
	k.ensureKinesisClient()
	w := request.Waiter{
		Name:        "WaitUntilStreamIsDeleted",
		MaxAttempts: 18,
		Delay:       request.ConstantWaiterDelay(10 * time.Second),
		Acceptors: []request.WaiterAcceptor{
			{
				State:    request.SuccessWaiterState,
				Matcher:  request.ErrorWaiterMatch,
				Expected: kinesis.ErrCodeResourceNotFoundException,
			},
		},
		Logger: k.session.Config.Logger,
		NewRequest: func(opts []request.Option) (*request.Request, error) {
			req, _ := k.kclient.DescribeStreamRequest(&kinesis.DescribeStreamInput{
				StreamName: aws.String(stream), // Required
			})
			req.SetContext(ctx)
			req.ApplyOptions(opts...)
			return req, nil
		},
	}
	w.ApplyOptions(opts...)
	return w.WaitWithContext(ctx)
}

func (k *Kinetic) GetShards(stream string) ([]string, error) {
	k.ensureKinesisClient()
	resp, err := k.kclient.DescribeStream(&kinesis.DescribeStreamInput{
		StreamName: aws.String(stream),
	})
	if err != nil {
		k.Log("Error describing kinesis stream:", err)
		return nil, err
	}
	if resp == nil {
		return nil, ErrNilDescribeStreamResponse
	}
	if resp.StreamDescription == nil {
		return nil, ErrNilStreamDescription
	}
	var shards []string
	for _, shard := range resp.StreamDescription.Shards {
		if shard.ShardId != nil {
			shards = append(shards, aws.StringValue(shard.ShardId))
		}
	}
	return shards, nil
}

func (k *Kinetic) GetSession() *session.Session {
	return k.session
}

// func (k *Kinetic) NewListener(config *listener.Config) (*listener.Listener, error) {
// 	return listener.NewListener(config, k, k.session, k.kclient)
// }