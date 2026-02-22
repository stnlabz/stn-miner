// File: main.go (Forensic Iron Suture)
// Version 2.5 - Full Logging + Memory Tracking

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/argon2"
)

var (
	sharesAccepted  uint64
	sharesConfirmed uint64
	hashesDone      uint64
	startTime       time.Time
	currentJob      string
	logFile         *os.File
)

type StratumMsg struct {
	Method string        `json:"method,omitempty"`
	Params []interface{} `json:"params,omitempty"`
	Id     int           `json:"id"`
	Result bool          `json:"result"`
}

func main() {
	startTime = time.Now()
	runtime.GOMAXPROCS(2)
	debug.SetGCPercent(30) // Maximum aggression on RAM cleanup

	// Setup Logger
	var err error
	logFile, err = os.OpenFile("miner.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Could not create log file")
	}
	defer logFile.Close()
	logger := log.New(logFile, "[STN] ", log.LstdFlags)

	logger.Println("--- Miner Starting ---")
	logger.Printf("Arch: %s | Cores: %d", runtime.GOARCH, runtime.NumCPU())

	conn, err := net.Dial("tcp", "192.168.20.107:3333")
	if err != nil {
		logger.Printf("Connection Error: %v", err)
		return
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	encoder := json.NewEncoder(conn)

	encoder.Encode(StratumMsg{Method: "mining.subscribe", Params: []interface{}{}, Id: 1})

	go printDashboard(logger)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			logger.Printf("Read Error: %v", err)
			break
		}

		var msg StratumMsg
		json.Unmarshal([]byte(line), &msg)

		if msg.Method == "mining.notify" {
			currentJob = msg.Params[0].(string)
			prevHash := msg.Params[1].(string)
			logger.Printf("New Job Received: %s", currentJob)

			for i := 0; i < 2; i++ {
				go func(id string, prev string) {
					var nonce int
					for {
						atomic.AddUint64(&hashesDone, 1)
						data := fmt.Sprintf("%s|%s|%d", id, prev, nonce)
						
						hash := argon2.IDKey([]byte(data), []byte("stn-salt"), 1, 64*1024, 1, 32)
						result := fmt.Sprintf("%x", hash)

						if strings.HasPrefix(result, "00000") {
							logger.Printf("Found Solution! Nonce: %d", nonce)
							submit := StratumMsg{
								Method: "mining.submit",
								Params: []interface{}{"pi-iron", id, nonce, result},
								Id:     2,
							}
							encoder.Encode(submit)
							atomic.AddUint64(&sharesAccepted, 1)
						}
						nonce++

						if nonce % 15 == 0 {
							runtime.GC()
						}
					}
				}(currentJob, prevHash)
			}
		}

		if msg.Id == 2 && msg.Result == true {
			atomic.AddUint64(&sharesConfirmed, 1)
			logger.Println("Share CONFIRMED by Master.")
		}
	}
}

func printDashboard(logger *log.Logger) {
	var m runtime.MemStats
	for {
		time.Sleep(2 * time.Second)
		runtime.ReadMemStats(&m)
		hps := float64(atomic.LoadUint64(&hashesDone)) / time.Since(startTime).Seconds()

		// Log memory status to the file for forensic analysis
		logger.Printf("STATS: HPS: %.2f | Alloc: %v MiB | Sys: %v MiB | NumGC: %v", 
			hps, m.Alloc/1024/1024, m.Sys/1024/1024, m.NumGC)

		fmt.Print("\033[H\033[2J")
		fmt.Printf("STN-MINER | Workers: 2\n")
		fmt.Println("----------------------------------------------------------------")
		fmt.Printf(" Job ID:     %s\n", currentJob)
		fmt.Printf(" Hashrate:   %.2f H/s\n", hps)
		fmt.Printf(" Mem Alloc:  %v MiB\n", m.Alloc/1024/1024)
		fmt.Printf(" Shares:     A:%d  C:%d\n", 
			atomic.LoadUint64(&sharesAccepted), 
			atomic.LoadUint64(&sharesConfirmed))
		fmt.Println("----------------------------------------------------------------")
		fmt.Println(" [Log] Writing forensic data to miner.log...")
	}
}
