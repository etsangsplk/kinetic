package listener

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"sync"

	"github.com/rewardStyle/kinetic/errs"
	"github.com/rewardStyle/kinetic/message"
	"github.com/rewardStyle/kinetic/multilang"
)

type kclReaderOptions struct {
	onInitCallbackFn       func() error
	onCheckpointCallbackFn func() error
	onShutdownCallbackFn   func() error
}

// KclReader handles the KCL Multilang Protocol to read records from KCL
type KclReader struct {
	*kclReaderOptions
	throttleSem chan Empty
	listener    *Listener
	scanner     *bufio.Scanner
	mutex       *sync.Mutex
	msgBuffer   []message.Message
	ackPending  bool
}

// NewKclReader creates a new stream reader to read records from KCL
func NewKclReader(fn ...func(*KclReaderConfig)) *KclReader {
	config := NewKclReaderConfig()
	for _, f := range fn {
		f(config)
	}
	return &KclReader{
		kclReaderOptions: config.kclReaderOptions,
		throttleSem: make(chan Empty, 5),
		msgBuffer: []message.Message{},
		mutex: &sync.Mutex{},
	}
}

// AssociateListener associates the KCL stream reader to a listener
func (r *KclReader) AssociateListener(l *Listener) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.listener != nil {
		return errs.ErrListenerAlreadyAssociated
	}
	r.listener = l
	return nil
}

func (r *KclReader) ensureClient() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.scanner == nil {
		if r.listener == nil {
			return errs.ErrNilListener
		}
		r.scanner = bufio.NewScanner(os.Stdin)
		bufio.NewReader(os.Stdin)
	}
	return nil
}

// GetRecord calls processRecords to attempt to put one message from message buffer to the listener's message
// channel
func (r *KclReader) GetRecord() (int, error) {
	return r.processRecords(1)
}

// GetRecords calls processRecords to attempt to put all messages on the message buffer on the listener's
// message channel
func (r *KclReader) GetRecords() (int, error) {
	return r.processRecords(-1)
}

// processRecords is a helper method which loops through the message buffer and puts messages on the listener's
// message channel.  After all the messages on the message buffer have been moved to the listener's message
// channel, a message is sent (following the Multilang protocol) to acknowledge that the processRecords message
// has been received / processed
func (r *KclReader) processRecords(numRecords int) (int, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Define the batchSize
	batchSize := 0;
	if len(r.msgBuffer) > 0 {
		if numRecords < 0 {
			batchSize = len(r.msgBuffer)
		} else {
			batchSize = int(math.Min(float64(len(r.msgBuffer)), float64(numRecords)))
		}
	}

	// Loop through the message buffer and put the correct number of messages on the listener's message channel
	for i := 0; i < batchSize; i++ {
		r.listener.messages <- &r.msgBuffer[0]
		r.msgBuffer = r.msgBuffer[1:]
	}

	// Send an acknowledgement that the 'ProcessRecords' message was received/processed
	if len(r.msgBuffer) == 0 && r.ackPending {
		err := r.sendMessage(multilang.NewStatusMessage(multilang.ProcessRecords))
		if err != nil {
			// TODO:  What to do if the ack status message fails?
		}
	}

	return batchSize, nil
}

func (r *KclReader) processAction() error {
	if err := r.ensureClient(); err != nil {
		return err
	}

	actionMessage := &multilang.ActionMessage{}
	for r.scanner.Scan() {
		err := json.Unmarshal(r.scanner.Bytes(), actionMessage)
		if err != nil {
			return err
		}

		switch actionMessage.Action {
		case multilang.Initialize:
			r.onInit()
			r.sendMessage(multilang.NewStatusMessage(multilang.Initialize))
		case multilang.Checkpoint:
			r.onCheckpoint()
			r.sendMessage(multilang.NewStatusMessage(multilang.Checkpoint))
		case multilang.Shutdown:
			r.onShutdown()
			r.sendMessage(multilang.NewStatusMessage(multilang.Shutdown))
		case multilang.ProcessRecords:
			go func(){
				r.mutex.Lock()
				defer r.mutex.Unlock()

				if r.ackPending {
					// TODO: error out
					// This is an error according to the Multilang protocol
				}

				r.msgBuffer = append(r.msgBuffer, actionMessage.Records...)
				r.ackPending = true;
			}()
		default:
		}
	}

	return nil
}

func (r *KclReader) sendMessage(msg *multilang.ActionMessage) error {
	b, err := json.Marshal(msg)
	if err != nil {
		// TODO:
	}
	_, err = os.Stdout.Write(b)
	if err != nil {
		// TODO:
	}

	return nil
}

func (r *KclReader) onInit() error {
	if r.onInitCallbackFn != nil {
		err := r.onInitCallbackFn()
		if err != nil {
			// TODO:
		}
	}
	return nil
}

func (r *KclReader) onCheckpoint() error {
	if r.onCheckpointCallbackFn != nil {
		err := r.onCheckpointCallbackFn()
		if err != nil {
			// TODO:
		}
	}
	return nil
}

func (r *KclReader) onShutdown() error {
	if r.onShutdownCallbackFn != nil {
		err := r.onShutdownCallbackFn()
		if err != nil {
			// TODO:
		}
	}
	return nil
}
