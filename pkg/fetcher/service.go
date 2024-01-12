package fetcher

import (
	"bytes"
	"encoding/base64"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"net/http"
	"strings"
)

type Service struct {
}

func (s *Service) Get(url string) ([]byte, error) {
	if strings.HasPrefix(url, "base64://") {
		return base64.StdEncoding.DecodeString(url[9:])
	}

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to send GET request to %s %w", url, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("failed to send GET request to %s: status code is %d", url, resp.StatusCode)
	}

	buffer := bytes.Buffer{}
	_, err = buffer.ReadFrom(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body for %s %w", url, err)
	}

	return buffer.Bytes(), nil
}
