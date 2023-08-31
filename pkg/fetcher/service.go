package fetcher

import (
	"bytes"
	"encoding/base64"
	"fmt"
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

	buffer := bytes.Buffer{}
	_, err = buffer.ReadFrom(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body for %s %w", url, err)
	}

	return buffer.Bytes(), nil
}
