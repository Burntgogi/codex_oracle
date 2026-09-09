package browser

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CDPClient struct {
	conn   net.Conn
	reader *bufio.Reader
	nextID int
	mu     sync.Mutex
}

func DialCDP(ctx context.Context, rawURL string) (*CDPClient, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "ws" {
		return nil, fmt.Errorf("unsupported CDP websocket scheme %q", parsed.Scheme)
	}
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", parsed.Host)
	if err != nil {
		return nil, err
	}
	client := &CDPClient{conn: conn, reader: bufio.NewReader(conn)}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
		defer conn.SetDeadline(time.Time{})
	}
	if err := client.handshake(parsed); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}

func (c *CDPClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *CDPClient) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return c.call(ctx, "", method, params)
}

func (c *CDPClient) CallSession(ctx context.Context, sessionID string, method string, params any) (json.RawMessage, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("session id is required")
	}
	return c.call(ctx, sessionID, method, params)
}

func (c *CDPClient) call(ctx context.Context, sessionID string, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	id := c.nextID
	command := map[string]any{
		"id":     id,
		"method": method,
	}
	if sessionID != "" {
		command["sessionId"] = sessionID
	}
	if params != nil {
		command["params"] = params
	}
	payload, err := json.Marshal(command)
	if err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.conn.SetDeadline(deadline)
		defer c.conn.SetDeadline(time.Time{})
	}
	if err := writeClientFrame(c.conn, payload); err != nil {
		return nil, err
	}
	for {
		message, err := readServerTextFrame(c.reader)
		if err != nil {
			return nil, err
		}
		var response struct {
			ID        int             `json:"id"`
			SessionID string          `json:"sessionId"`
			Result    json.RawMessage `json:"result"`
			Error     json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(message, &response); err != nil {
			return nil, err
		}
		if response.ID != id {
			continue
		}
		if sessionID != "" && response.SessionID != "" && response.SessionID != sessionID {
			continue
		}
		if len(response.Error) > 0 {
			return nil, fmt.Errorf("CDP error: %s", string(response.Error))
		}
		return response.Result, nil
	}
}

func (c *CDPClient) handshake(parsed *url.URL) error {
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	path := parsed.RequestURI()
	if path == "" {
		path = "/"
	}
	request := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + parsed.Host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(c.conn, request); err != nil {
		return err
	}
	status, err := c.reader.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.Contains(status, " 101 ") {
		return fmt.Errorf("websocket upgrade failed: %s", strings.TrimSpace(status))
	}
	accept := ""
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), "Sec-WebSocket-Accept") {
			accept = strings.TrimSpace(value)
		}
	}
	if accept != websocketAccept(key) {
		return fmt.Errorf("websocket accept mismatch")
	}
	return nil
}

func writeClientFrame(writer io.Writer, payload []byte) error {
	header := []byte{0x81}
	length := len(payload)
	switch {
	case length < 126:
		header = append(header, byte(length)|0x80)
	case length <= 65535:
		header = append(header, 126|0x80, byte(length>>8), byte(length))
	default:
		header = append(header, 127|0x80)
		var extended [8]byte
		binary.BigEndian.PutUint64(extended[:], uint64(length))
		header = append(header, extended[:]...)
	}
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}
	masked := make([]byte, len(payload))
	for i := range payload {
		masked[i] = payload[i] ^ mask[i%4]
	}
	if _, err := writer.Write(header); err != nil {
		return err
	}
	if _, err := writer.Write(mask); err != nil {
		return err
	}
	_, err := writer.Write(masked)
	return err
}

func readServerTextFrame(reader *bufio.Reader) ([]byte, error) {
	for {
		header := make([]byte, 2)
		if _, err := io.ReadFull(reader, header); err != nil {
			return nil, err
		}
		opcode := header[0] & 0x0f
		masked := header[1]&0x80 != 0
		length := int(header[1] & 0x7f)
		switch length {
		case 126:
			extended := make([]byte, 2)
			if _, err := io.ReadFull(reader, extended); err != nil {
				return nil, err
			}
			length = int(binary.BigEndian.Uint16(extended))
		case 127:
			extended := make([]byte, 8)
			if _, err := io.ReadFull(reader, extended); err != nil {
				return nil, err
			}
			wide := binary.BigEndian.Uint64(extended)
			if wide > uint64(^uint(0)>>1) {
				return nil, fmt.Errorf("websocket frame too large: %s", strconv.FormatUint(wide, 10))
			}
			length = int(wide)
		}
		var mask []byte
		if masked {
			mask = make([]byte, 4)
			if _, err := io.ReadFull(reader, mask); err != nil {
				return nil, err
			}
		}
		payload := make([]byte, length)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return nil, err
		}
		if masked {
			for i := range payload {
				payload[i] ^= mask[i%4]
			}
		}
		switch opcode {
		case 0x1:
			return payload, nil
		case 0x8:
			return nil, io.EOF
		case 0x9:
			_ = writePongFrame(reader, payload)
		}
	}
}

func writePongFrame(_ *bufio.Reader, _ []byte) error {
	return nil
}

func websocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(sum[:])
}
