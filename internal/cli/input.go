package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func readTextArgFileOrStdin(argText string, filePath string) (string, error) {
	sources := 0
	if argText != "" {
		sources++
	}
	if filePath != "" {
		sources++
	}
	if hasStdinInput() {
		sources++
	}

	if sources == 0 {
		return "", fmt.Errorf("no text provided; pass text argument, --file, or pipe stdin")
	}
	if sources > 1 {
		return "", fmt.Errorf("provide exactly one text source: argument, --file, or stdin")
	}
	if argText != "" {
		if argText == "-" {
			return readFromReader(os.Stdin)
		}
		return argText, nil
	}
	if filePath != "" {
		return readFile(filePath)
	}
	return readFromReader(os.Stdin)
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

func hasStdinInput() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice == 0
}
