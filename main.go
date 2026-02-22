// File: main.go (The Emergency Brake)
// Version 3.1 - Zero-Concurrency Sequential Mining

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
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
)

type StratumMsg struct {
	Method string        `json:"method,omitempty"`
	Params []interface{} `json:"params,omitempty"`
	Id     int           `json:"id"`
	Result bool          `json:"result"`
}

func main() {
	startTime = time.Now()
	
	// FORCE the runtime to be as small as possible
	runtime.GOMAXPROCS(1)
	debug.SetMemoryLimit(400 * 1024 * 1024) // Hard 400MB ceiling
	debug.SetGCPercent(5) 

	conn, err := net.Dial("tcp", "192.168.20.107:3333")
	if err != nil {
		return
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	encoder := json.NewEncoder(conn)
	encoder.Encode(StratumMsg{Method: "mining.subscribe", Id: 1})

	go printDashboard()

	var nonce int
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		var msg StratumMsg
		json.Unmarshal([]byte(line), &msg)

		if msg.Method == "mining.notify" {
			currentJob = msg.Params[0].(string)
			prevHash := msg.Params[1].(string)

			// SEQUENTIAL MINING: No "go" keyword here.
			// This prevents multiple 64MB blocks from existing at once.
			for k := 0; k < 10; k++ { // Process 10 hashes per network check
				atomic.AddUint64(&hashesDone, 1)
				data := fmt.Sprintf("%s|%s|%d", currentJob, prevHash, nonce)
				
				// Argon2 Execution
				hash := argon2.IDKey([]byte(data), []byte("stn-salt"), 1, 64*1024, 1, 32)
				result := fmt.Sprintf("%x", hash)

				if strings.HasPrefix(result, "00000") {
					submit := StratumMsg{
						Method: "mining.submit",
						Params: []interface{}{"pi-suture", currentJob, nonce, result},
						Id:     2,
					}
					encoder.Encode(submit)
					atomic.AddUint64(&sharesAccepted, 1)
				}
				nonce++

				// THE PURGE: Clean up every single hash
				hash = nil
				runtime.GC()
				debug.FreeOSMemory()
			}
		}
		
		if msg.Id == 2 && msg.Result == true {
			atomic.AddUint64(&sharesConfirmed, 1)
		}
	}
}

func printDashboard() {
	var m runtime.MemStats
	for {
		time.Sleep(1 * time.Second)
		runtime.ReadMemStats(&m)
		hps := float64(atomic.LoadUint64(&hashesDone)) / time.Since(startTime).Seconds()

		fmt.Print("\033[H\033[2J")
		fmt.Printf("STN-MINER | M.R. | RAM: %d MiB\n", m.Alloc/1024/1024)
		fmt.Println("----------------------------------------------------------------")
		fmt.Printf(" Rate: %.2f H/s | Target: Index 2 | Sys: %d MiB\n", hps, m.Sys/1024/1024)
		fmt.Printf(" Shares: A:%d  C:%d\n", atomic.LoadUint64(&sharesAccepted), atomic.LoadUint64(&sharesConfirmed))
		fmt.Println("----------------------------------------------------------------")
	}
}
