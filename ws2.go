package gost

import (
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	"net/url"

	"github.com/go-log/log"
	"github.com/gorilla/websocket"
)

const (
	defaultWSPath2 = "/ws"
)

// // WSOptions describes the options for websocket.
type WSOptions2 struct {
	ReadBufferSize    int
	WriteBufferSize   int
	HandshakeTimeout  time.Duration
	EnableCompression bool
	UserAgent         string
	Path              string
}

type wsTransporter2 struct {
	tcpTransporter
	options *WSOptions
}

// WSTransporter creates a Transporter that is used by websocket proxy client.
func WSTransporter2(opts *WSOptions) Transporter {
	return &wsTransporter{
		options: opts,
	}
}

func (tr *wsTransporter2) Handshake(conn net.Conn, options ...HandshakeOption) (net.Conn, error) {
	opts := &HandshakeOptions{}
	for _, option := range options {
		option(opts)
	}
	wsOptions := tr.options
	if opts.WSOptions != nil {
		wsOptions = opts.WSOptions
	}
	if wsOptions == nil {
		wsOptions = &WSOptions{}
	}

	path := wsOptions.Path
	if path == "" {
		path = defaultWSPath
	}
	url := url.URL{Scheme: "ws", Host: opts.Host, Path: path}
	return websocketClientConn(url.String(), conn, nil, wsOptions)
}

type wsListener2 struct {
	addr     net.Addr
	upgrader *websocket.Upgrader
	srv      *http.Server
	connChan chan net.Conn
	errChan  chan error
}

// WSListener creates a Listener for websocket proxy server.
func WSListener2(ln net.Listener, options *WSOptions) (Listener, error) {

	if options == nil {
		options = &WSOptions{}
	}
	l := &wsListener2{
		upgrader: &websocket.Upgrader{
			ReadBufferSize:    options.ReadBufferSize,
			WriteBufferSize:   options.WriteBufferSize,
			CheckOrigin:       func(r *http.Request) bool { return true },
			EnableCompression: options.EnableCompression,
		},
		connChan: make(chan net.Conn, 1024),
		errChan:  make(chan error, 1),
	}

	path := options.Path
	if path == "" {
		path = defaultWSPath2
	}
	mux := http.NewServeMux()
	mux.Handle(path, http.HandlerFunc(l.upgrade))
	l.srv = &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
	}

	l.addr = ln.Addr()

	go func() {
		err := l.srv.Serve(ln)
		if err != nil {
			l.errChan <- err
		}
		close(l.errChan)
	}()
	select {
	case err := <-l.errChan:
		return nil, err
	default:
	}

	return l, nil
}

func (l *wsListener2) upgrade(w http.ResponseWriter, r *http.Request) {
	log.Logf("[ws] %s -> %s", r.RemoteAddr, l.addr)
	if Debug {
		dump, _ := httputil.DumpRequest(r, false)
		log.Log(string(dump))
	}
	conn, err := l.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Logf("[ws] %s - %s : %s", r.RemoteAddr, l.addr, err)
		return
	}
	select {
	case l.connChan <- websocketServerConn(conn):
	default:
		conn.Close()
		log.Logf("[ws] %s - %s: connection queue is full", r.RemoteAddr, l.addr)
	}
}

func (l *wsListener2) Accept() (conn net.Conn, err error) {
	select {
	case conn = <-l.connChan:
	case err = <-l.errChan:
	}
	return
}

func (l *wsListener2) Close() error {
	return l.srv.Close()
}

func (l *wsListener2) Addr() net.Addr {
	return l.addr
}
