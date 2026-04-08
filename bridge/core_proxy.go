package bridge

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/gorilla/websocket"
)

const webCoreProxyPrefix = "/__core"

type coreConfig struct {
	Experimental struct {
		ClashAPI struct {
			ExternalController string `json:"external_controller"`
			Secret             string `json:"secret"`
		} `json:"clash_api"`
	} `json:"experimental"`
}

func (a *App) handleCoreProxy(w http.ResponseWriter, r *http.Request) {
	if websocket.IsWebSocketUpgrade(r) {
		a.handleCoreWebSocketProxy(w, r)
		return
	}

	target, secret, err := getCoreControllerTarget("http")
	if err != nil {
		writeWebProxyError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = joinURLPath(target.Path, strings.TrimPrefix(r.URL.Path, webCoreProxyPrefix))
		req.URL.RawPath = req.URL.EscapedPath()
		req.URL.RawQuery = r.URL.RawQuery
		req.Host = target.Host

		copyHeaders(req.Header, r.Header)
		req.Header.Del("Origin")
		req.Header.Del("Authorization")
		if secret != "" {
			req.Header.Set("Authorization", "Bearer "+secret)
		}
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		writeWebProxyError(w, http.StatusServiceUnavailable, err.Error())
	}
	proxy.ServeHTTP(w, r)
}

func (a *App) handleCoreWebSocketProxy(w http.ResponseWriter, r *http.Request) {
	target, secret, err := getCoreControllerTarget("ws")
	if err != nil {
		writeWebProxyError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	target.Path = joinURLPath(target.Path, strings.TrimPrefix(r.URL.Path, webCoreProxyPrefix))

	query := r.URL.Query()
	query.Del("token")
	if secret != "" {
		query.Set("token", secret)
	}
	target.RawQuery = query.Encode()

	headers := http.Header{}
	if secret != "" {
		headers.Set("Authorization", "Bearer "+secret)
	}

	upstreamConn, resp, err := websocket.DefaultDialer.Dial(target.String(), headers)
	if err != nil {
		statusCode := http.StatusServiceUnavailable
		message := err.Error()

		if resp != nil {
			statusCode = resp.StatusCode
			if body, readErr := io.ReadAll(resp.Body); readErr == nil {
				resp.Body.Close()
				if len(body) != 0 {
					message = strings.TrimSpace(string(body))
				}
			}
		}

		writeWebProxyError(w, statusCode, message)
		return
	}
	defer upstreamConn.Close()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}

			parsed, err := url.Parse(origin)
			if err != nil {
				return false
			}

			return parsed.Host == r.Host
		},
	}

	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer clientConn.Close()

	errCh := make(chan error, 2)

	go proxyWebSocketMessages(errCh, clientConn, upstreamConn)
	go proxyWebSocketMessages(errCh, upstreamConn, clientConn)

	<-errCh
}

func proxyWebSocketMessages(errCh chan<- error, src *websocket.Conn, dst *websocket.Conn) {
	for {
		messageType, data, err := src.ReadMessage()
		if err != nil {
			errCh <- err
			return
		}

		if err := dst.WriteMessage(messageType, data); err != nil {
			errCh <- err
			return
		}
	}
}

func getCoreControllerTarget(scheme string) (*url.URL, string, error) {
	content, err := os.ReadFile(GetPath("data/sing-box/config.json"))
	if err != nil {
		return nil, "", fmt.Errorf("core config not found: %w", err)
	}

	var config coreConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, "", fmt.Errorf("invalid core config: %w", err)
	}

	controller := strings.TrimSpace(config.Experimental.ClashAPI.ExternalController)
	if controller == "" {
		controller = "127.0.0.1:20123"
	}

	target, err := parseCoreControllerURL(controller, scheme)
	if err != nil {
		return nil, "", err
	}

	return target, config.Experimental.ClashAPI.Secret, nil
}

func parseCoreControllerURL(controller string, scheme string) (*url.URL, error) {
	if !strings.Contains(controller, "://") {
		controller = scheme + "://" + controller
	}

	target, err := url.Parse(controller)
	if err != nil {
		return nil, fmt.Errorf("invalid core controller address: %w", err)
	}

	host := target.Hostname()
	port := target.Port()

	if host == "" {
		host = "127.0.0.1"
	}

	switch host {
	case "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}

	if port == "" {
		port = "20123"
	}

	target.Scheme = scheme
	target.Host = net.JoinHostPort(host, port)

	return target, nil
}

func joinURLPath(basePath string, requestPath string) string {
	basePath = strings.TrimSuffix(basePath, "/")
	requestPath = "/" + strings.TrimPrefix(requestPath, "/")

	if basePath == "" {
		return requestPath
	}

	return basePath + requestPath
}

func copyHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
}

func writeWebProxyError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": message,
	})
}
