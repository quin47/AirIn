package asr

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 100 * time.Millisecond
	pingPeriod = 30 * time.Second
	readLimit  = 256 * 1024
)

type client struct {
	cfg Config

	conn   *websocket.Conn
	connMu sync.Mutex

	resultCh chan RecognitionResult
	errCh    chan error

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewClient(cfg Config) Client {
	return &client{
		cfg:      cfg,
		resultCh: make(chan RecognitionResult, 64),
		errCh:    make(chan error, 8),
	}
}

func (c *client) Connect(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)

	var header http.Header
	if c.cfg.AccessKey != "" && c.cfg.SecretKey != "" {
		var err error
		header, err = signHeaders(c.cfg.AccessKey, c.cfg.SecretKey)
		if err != nil {
			return fmt.Errorf("sign headers: %w", err)
		}
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.DialContext(c.ctx, wsURL(), header)
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}
	c.conn = conn
	conn.SetReadLimit(readLimit)

	if err := c.sendFullClientRequest(); err != nil {
		conn.Close()
		return fmt.Errorf("send handshake: %w", err)
	}

	c.wg.Add(2)
	go c.readLoop()
	go c.pingLoop()

	log.Printf("asr: WebSocket 已连接 (cluster=%s)", c.cfg.Cluster)
	return nil
}

func (c *client) sendFullClientRequest() error {
	req := fullClientRequest{
		App: appInfo{
			AppID:   c.cfg.AppID,
			Token:   c.cfg.Token,
			Cluster: c.cfg.Cluster,
		},
		User: userInfo{
			UID: c.cfg.UID,
		},
		Audio: audioInfo{
			Format:   c.cfg.Format,
			Codec:    c.cfg.Codec,
			Rate:     c.cfg.Rate,
			Bits:     c.cfg.Bits,
			Channel:  c.cfg.Channel,
			Language: c.cfg.Language,
		},
		Request: requestInfo{
			ReqID:          newUUID(),
			Sequence:       1,
			ShowUtterances: c.cfg.ShowUtterances,
			ResultType:     c.cfg.ResultType,
			NBest:          c.cfg.NBest,
			Workflow:       c.cfg.Workflow,
		},
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	frame := encodeFullClientRequest(payload)
	return c.write(frame)
}

func (c *client) SendAudio(pcm []int16) error {
	if len(pcm) == 0 {
		return nil
	}
	frame := encodeAudioOnly(pcm)
	return c.write(frame)
}

func (c *client) SendEnd() error {
	frame := encodeAudioLast(nil)
	return c.write(frame)
}

func (c *client) write(data []byte) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("connection closed")
	}
	c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (c *client) readLoop() {
	defer c.wg.Done()
	defer close(c.resultCh)
	defer close(c.errCh)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		_, data, err := c.conn.ReadMessage()
		if err != nil {
			select {
			case c.errCh <- fmt.Errorf("read: %w", err):
			default:
			}
			return
		}

		resp, err := decodeFrame(data)
		if err != nil {
			select {
			case c.errCh <- fmt.Errorf("decode: %w", err):
			default:
			}
			continue
		}

		for _, utterance := range resp.Result {
			result := RecognitionResult{
				SeqNum:   resp.Sequence,
				Text:     utterance.Text,
				Definite: utterance.Definite,
			}
			if utterance.Definite {
				utt := Utterance{
					Text:      utterance.Text,
					StartTime: utterance.StartTime,
					EndTime:   utterance.EndTime,
					Words:     utterance.Words,
				}
				result.Utterance = &utt
			}

			select {
			case c.resultCh <- result:
			case <-c.ctx.Done():
				return
			}
		}
	}
}

func (c *client) pingLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.connMu.Lock()
			if c.conn != nil {
				c.conn.SetWriteDeadline(time.Now().Add(writeWait))
				c.conn.WriteMessage(websocket.PingMessage, nil)
			}
			c.connMu.Unlock()
		}
	}
}

func (c *client) Results() <-chan RecognitionResult {
	return c.resultCh
}

func (c *client) Errors() <-chan error {
	return c.errCh
}

func (c *client) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()

	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.conn != nil {
		msg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
		c.conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(writeWait))
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
