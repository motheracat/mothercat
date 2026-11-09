package opportunities

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	defaultClamAVTimeout = 15 * time.Second
	clamAVChunkSize      = 64 << 10
)

type ClamAVScanner struct {
	address string
	timeout time.Duration
}

type MalwareDetectedError struct {
	Signature string
}

func (e *MalwareDetectedError) Error() string { return "document was rejected by malware scanner" }

func NewClamAVScanner(address string) (*ClamAVScanner, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, errors.New("ClamAV address is required")
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("ClamAV address must be host:port: %w", err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, errors.New("ClamAV port must be between 1 and 65535")
	}
	return &ClamAVScanner{address: address, timeout: defaultClamAVTimeout}, nil
}

func (s *ClamAVScanner) Scan(ctx context.Context, data []byte) error {
	if s == nil || s.address == "" {
		return errors.New("ClamAV scanner is not configured")
	}
	timeout := s.timeout
	if timeout <= 0 {
		timeout = defaultClamAVTimeout
	}
	dialer := net.Dialer{Timeout: timeout}
	connection, err := dialer.DialContext(ctx, "tcp", s.address)
	if err != nil {
		return fmt.Errorf("connect to ClamAV: %w", err)
	}
	defer func() { _ = connection.Close() }()

	deadline := time.Now().Add(timeout)
	if requestDeadline, ok := ctx.Deadline(); ok && requestDeadline.Before(deadline) {
		deadline = requestDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set ClamAV deadline: %w", err)
	}
	if err := writeAll(connection, []byte("zINSTREAM\x00")); err != nil {
		return fmt.Errorf("start ClamAV stream: %w", err)
	}
	for offset := 0; offset < len(data); {
		end := offset + clamAVChunkSize
		if end > len(data) {
			end = len(data)
		}
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(end-offset))
		if err := writeAll(connection, length[:]); err != nil {
			return fmt.Errorf("write ClamAV chunk length: %w", err)
		}
		if err := writeAll(connection, data[offset:end]); err != nil {
			return fmt.Errorf("write ClamAV document chunk: %w", err)
		}
		offset = end
	}
	if err := writeAll(connection, []byte{0, 0, 0, 0}); err != nil {
		return fmt.Errorf("finish ClamAV stream: %w", err)
	}

	response, err := bufio.NewReader(io.LimitReader(connection, 4096)).ReadString(0)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read ClamAV response: %w", err)
	}
	response = strings.TrimSpace(strings.TrimSuffix(response, "\x00"))
	if strings.HasSuffix(response, " OK") {
		return nil
	}
	if strings.Contains(response, " FOUND") {
		return &MalwareDetectedError{Signature: strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(response, "stream: "), " FOUND"))}
	}
	return fmt.Errorf("ClamAV scan failed: %s", safeScannerResponse(response))
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := writer.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

func safeScannerResponse(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "empty scanner response"
	}
	if len(value) > 160 {
		value = value[:160]
	}
	return value
}
