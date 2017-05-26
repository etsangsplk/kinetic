package main

import (
	"encoding/json"
	"context"
	"log"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/rewardStyle/kinetic"
	"github.com/rewardStyle/kinetic/listener"
	"github.com/rewardStyle/kinetic/message"
	"github.com/rewardStyle/kinetic/producer"
)

func main() {

	// Set the RNG Seed based on current time (in order to randomize the RNG)
	rand.Seed(time.Now().UTC().UnixNano())

	// Instantiate StreamData Object to keep stats
	streamData := NewStreamData()

	// Instantiate a new kinentic object
	k, err := kinetic.New(func(c *kinetic.Config) {
		c.SetCredentials("some-access-key", "some-secret-key", "some-security-token")
		c.SetRegion("some-region")
		c.SetEndpoint("http://127.0.0.1:4567")
	})
	if err != nil {
		// TODO: process error
	}

	// Create a kinetic stream
	stream := "some-stream-" + strconv.Itoa(rand.Int())
	err = k.CreateStream(stream, 1)
	if err != nil {
		// TODO: process error
	}

	// Delete the kinetic stream if no dups were found (this is for debugging the kinetic stream)
	defer func(s *StreamData) {
		if !s.hasDuplicates() {
			k.DeleteStream(stream)
			k.WaitUntilStreamDeleted(context.TODO(), stream,
				request.WithWaiterDelay(request.ConstantWaiterDelay(1*time.Second)))
		}
	}(streamData)

	// Wait until the stream is ready to go
	err = k.WaitUntilStreamExists(context.TODO(), stream,
		request.WithWaiterDelay(request.ConstantWaiterDelay(1*time.Second)))
	if err != nil {
		// TODO: process error
	}

	// Determine the shard name
	shards, err := k.GetShards(stream)
	if err != nil {
		// TODO: process error
	}

	log.Printf("Stream Name: %s\n", stream)
	log.Printf("Shard Name: %s\n", shards[0])

	// Create a new kinetic producer
	p, err := producer.NewProducer(func(c *producer.Config) {
		c.SetAwsConfig(k.Session.Config)
		c.SetKinesisStream(stream)
		c.SetBatchSize(5)
		c.SetBatchTimeout(1000 * time.Millisecond)
	})
	if err != nil {
		// TODO: process error
	}

	// Create a new kinetic listener
	l, err := listener.NewListener(func(c *listener.Config) {
		c.SetAwsConfig(k.Session.Config)
		c.SetReader(listener.NewKinesisReader(stream, shards[0]))
		c.SetQueueDepth(20)
		c.SetConcurrency(10)
		c.SetGetRecordsReadTimeout(1000 * time.Millisecond)
		//c.SetLogLevel(aws.LogDebug)
	})
	if err != nil {
		// TODO: process error
	}

	numMsg := 1000000
	numSent := 0

	// Use the producer to write messages to the kinetic stream
	wg := sync.WaitGroup{}
	wg.Add(numMsg + 1)
	go func(sent *int) {
		for i := 0; i < numMsg; i++ {
			jsonStr, _ := json.Marshal(NewMessage())
			if err := p.Send(&message.Message {
				PartitionKey: aws.String("key"),
				Data: []byte(jsonStr),
			}); err == nil {
				*sent++
			}
		}
		wg.Done()
	}(&numSent)

	// Use the listener to read messages from the kinetic stream
	go func() {
		l.Listen(func(b []byte, fnwg *sync.WaitGroup) {
			// Unmarshal data
			msg := &Message{}
			json.Unmarshal(b, msg)

			// Only mark "done" if the message isn't a duplicate
			if !streamData.exists(msg.ID) {
				wg.Done()
			} else {
				log.Printf("WARNING: Duplicate message: %v\n", msg)
			}

			// Record message regardless if it is a duplicate
			streamData.put(msg.ID, msg.Message)
			fnwg.Done()
		})
	}()

	wg.Wait()

	streamData.printSummary()
}
