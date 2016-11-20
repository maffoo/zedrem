package main

import (
        "bytes"
        "flag"
        "fmt"
        "net/http"
        "path/filepath"
        "strings"
)


type LocalWebFSHandler struct {
        handler RPCHandler
}

func (self *LocalWebFSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
        defer r.Body.Close()

        reqPath := r.URL.Path
        if strings.HasPrefix(reqPath, "/") {
                reqPath = reqPath[1:]
        }

        reqChan := make(chan []byte)
        repChan := make(chan []byte)
        closeChan := make(chan bool)

        go self.handler.handleRequest(reqChan, repChan, closeChan)

        defer quietPanicRecover()

        // First send request line
        requestLine := fmt.Sprintf("%s %s", r.Method, reqPath)
        fmt.Println(requestLine)
        reqChan <- []byte(requestLine)
        // Then send headers
        var headerBuffer bytes.Buffer
        for h, v := range r.Header {
                headerBuffer.Write([]byte(fmt.Sprintf("%s: %s\n", h, v)))
        }
        reqChan <- headerBuffer.Bytes()

        // Send body
        for {
                buffer := make([]byte, BUFFER_SIZE)
                n, _ := r.Body.Read(buffer)
                if n == 0 {
                        break
                }
                reqChan <- buffer[:n]
        }
        reqChan <- DELIMITERBUFFER
        statusCodeBuffer, ok := <- repChan
        if !ok {
                http.Error(w, "Connection closed", http.StatusInternalServerError)
                return
        }
        statusCode := BytesToInt(statusCodeBuffer)
        headersBuffer, ok := <- repChan
        if !ok {
                http.Error(w, "Connection close", http.StatusInternalServerError)
                return
        }
        headers := strings.Split(string(headersBuffer), "\n")
        for _, header := range headers {
                headerParts := strings.Split(header, ": ")
                w.Header().Set(headerParts[0], headerParts[1])
        }
        w.WriteHeader(statusCode)

        for {
                buffer, ok := <- repChan
                if !ok {
                        w.Write([]byte("Connection closed"))
                        break
                }
                if IsDelimiter(buffer) {
                        break
                }
                _, err := w.Write(buffer)
                if err != nil {
                        fmt.Println("Got error", err)
                        break
                }
        }
}

func ParseLocalFlags(args []string) (ip string, port int, rootPath string) {
        config := ParseConfig()

        var stats bool
        flagSet := flag.NewFlagSet("zedrem", flag.ExitOnError)
        flagSet.StringVar(&ip, "h", config.Server.Ip, "IP to bind to")
        flagSet.IntVar(&port, "p", config.Server.Port, "Port to listen on")
        flagSet.BoolVar(&stats, "stats", false, "Whether to print go-routine count and memory usage stats periodically.")
        flagSet.Parse(args)
        if stats {
                go PrintStats()
        }
        rootPath = "."
        if flagSet.NArg() > 0 {
                rootPath = args[len(args)-1]
        }
        return
}

func RunLocal(ip string, port int, rootPath string) {
        rootPath, _ = filepath.Abs(rootPath)

        url := fmt.Sprintf("http://%s:%d", ip, port)
        fmt.Printf("Zedrem server running on %s, rootPath=%s\n", url, rootPath)
        fmt.Println("Press Ctrl-c to quit.")

        handler := RootedRPCHandler{rootPath}
        http.Handle("/", &LocalWebFSHandler{&handler})
        http.ListenAndServe(fmt.Sprintf("%s:%d", ip, port), nil)
}
