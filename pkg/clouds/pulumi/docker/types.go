package docker

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/docker/docker/api/types/registry"
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/util"
)

type ResponseAux struct {
	ID     string `json:"ID"`
	Tag    string `json:"Tag"`
	Digest string `json:"Digest"`
	Size   int    `json:"Size"`
}

// ResponseMessage reflects typical response message from Docker daemon of V1
type ResponseMessage struct {
	Id          string      `json:"id"`
	Status      string      `json:"status"`
	Stream      string      `json:"stream"`
	Aux         ResponseAux `json:"aux"`
	ErrorDetail struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	ProgressDetail struct {
		Current int `json:"current"`
		Total   int `json:"total"`
	}
	Progress string `json:"progress"`
	Error    string `json:"error"`

	summary string `json:"-"`
}

func streamMessagesToChannel(reader *bufio.Reader, msgChan chan readerNextMessage) error {
	scanner := util.NewLineOrReturnScanner(reader)
	for {
		if !scanner.Scan() {
			msgChan <- readerNextMessage{EOF: true}
			return nil
		}
		line := string(scanner.Bytes())
		err := scanner.Err()
		if err != nil {
			msgChan <- readerNextMessage{Error: err}
		}
		msg := ResponseMessage{}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			msgChan <- readerNextMessage{Error: err}
		} else {
			if msg.Error != "" {
				msgChan <- readerNextMessage{Error: errors.New(msg.Error)}
			} else {
				msgChan <- readerNextMessage{Message: msg}
			}
		}
	}
}

// ResponseMessageV2 reflects typical response message from Docker daemon of V2
type ResponseMessageV2 struct {
	Id  string `json:"id"`
	Aux string `json:"aux"` // contains base64-encoded PB object
}

func EncodeDockerAuthHeader(username, password string) (string, error) {
	encodedJSON, err := json.Marshal(registry.AuthConfig{
		Username: username,
		Password: password,
		Auth:     base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", username, password))),
	})
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(encodedJSON), nil
}
