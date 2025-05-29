package test

import (
	"github.com/stretchr/testify/mock"

	"github.com/simple-container-com/api/pkg/util"
)

type ConsoleMock struct {
	mock.Mock
}

type ConsoleWriterMock struct {
	mock.Mock
}

type ConsoleReaderMock struct {
	mock.Mock
}

func (c *ConsoleMock) AlwaysRespondDefault() *ConsoleMock {
	args := c.Called()
	return args.Get(0).(*ConsoleMock)
}

func (c *ConsoleMock) AskYesNoQuestionWithDefault(question string, yes bool) (bool, error) {
	args := c.Called(question, yes)
	res := args.Get(0)
	if args.Get(1) != nil {
		return res.(bool), args.Error(1)
	}
	return res.(bool), nil
}

func (c *ConsoleMock) AskQuestionWithDefault(question string, defaultResponse string) (string, error) {
	args := c.Called(question, defaultResponse)
	res := args.Get(0)
	if args.Get(1) != nil {
		return res.(string), args.Error(1)
	}
	return res.(string), nil
}

func (c *ConsoleMock) SetWriter(writer util.ConsoleWriter) {
	c.Called(writer)
}

func (c *ConsoleMock) SetReader(reader util.ConsoleReader) {
	c.Called(reader)
}

func (c *ConsoleMock) Writer() util.ConsoleWriter {
	args := c.Called()
	return args.Get(0).(util.ConsoleWriter)
}

func (c *ConsoleMock) Reader() util.ConsoleReader {
	args := c.Called()
	return args.Get(0).(util.ConsoleReader)
}

func (c *ConsoleMock) AskQuestion(question string) (string, error) {
	args := c.Called(question)
	res := args.Get(0)
	if args.Get(1) != nil {
		return res.(string), args.Error(1)
	}
	return res.(string), nil
}

func (c *ConsoleWriterMock) Print(args ...interface{}) {
	c.Called(args...)
}

func (c *ConsoleWriterMock) Println(args ...interface{}) {
	c.Called(args...)
}

func (c *ConsoleReaderMock) ReadPassword() (string, error) {
	args := c.Called()
	res := args.Get(0)
	if args.Get(1) != nil {
		return res.(string), args.Error(1)
	}
	return res.(string), nil
}

func (c *ConsoleReaderMock) ReadLine() (string, error) {
	args := c.Called()
	res := args.Get(0)
	if args.Get(1) != nil {
		return res.(string), args.Error(1)
	}
	return res.(string), nil
}
