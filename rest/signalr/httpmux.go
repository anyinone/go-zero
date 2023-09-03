package signalr

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anyinone/jsoniter"
	"github.com/teivah/onecontext"
	"nhooyr.io/websocket"
)

type httpMux struct {
	server Server
}

func newHTTPMux(server Server) *httpMux {
	return &httpMux{
		server: server,
	}
}

func (h *httpMux) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if strings.Contains(request.RequestURI, "negotiate") {
		h.negotiate(writer, request)
	} else {
		switch request.Method {
		case "POST":
			h.handlePost(writer, request)
		case "GET":
			h.handleGet(writer, request)
		default:
			writer.WriteHeader(http.StatusBadRequest)
		}
	}
}

func (h *httpMux) handlePost(writer http.ResponseWriter, request *http.Request) {
	connectionID := request.URL.Query().Get("id")
	if connectionID == "" {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	info, _ := h.server.prefixLoggers("")
	for {
		c := h.Load(connectionID)
		switch conn := c.(type) {
		case *serverSSEConnection:
			writer.WriteHeader(conn.consumeRequest(request))
			return
		case *negotiateConnection:
			// connection start initiated but not completed
		default:
			// ConnectionID already used for WebSocket(?)
			writer.WriteHeader(http.StatusConflict)
			return
		}
		<-time.After(10 * time.Millisecond)
		_ = info.Log("event", "handlePost for SSE connection repeated")
	}
}

func (h *httpMux) handleGet(writer http.ResponseWriter, request *http.Request) {
	upgrade := false
	for _, connHead := range strings.Split(request.Header.Get("Connection"), ",") {
		if strings.ToLower(strings.TrimSpace(connHead)) == "upgrade" {
			upgrade = true
			break
		}
	}
	if upgrade &&
		strings.ToLower(request.Header.Get("Upgrade")) == "websocket" {
		h.handleWebsocket(writer, request)
	} else if strings.ToLower(request.Header.Get("Accept")) == "text/event-stream" {
		h.handleServerSentEvent(writer, request)
	} else {
		writer.WriteHeader(http.StatusBadRequest)
	}
}

func (h *httpMux) handleServerSentEvent(writer http.ResponseWriter, request *http.Request) {
	connectionID := request.URL.Query().Get("id")
	if connectionID == "" {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	c := h.Load(connectionID)
	if _, ok := c.(*negotiateConnection); ok {
		ctx, _ := onecontext.Merge(h.server.context(), request.Context())
		sseConn, jobChan, jobResultChan, err := newServerSSEConnection(ctx, c.ConnectionID(), h.LoadRemoteAddr(request))
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		flusher, ok := writer.(http.Flusher)
		if !ok {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Connection is negotiated but not initiated
		// We compose http and send it over sse
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Connection", "keep-alive")
		writer.Header().Set("Cache-Control", "no-cache")
		writer.WriteHeader(http.StatusOK)
		// End this Server Sent Event (yes, your response now is one and the client will wait for this initial event to end)
		_, _ = fmt.Fprint(writer, ":\r\n\r\n")
		writer.(http.Flusher).Flush()
		go func() {
			// We can't WriteHeader 500 if we get an error as we already wrote the header, so ignore it.
			_ = h.server.Serve(sseConn)
		}()
		// Loop for write jobs from the sseServerConnection
		for buf := range jobChan {
			n, err := writer.Write(buf)
			if err == nil {
				flusher.Flush()
			}
			jobResultChan <- RWJobResult{n: n, err: err}
		}
		close(jobResultChan)
	} else {
		// connectionID in use
		writer.WriteHeader(http.StatusConflict)
	}
}

func (h *httpMux) handleWebsocket(writer http.ResponseWriter, request *http.Request) {
	accOptions := &websocket.AcceptOptions{
		CompressionMode:    websocket.CompressionContextTakeover,
		InsecureSkipVerify: h.server.insecureSkipVerify(),
		OriginPatterns:     h.server.originPatterns(),
	}
	websocketConn, err := websocket.Accept(writer, request, accOptions)
	if err != nil {
		_, debug := h.server.loggers()
		_ = debug.Log(evt, "handleWebsocket", msg, "error accepting websockets", "error", err)
		// don't need to write an error header here as websocket.Accept has already used http.Error
		return
	}
	websocketConn.SetReadLimit(int64(h.server.maximumReceiveMessageSize()))
	connectionMapKey := request.URL.Query().Get("id")
	if connectionMapKey == "" {
		_ = websocketConn.Close(1002, "Bad request")
		return
	}
	c := h.Load(connectionMapKey)
	if _, ok := c.(*negotiateConnection); ok {
		// Connection is negotiated but not initiated
		ctx, _ := onecontext.Merge(h.server.context(), request.Context())
		err = h.server.Serve(newWebSocketConnection(ctx, c.ConnectionID(), websocketConn, h.LoadRemoteAddr(request)))
		if err != nil {
			_ = websocketConn.Close(1005, err.Error())
		}
	} else {
		// Already initiated
		_ = websocketConn.Close(1002, "Bad request")
	}
}

func (h *httpMux) negotiate(w http.ResponseWriter, req *http.Request) {
	connectionID := h.server.createId()
	negotiateVersion, err := strconv.Atoi(req.URL.Query().Get("negotiateVersion"))
	if err != nil {
		negotiateVersion = 0
	}
	connectionToken := ""
	if negotiateVersion == 1 {
		connectionToken = h.server.createId()
	}
	var availableTransports []availableTransport
	for _, transport := range h.server.availableTransports() {
		switch transport {
		case "ServerSentEvents":
			availableTransports = append(availableTransports,
				availableTransport{
					Transport:       "ServerSentEvents",
					TransferFormats: []string{"Text"},
				})
		case "WebSockets":
			availableTransports = append(availableTransports,
				availableTransport{
					Transport:       "WebSockets",
					TransferFormats: []string{"Text", "Binary"},
				})
		}
	}
	response := negotiateResponse{
		ConnectionToken:     connectionToken,
		ConnectionID:        connectionID,
		NegotiateVersion:    negotiateVersion,
		AvailableTransports: availableTransports,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = jsoniter.NewEncoder(w).Encode(response) // Can't imagine an error when encoding
}

func (h *httpMux) Load(token string) Connection {
	if conn, ok := h.server.Load(token); ok {
		return conn
	}
	return &negotiateConnection{
		ConnectionBase{connectionID: token},
	}
}

func (h *httpMux) LoadRemoteAddr(request *http.Request) string {
	realIp := request.Header.Get("X-real-ip")
	if len(realIp) > 0 {
		return realIp
	}
	remoteAddr := request.RemoteAddr
	if strings.Contains(remoteAddr, ":") {
		return strings.Split(remoteAddr, ":")[0]
	}
	return remoteAddr
}

type negotiateConnection struct {
	ConnectionBase
}

func (n *negotiateConnection) Read([]byte) (int, error) {
	return 0, nil
}

func (n *negotiateConnection) Write([]byte) (int, error) {
	return 0, nil
}
