package bridge

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path"
	"reflect"
	"strings"
)

const DefaultWebListenAddress = "127.0.0.1:18080"

type rpcRequest struct {
	Args []json.RawMessage `json:"args"`
}

type webEmitRequest struct {
	Event string `json:"event"`
	Args  []any  `json:"args"`
}

func ParseWebMode(args []string) (enabled bool, listenAddr string, err error) {
	listenAddr = DefaultWebListenAddress

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "web" || arg == "--web":
			enabled = true
		case arg == "--listen":
			if i+1 >= len(args) {
				return false, listenAddr, errors.New("missing value for --listen")
			}
			listenAddr = args[i+1]
			i += 1
		case strings.HasPrefix(arg, "--listen="):
			listenAddr = strings.TrimPrefix(arg, "--listen=")
		}
	}

	return enabled, listenAddr, nil
}

func RunWebServer(app *App, assets embed.FS, listenAddr string) error {
	app.Ctx = context.Background()

	distFS, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/__web/rpc/", app.handleRPC)
	mux.HandleFunc("/__web/events/stream", handleWebEventStream)
	mux.HandleFunc("/__web/events/emit", handleWebEventEmit)
	mux.HandleFunc("/__core/", app.handleCoreProxy)
	mux.Handle("/", createFrontendHandler(distFS))

	server := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	log.Printf("Web panel listening on http://%s", listenAddr)
	return server.ListenAndServe()
}

func (a *App) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	methodName := strings.TrimPrefix(r.URL.Path, "/__web/rpc/")
	if methodName == "" {
		http.Error(w, "method name is required", http.StatusBadRequest)
		return
	}

	var payload rpcRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	result, err := invokeAppMethod(a, methodName, payload.Args)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func invokeAppMethod(app *App, methodName string, args []json.RawMessage) (result any, err error) {
	method := reflect.ValueOf(app).MethodByName(methodName)
	if !method.IsValid() {
		return nil, fmt.Errorf("method not found: %s", methodName)
	}

	methodType := method.Type()
	if methodType.NumIn() != len(args) {
		return nil, fmt.Errorf(
			"method %s expects %d args but got %d",
			methodName,
			methodType.NumIn(),
			len(args),
		)
	}

	inputs := make([]reflect.Value, methodType.NumIn())
	for i := 0; i < methodType.NumIn(); i++ {
		argType := methodType.In(i)
		value := reflect.New(argType)
		if err := json.Unmarshal(args[i], value.Interface()); err != nil {
			return nil, fmt.Errorf("decode arg %d for %s: %w", i+1, methodName, err)
		}
		inputs[i] = value.Elem()
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic invoking %s: %v", methodName, recovered)
		}
	}()

	outputs := method.Call(inputs)
	if len(outputs) == 0 {
		return nil, nil
	}

	return outputs[0].Interface(), nil
}

func handleWebEventStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	_, events, removeClient := webEventBus.AddClient()
	defer removeClient()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}

			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}

			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func handleWebEventEmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload webEmitRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if payload.Event == "" {
		http.Error(w, "event name is required", http.StatusBadRequest)
		return
	}

	webEventBus.EmitLocal(payload.Event, payload.Args...)
	w.WriteHeader(http.StatusNoContent)
}

func createFrontendHandler(distFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath := path.Clean("/" + r.URL.Path)
		if requestPath == "/" {
			http.ServeFileFS(w, r, distFS, "index.html")
			return
		}

		trimmedPath := strings.TrimPrefix(requestPath, "/")
		if strings.Contains(path.Base(trimmedPath), ".") {
			if _, err := fs.Stat(distFS, trimmedPath); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		http.ServeFileFS(w, r, distFS, "index.html")
	})
}
