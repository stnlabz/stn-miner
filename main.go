// File: main.go (4GB Force Stabilization)
// Version 2.8 - Manual Heap Eviction

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
	
	// Single core is non-negotiable for 4GB stability
	runtime.GOMAXPROCS(1) 
	
	// Set GC to be extremely aggressive
	debug.SetGCPercent(5) 

	conn, err := net.Dial("tcp", "192.168.20.107:3333")
	if err != nil {
		return
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	encoder := json.NewEncoder(conn)
	encoder.Encode(StratumMsg{Method: "mining.subscribe", Params: []interface{}{}, Id: 1})

	go printDashboard()

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

			go func(id string, prev string) {
				var nonce int
				for {
					atomic.AddUint64(&hashesDone, 1)
					data := fmt.Sprintf("%s|%s|%d", id, prev, nonce)
					
					// Argon2id Computation
					hash := argon2.IDKey([]byte(data), []byte("stn-salt"), 1, 64*1024, 1, 32)
					result := fmt.Sprintf("%x", hash)

					if strings.HasPrefix(result, "00000") {
						submit := StratumMsg{
							Method: "mining.submit",
							Params: []interface{}{"pi-iron", id, nonce, result},
							Id:     2,
						}
						encoder.Encode(submit)
						atomic.AddUint64(&sharesAccepted, 1)
					}
					nonce++

					// THE NUCLEAR OPTION: Force the memory back to Linux
					if nonce % 5 == 0 {
						runtime.GC()
						debug.FreeOSMemory() 
						// Give the kernel 25ms to actually reclaim the pages
						time.Sleep(25 * time.Millisecond) 
					}
				}
			}(currentJob, prevHash)
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
		fmt.Printf("STN-MINER | M.R. | 4GB Force-Release | RAM: %d MiB\n", m.Alloc/1024/1024)
		fmt.Println("----------------------------------------------------------------")
		fmt.Printf(" Rate: %.2f H/s | System RAM (Sys): %d MiB\n", hps, m.Sys/1024/1024)
		fmt.Printf(" Shares: A:%d  C:%d\n", atomic.LoadUint64(&sharesAccepted), atomic.LoadUint64(&sharesConfirmed))
		fmt.Println("----------------------------------------------------------------")
		if m.Alloc/1024/1024 > 800 {
			fmt.Println(" [!] RECLAIMING: Forcing OS Memory Release...")
		}
	}
}
