package transfer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

const minSecretLen = 8

// PromptSecret reads a secret from the terminal without echo, printing the
// prompt to stderr. With confirm set, it reads twice and requires both
// entries to match. Secrets shorter than 8 characters are rejected.
func PromptSecret(prompt string, confirm bool) (string, error) {
	first, err := readSecret(prompt)
	if err != nil {
		return "", err
	}
	if len(first) < minSecretLen {
		return "", fmt.Errorf("secret must be at least %d characters", minSecretLen)
	}
	if !confirm {
		return first, nil
	}
	again, err := readSecret(prompt)
	if err != nil {
		return "", err
	}
	if again != first {
		return "", errors.New("secrets do not match")
	}
	return first, nil
}

func readSecret(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	fd := int(os.Stdin.Fd())
	if old, err := term.GetState(fd); err == nil {
		defer func() { _ = term.Restore(fd, old) }()
	}
	data, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("failed to read secret: %w", err)
	}
	return string(data), nil
}

// SecretFromFile reads a secret from path ("-" reads stdin) and trims
// exactly one trailing newline. An empty result is an error.
func SecretFromFile(path string) (string, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return "", fmt.Errorf("failed to read secret file: %w", err)
	}
	s := string(data)
	if strings.HasSuffix(s, "\r\n") {
		s = strings.TrimSuffix(s, "\r\n")
	} else {
		s = strings.TrimSuffix(s, "\n")
	}
	if s == "" {
		return "", errors.New("secret file is empty")
	}
	return s, nil
}
