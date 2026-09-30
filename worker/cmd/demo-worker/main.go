package main

import (
	"bufio"
	"csdemoreview/worker/internal/service"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
)

type request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func main() {
	dir := flag.String("data-dir", "", "local data directory")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "--data-dir is required")
		os.Exit(2)
	}
	var output sync.Mutex
	enc := json.NewEncoder(os.Stdout)
	emit := func(value any) {
		output.Lock()
		defer output.Unlock()
		if e := enc.Encode(value); e != nil {
			fmt.Fprintln(os.Stderr, e)
		}
	}
	s, e := service.New(*dir, func(event string, data any) { emit(map[string]any{"event": event, "data": data}) })
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	defer s.Close()
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 65536), 1<<20)
	var active sync.WaitGroup
	limit := make(chan struct{}, 16)
	for scanner.Scan() {
		var r request
		if e = json.Unmarshal(scanner.Bytes(), &r); e != nil {
			emit(map[string]any{"id": "", "error": map[string]string{"code": "BAD_REQUEST", "message": "Invalid JSON request"}})
			continue
		}
		if r.ID == "" || len(r.ID) > 128 {
			emit(map[string]any{"id": r.ID, "error": map[string]string{"code": "BAD_REQUEST", "message": "Request ID required (max 128 characters)"}})
			continue
		}
		limit <- struct{}{}
		active.Add(1)
		go func(r request) {
			defer active.Done()
			defer func() {
				<-limit
				if p := recover(); p != nil {
					emit(map[string]any{"id": r.ID, "error": map[string]string{"code": "INTERNAL_ERROR", "message": fmt.Sprintf("Request failed safely: %v", p)}})
				}
			}()
			result, e := s.Call(r.Method, r.Params)
			if e != nil {
				emit(map[string]any{"id": r.ID, "error": map[string]string{"code": "REQUEST_FAILED", "message": e.Error()}})
			} else {
				emit(map[string]any{"id": r.ID, "result": result})
			}
		}(r)
	}
	s.Close()
	active.Wait()
	if scanner.Err() != nil {
		fmt.Fprintln(os.Stderr, scanner.Err())
	}
}
