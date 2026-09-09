package browser_test

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	"codex_oracle/internal/browser"
)

func TestCDPClientCallSendsJSONCommand(t *testing.T) {
	serverURL, done := startFakeWebSocketServer(t, func(t *testing.T, command map[string]any) string {
		t.Helper()
		if command["method"] != "Browser.getVersion" {
			t.Fatalf("method = %#v, want Browser.getVersion", command["method"])
		}
		params, ok := command["params"].(map[string]any)
		if !ok || params["x"] != "y" {
			t.Fatalf("params = %#v, want x=y", command["params"])
		}
		return `{"id":1,"result":{"product":"Chrome/Fake"}}`
	})

	client, err := browser.DialCDP(context.Background(), serverURL)
	if err != nil {
		t.Fatalf("DialCDP returned error: %v", err)
	}
	defer client.Close()

	result, err := client.Call(context.Background(), "Browser.getVersion", map[string]any{"x": "y"})
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}

	var payload struct {
		Product string `json:"product"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal result: %v", err)
	}
	if payload.Product != "Chrome/Fake" {
		t.Fatalf("Product = %q, want Chrome/Fake", payload.Product)
	}
	<-done
}

func TestCDPClientCallSessionSendsSessionID(t *testing.T) {
	serverURL, done := startFakeWebSocketServer(t, func(t *testing.T, command map[string]any) string {
		t.Helper()
		if command["method"] != "Runtime.evaluate" {
			t.Fatalf("method = %#v, want Runtime.evaluate", command["method"])
		}
		if command["sessionId"] != "session-1" {
			t.Fatalf("sessionId = %#v, want session-1", command["sessionId"])
		}
		return `{"id":1,"sessionId":"session-1","result":{"result":{"type":"string","value":"ok"}}}`
	})

	client, err := browser.DialCDP(context.Background(), serverURL)
	if err != nil {
		t.Fatalf("DialCDP returned error: %v", err)
	}
	defer client.Close()

	result, err := client.CallSession(context.Background(), "session-1", "Runtime.evaluate", map[string]any{"expression": "1"})
	if err != nil {
		t.Fatalf("CallSession returned error: %v", err)
	}

	if !strings.Contains(string(result), `"value":"ok"`) {
		t.Fatalf("result = %s, want value ok", result)
	}
	<-done
}

func startFakeWebSocketServer(t *testing.T, handle func(*testing.T, map[string]any) string) (string, <-chan struct{}) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer listener.Close()
		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("Accept: %v", err)
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		key := ""
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Errorf("ReadString: %v", err)
				return
			}
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				break
			}
			if strings.HasPrefix(strings.ToLower(trimmed), "sec-websocket-key:") {
				key = strings.TrimSpace(strings.SplitN(trimmed, ":", 2)[1])
			}
		}
		accept := websocketAccept(key)
		_, _ = io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\n")
		_, _ = io.WriteString(conn, "Upgrade: websocket\r\n")
		_, _ = io.WriteString(conn, "Connection: Upgrade\r\n")
		_, _ = io.WriteString(conn, "Sec-WebSocket-Accept: "+accept+"\r\n\r\n")

		message, err := readClientFrame(reader)
		if err != nil {
			t.Errorf("readClientFrame: %v", err)
			return
		}
		var command map[string]any
		if err := json.Unmarshal([]byte(message), &command); err != nil {
			t.Errorf("Unmarshal command: %v", err)
			return
		}
		_ = writeServerFrame(conn, handle(t, command))
	}()
	u := url.URL{Scheme: "ws", Host: listener.Addr().String(), Path: "/devtools/browser/fake"}
	return u.String(), done
}

func websocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func readClientFrame(reader *bufio.Reader) (string, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil {
		return "", err
	}
	masked := header[1]&0x80 != 0
	length := int(header[1] & 0x7f)
	if length == 126 {
		extended := make([]byte, 2)
		if _, err := io.ReadFull(reader, extended); err != nil {
			return "", err
		}
		length = int(binary.BigEndian.Uint16(extended))
	}
	mask := make([]byte, 4)
	if masked {
		if _, err := io.ReadFull(reader, mask); err != nil {
			return "", err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return "", err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return string(payload), nil
}

func writeServerFrame(writer io.Writer, payload string) error {
	data := []byte(payload)
	header := []byte{0x81}
	if len(data) < 126 {
		header = append(header, byte(len(data)))
	} else {
		header = append(header, 126, byte(len(data)>>8), byte(len(data)))
	}
	if _, err := writer.Write(header); err != nil {
		return err
	}
	_, err := writer.Write(data)
	return err
}

func TestCDPDialRejectsNonWebSocketURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := browser.DialCDP(ctx, "http://127.0.0.1/devtools")
	if err == nil {
		t.Fatal("DialCDP returned nil error, want unsupported scheme error")
	}
}
