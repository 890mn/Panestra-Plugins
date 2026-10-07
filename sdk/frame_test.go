package sdk

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"testing"
)

type shortWriter struct{}

func (shortWriter) Write(raw []byte) (int, error) { return len(raw) - 1, nil }

func TestFramerRejectsInvalidInputAndShortWrites(t *testing.T) {
	for _, size := range []uint32{0, MaxFrame + 1} {
		var header [4]byte
		binary.BigEndian.PutUint32(header[:], size)
		if _, err := (&Framer{R: bytes.NewReader(header[:])}).Read(); err == nil {
			t.Fatal("invalid frame size accepted", size)
		}
	}
	for _, raw := range [][]byte{[]byte("{}"), []byte(`{"jsonrpc":"1.0"}`), []byte("invalid")} {
		var buffer bytes.Buffer
		binary.Write(&buffer, binary.BigEndian, uint32(len(raw)))
		buffer.Write(raw)
		if _, err := (&Framer{R: &buffer}).Read(); err == nil {
			t.Fatal("invalid JSON-RPC accepted")
		}
	}
	if err := (&Framer{W: shortWriter{}}).Write(Message{Method: "health"}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("short write not reported", err)
	}
	if _, err := (&Framer{R: bytes.NewReader([]byte{0, 0, 0, 9, '{'})}).Read(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("truncated frame accepted", err)
	}
}

func TestConcurrentPublishFramesDoNotInterleave(t *testing.T) {
	var buffer bytes.Buffer
	writer := &Framer{W: &buffer}
	var group sync.WaitGroup
	for index := 0; index < 32; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := writer.Write(Message{Method: "publish", Params: Raw(map[string]any{"value": 42})}); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	reader := &Framer{R: &buffer}
	for index := 0; index < 32; index++ {
		message, err := reader.Read()
		if err != nil || message.Method != "publish" {
			t.Fatal("concurrent frame corrupted", err)
		}
	}
	if buffer.Len() != 0 {
		t.Fatal("unexpected trailing frames")
	}
}

func TestRejectedHandshakeNeverInitializesPlugin(t *testing.T) {
	var input, output bytes.Buffer
	if err := (&Framer{W: &input}).Write(Message{ID: "hello", Error: "denied"}); err != nil {
		t.Fatal(err)
	}
	initialized := false
	err := Serve(context.Background(), &Framer{R: &input, W: &output}, "dev.example.test", "unused", "digest", func(context.Context, string, []string, Publish) (Handler, error) { initialized = true; return nil, nil })
	if err == nil || initialized {
		t.Fatal("rejected host initialized plugin")
	}
	message, err := (&Framer{R: &output}).Read()
	if err != nil || message.Method != "hello" {
		t.Fatal("identity handshake missing", err)
	}
}

func TestDecodeRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	for _, raw := range []string{`{"operation":"status","extra":true}`, `{"operation":"status"}{}`} {
		if _, err := Decode[Request]([]byte(raw)); err == nil {
			t.Fatal("ambiguous request accepted")
		}
	}
	if CanConfigure(Request{Role: "viewer"}) || CanControl(Request{Role: "viewer"}) || !CanControl(Request{Role: "operator"}) {
		t.Fatal("role boundary changed")
	}
}
