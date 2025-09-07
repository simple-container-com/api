package util

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/howeyc/gopass"
)

var (
	DefaultConsoleReader ConsoleReader = StdinConsoleReader{}
	DefaultConsoleWriter ConsoleWriter = StdoutConsoleWriter{}
)

type ConsoleReader interface {
	ReadLine() (string, error)
	ReadPassword() (string, error)
}

type ConsoleWriter interface {
	Print(args ...interface{})
	Println(args ...interface{})
}

type StdinConsoleReader struct{}

type StdoutConsoleWriter struct{}

type ConsoleImpl struct {
	reader        ConsoleReader
	writer        ConsoleWriter
	alwaysDefault bool
}

type Console interface {
	AskYesNoQuestionWithDefault(question string, yes bool) (bool, error)
	AskQuestionWithDefault(question string, defaultResponse string) (string, error)
	AskQuestion(question string) (string, error)
	Writer() ConsoleWriter
	Reader() ConsoleReader
	SetReader(reader ConsoleReader)
	SetWriter(writer ConsoleWriter)
}

func NewDefaultConsole() *ConsoleImpl {
	return &ConsoleImpl{writer: DefaultConsoleWriter, reader: DefaultConsoleReader}
}

func (c *ConsoleImpl) AlwaysRespondDefault() *ConsoleImpl {
	c.alwaysDefault = true
	return c
}

func (c *ConsoleImpl) AskYesNoQuestionWithDefault(question string, yes bool) (bool, error) {
	defaultResp := "N"
	if yes {
		defaultResp = "Y"
	}
	resp, err := c.AskQuestionWithDefault(question, defaultResp)
	return strings.ToUpper(resp) == "Y", err
}

func (c *ConsoleImpl) AskQuestionWithDefault(question string, defaultResponse string) (string, error) {
	c.writer.Print(question, " [", defaultResponse, "]: ")
	if c.alwaysDefault {
		c.writer.Println()
		return defaultResponse, nil
	}
	res, err := c.reader.ReadLine()
	res = strings.TrimSpace(res)

	if res == "" {
		res = defaultResponse
	}

	return res, err
}

func (c *ConsoleImpl) SetWriter(writer ConsoleWriter) {
	c.writer = writer
}

func (c *ConsoleImpl) Writer() ConsoleWriter {
	return c.writer
}

func (c *ConsoleImpl) SetReader(reader ConsoleReader) {
	c.reader = reader
}

func (c *ConsoleImpl) Reader() ConsoleReader {
	return c.reader
}

func (c *ConsoleImpl) AskQuestion(question string) (string, error) {
	c.writer.Print(question, ": ")
	if c.alwaysDefault {
		return "", errors.New("cannot respond with default: no default provided")
	}
	res, err := c.reader.ReadLine()
	res = strings.TrimSpace(res)

	return res, err
}

func (w StdoutConsoleWriter) Print(args ...interface{}) {
	fmt.Print(args...)
}

func (w StdoutConsoleWriter) Println(args ...interface{}) {
	fmt.Println(args...)
}

func (reader StdinConsoleReader) ReadPassword() (string, error) {
	bytePass, err := gopass.GetPasswd()
	return string(bytePass), err
}

func (reader StdinConsoleReader) ReadLine() (string, error) {
	input, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(input, "\n"), nil
}

// MultiWriteCloser multiWriteCloser creates a WriteCloser that duplicates its writes and
// closes to all the provided writers.
func MultiWriteCloser(wc ...io.WriteCloser) io.WriteCloser {
	return mwc(wc)
}

type mwc []io.WriteCloser

func (m mwc) Write(p []byte) (n int, err error) {
	for _, w := range m {
		if n, err = w.Write(p); err != nil {
			return //nolint: gofumpt
		}
	}
	return
}

func (m mwc) Close() error {
	for _, w := range m {
		if err := w.Close(); err != nil {
			return err
		}
	}
	return nil
}
