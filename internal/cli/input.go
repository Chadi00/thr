package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func readTextArgFileOrStdin(argText string, filePath string) (string, error) {
	if filePath != "" {
		if argText != "" {
			return "", fmt.Errorf("provide either text argument or --file, not both")
		}
		return readFile(filePath)
	}

	if argText != "" {
		if argText == "-" {
			return readFromStdin()
		}
		return argText, nil
	}

	if hasStdinInput() {
		return readFromStdin()
	}

	return "", fmt.Errorf("no text provided; pass text argument, --file, or pipe stdin")
}

func readFile(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file %q: %w", path, err)
	}
	return strings.TrimRight(string(body), "\n"), nil
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
		return "", fmt.Errorf("no text provided; pass text argument, --file, or pipe stdin")
	}
	return value, nil
}

func hasStdinInput() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice == 0
}
