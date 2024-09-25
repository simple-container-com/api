package docker

import "fmt"

// MsgReader interface allowing to read streamed output from Docker daemon
type MsgReader interface {
	// Next synchronously reads next message from Docker daemon
	Next() (*ResponseMessage, error)

	// Listen allows to process messages streamed from Docker daemon
	Listen(output bool, callback MsgCallback) error
}

// MsgCallback callback to be called for each message from Docker daemon
type MsgCallback func(message *ResponseMessage, err error) error

type chanMsgReader struct {
	expectedEOFs int
	receivedEOFs int
	msgChan      chan readerNextMessage
}

type readerNextMessage struct {
	EOF     bool
	Message ResponseMessage
	Error   error
}

// Next blocks until next message appears in the response from Docker daemon
func (reader *chanMsgReader) Next() (*ResponseMessage, error) {
	next := <-reader.msgChan
	if next.EOF {
		reader.receivedEOFs++
		if reader.receivedEOFs >= reader.expectedEOFs {
			return nil, next.Error
		}
	}
	return &next.Message, next.Error
}

// Listen processes all messages streamed from Docker deamon
func (reader *chanMsgReader) Listen(output bool, callback MsgCallback) error {
	for {
		res, err := reader.Next()
		if res == nil {
			break
		}
		if err != nil {
			return err
		}

		if res.Status != "" {
			res.summary = fmt.Sprint(res.Id, ": ", res.Status, " ")
			if res.ProgressDetail.Total > 0 {
				res.summary += fmt.Sprintln(res.ProgressDetail.Current, " of ", res.ProgressDetail.Total)
			} else {
				res.summary += fmt.Sprintln()
			}
			if res.Progress != "" {
				res.summary += fmt.Sprintln(res.Progress)
			}
		} else if res.Stream != "" {
			res.summary = fmt.Sprint(res.Stream)
		} else if res.Aux.Digest != "" {
			res.summary = fmt.Sprint(res.Aux.Digest)
		}

		if callback != nil {
			if err := callback(res, err); err != nil {
				return err
			}
		}
		if output {
			fmt.Print(res.summary)
		}
	}
	return nil
}
