package asr

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
)

const (
	headerSize = 8

	protocolVersion = 0x1
	headerUnits     = 0x2

	msgFullClientRequest = 0x1
	msgAudioOnly         = 0x2
	msgServerResponse    = 0x9
	msgServerError       = 0xf

	flagNone = 0x0
	flagLast = 0x2

	serializationNone = 0x0
	serializationJSON = 0x1

	compressionNone = 0x0
)

func packHeader(msgType, flags, serialization, compression uint8, payloadLen uint32) []byte {
	h := make([]byte, headerSize)
	h[0] = (protocolVersion << 4) | headerUnits
	h[1] = (msgType << 4) | flags
	h[2] = (serialization << 4) | compression
	binary.BigEndian.PutUint32(h[4:], payloadLen)
	return h
}

func unpackHeader(data []byte) (msgType, flags, serialization, compression uint8, payloadLen uint32, err error) {
	if len(data) < headerSize {
		return 0, 0, 0, 0, 0, fmt.Errorf("header too short: %d bytes", len(data))
	}
	msgType = (data[1] >> 4) & 0x0f
	flags = data[1] & 0x0f
	serialization = (data[2] >> 4) & 0x0f
	compression = data[2] & 0x0f
	payloadLen = binary.BigEndian.Uint32(data[4:8])
	return
}

func encodeFullClientRequest(payload []byte) []byte {
	h := packHeader(msgFullClientRequest, flagNone, serializationJSON, compressionNone, uint32(len(payload)))
	return append(h, payload...)
}

func encodeAudioOnly(pcm []int16) []byte {
	buf := make([]byte, len(pcm)*2)
	for i, sample := range pcm {
		binary.BigEndian.PutUint16(buf[i*2:], uint16(sample))
	}
	h := packHeader(msgAudioOnly, flagNone, serializationNone, compressionNone, uint32(len(buf)))
	return append(h, buf...)
}

func encodeAudioLast(pcm []int16) []byte {
	buf := make([]byte, len(pcm)*2)
	for i, sample := range pcm {
		binary.BigEndian.PutUint16(buf[i*2:], uint16(sample))
	}
	h := packHeader(msgAudioOnly, flagLast, serializationNone, compressionNone, uint32(len(buf)))
	return append(h, buf...)
}

func decodeFrame(data []byte) (*serverResponse, error) {
	if len(data) < headerSize {
		return nil, fmt.Errorf("frame too short: %d bytes", len(data))
	}
	msgType, _, serialization, compression, payloadLen, err := unpackHeader(data)
	if err != nil {
		return nil, err
	}
	if msgType != msgServerResponse && msgType != msgServerError {
		return nil, fmt.Errorf("unexpected message type: 0x%x", msgType)
	}
	if compression != compressionNone {
		return nil, fmt.Errorf("unsupported compression: 0x%x", compression)
	}
	if serialization != serializationJSON {
		return nil, fmt.Errorf("unsupported serialization: 0x%x", serialization)
	}
	if uint32(len(data)-headerSize) < payloadLen {
		return nil, fmt.Errorf("payload truncated: have %d, want %d", len(data)-headerSize, payloadLen)
	}
	payload := data[headerSize : headerSize+int(payloadLen)]
	var resp serverResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if resp.Code != 1000 {
		return nil, fmt.Errorf("server error: code=%d, msg=%s", resp.Code, resp.Message)
	}
	return &resp, nil
}
