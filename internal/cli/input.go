package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func readTextArgOrExplicitStdin(argText string) (string, error) {
	if argText != "" {
		if argText == "-" {
			return readFromStdin()
		}
		return argText, nil
	}

	return "", fmt.Errorf("no text provided; pass text argument or '-' for stdin")
}

func readFromReader(reader io.Reader) (string, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return strings.TrimRight(string(body), "\n"), nil
}

func readFromStdin() (string, error) {
	value, err := readFromReader(os.Stdin)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("no text provided; pass text argument or '-' for stdin")
	}
	return value, nil
}
