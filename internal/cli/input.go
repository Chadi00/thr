package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const defaultMaxMemoryBytes int64 = 256 * 1024

func readTextArgOrExplicitStdin(argText string, maxBytes int64) (string, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxMemoryBytes
	}
	if argText != "" {
		if argText == "-" {
			return readFromStdin(maxBytes)
		}
		if int64(len([]byte(argText))) > maxBytes {
			return "", fmt.Errorf("text is too large: maximum is %d bytes", maxBytes)
		}
		return argText, nil
	}

	return "", fmt.Errorf("no text provided; pass text argument or '-' for stdin")
}

func readFromReader(reader io.Reader, maxBytes int64) (string, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return "", fmt.Errorf("stdin is too large: maximum is %d bytes", maxBytes)
	}
	return strings.TrimRight(string(body), "\n"), nil
}

func readFromStdin(maxBytes int64) (string, error) {
	value, err := readFromReader(os.Stdin, maxBytes)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("no text provided; pass text argument or '-' for stdin")
	}
	return value, nil
}
