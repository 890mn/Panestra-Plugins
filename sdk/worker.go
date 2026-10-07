package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Request is sent over the local, parent-owned IPC channel, never a network listener.
type Request struct {
	Operation string          `json:"operation"`
	Role      string          `json:"role"`
	Body      json.RawMessage `json:"body"`
}
type Response struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}
type Publish func(source string, value any)
type Handler func(context.Context, Request) Response
type Factory func(context.Context, string, []string, Publish) (Handler, error)

func Result(value any) Response { return Response{200, Raw(value)} }
func Failure(status int, code, message string) Response {
	return Response{status, Raw(map[string]string{"code": code, "message": message})}
}
func Error(err error) Response { return Failure(400, "REQUEST_FAILED", err.Error()) }
func Decode[T any](raw json.RawMessage) (T, error) {
	var value T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, errors.New("请求格式无效")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return value, errors.New("请求包含多余内容")
	}
	return value, nil
}
func CanConfigure(r Request) bool { return r.Role == "owner" }
func CanControl(r Request) bool   { return r.Role == "owner" || r.Role == "operator" }
func Denied() Response            { return Failure(403, "FORBIDDEN", "此设备没有操作权限") }

func Run(id string, factory Factory) {
	if err := Serve(context.Background(), &Framer{R: os.Stdin, W: os.Stdout}, id, os.Getenv("PANESTRA_PLUGIN_DATA"), os.Getenv("PANESTRA_MANIFEST_DIGEST"), factory); err != nil {
		fmt.Fprintln(os.Stderr, "plugin stopped:", err)
		os.Exit(1)
	}
}

// Serve initializes only after the host checks the identity and grants.
func Serve(parent context.Context, frame *Framer, id, dataDir, digest string, factory Factory) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	if err := frame.Write(Message{ID: "hello", Method: "hello", Params: Raw(map[string]any{"pluginId": id, "version": Version, "protocolVersion": 1, "manifestDigest": digest})}); err != nil {
		return err
	}
	hello, err := frame.Read()
	if err != nil {
		return err
	}
	if hello.ID != "hello" || hello.Error != "" {
		return errors.New("host rejected handshake")
	}
	var welcome struct {
		Protocol int      `json:"protocolVersion"`
		Granted  []string `json:"grantedCapabilities"`
	}
	if json.Unmarshal(hello.Result, &welcome) != nil || welcome.Protocol != 1 {
		return errors.New("invalid host handshake")
	}
	var stopped bool
	var publishMu sync.Mutex
	publish := func(source string, value any) {
		publishMu.Lock()
		defer publishMu.Unlock()
		if !stopped {
			if err := frame.Write(Message{Method: "publish", Params: Raw(map[string]any{"source": source, "value": value})}); err != nil {
				stopped = true
				cancel()
			}
		}
	}
	defer func() { publishMu.Lock(); stopped = true; publishMu.Unlock() }()
	handler, err := factory(ctx, dataDir, welcome.Granted, publish)
	if err != nil {
		return err
	}
	if err := frame.Write(Message{Method: "health"}); err != nil {
		return err
	}
	slots := make(chan struct{}, 4)
	for {
		message, err := frame.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		switch message.Method {
		case "health":
			if err := frame.Write(Message{Method: "health"}); err != nil {
				return err
			}
		case "request":
			request, err := Decode[Request](message.Params)
			if err != nil || message.ID == "" {
				return errors.New("invalid host request")
			}
			select {
			case slots <- struct{}{}:
				go func(message Message, request Request) {
					defer func() { <-slots }()
					requestCtx, stop := context.WithTimeout(ctx, 30*time.Second)
					defer stop()
					response := handler(requestCtx, request)
					_ = frame.Write(Message{ID: message.ID, Result: Raw(response)})
				}(message, request)
			default:
				_ = frame.Write(Message{ID: message.ID, Result: Raw(Failure(429, "BUSY", "插件正在处理其他请求"))})
			}
		default:
			return errors.New("unsupported host method")
		}
	}
}
