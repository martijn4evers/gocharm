package hook

import (
	"bytes"
	osexec "os/exec"
	"strings"

	"gopkg.in/errgo.v1"
)

// ToolRunner is used to run hook tools.
type ToolRunner interface {
	// Run runs the hook tool with the given name
	// and arguments, and returns its standard output.
	// If the command is unimplemented, it should
	// return an error with an ErrUnimplemented cause.
	Run(cmd string, args ...string) (stdout []byte, err error)
	Close() error
}

// newToolRunnerFromEnvironment returns an implementation of ToolRunner
// that uses a direct connection to the unit agent's socket to
// run the tools.
func newToolRunnerFromEnvironment() (ToolRunner, error) {
	return &execToolRunner{}, nil
}

func isUnimplemented(errStr string) bool {
	return strings.HasPrefix(errStr, "bad request: unknown command")
}

var ErrUnimplemented = errgo.New("unimplemented hook tool")

type execToolRunner struct{}

func (execToolRunner) Run(cmd string, args ...string) ([]byte, error) {
	execCmd := cmd
	c := osexec.Command(execCmd, args...)
	c.Args[0] = cmd
	var errBuf, outBuf bytes.Buffer
	c.Stdout = &outBuf
	c.Stderr = &errBuf

	if err := c.Run(); err != nil {
		if errBuf.Len() > 0 {
			errText := strings.TrimSpace(errBuf.String())
			errText = strings.TrimPrefix(errText, "error: ")
			if isUnimplemented(errText) {
				return nil, errgo.WithCausef(nil, ErrUnimplemented, "%s", errText)
			}
			return nil, errgo.New(errText)
		}
		return nil, err
	}
	return outBuf.Bytes(), nil
}

func (execToolRunner) Close() error {
	return nil
}
