// File: main.go (The Iron Suture)
// Version 2.3 - Restoring the Dashboard + RAM Lockdown

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"runtime"
	"runtime/debug" // Added for manual memory control
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/argon2"
)

var (
	sharesAccepted uint64
	hashesDone     uint64
	startTime      time.Time
	currentJob     string
)

type StratumMsg struct {
	Method string        `json:"method,omitempty"`
	Params []interface{} `json:"params,omitempty"`
	Id     int           `json:"id"`
	Result bool          `json:"result"`
}

func main() {
	startTime = time.Now()
	
	// FIX: Limit to 2 cores. Pi 5 can handle 2 easily if we manage the RAM.
	runtime.GOMAXPROCS(2)
	
	// Force the Garbage Collector to be 50% more aggressive
	debug.SetGCPercent(50)

	conn, err := net.Dial("tcp", "192.168.20.107:3333")
	if err != nil {
		fmt.Println("[!] Could not connect to .107. Is the Stratum service running?")
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
			params := msg.Params
			currentJob = params[0].(string)
			prevHash := params[1].(string)

			for i := 0; i < 2; i++ {
				go func(id string, prev string) {
					var nonce int
					for {
						atomic.AddUint64(&hashesDone, 1)
						data := fmt.Sprintf("%s|%s|%d", id, prev, nonce)
						
						// Argon2id - The "Sovereign" Standard
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

						// THE FIX: Every 10 hashes, clear the RAM and take a tiny breath.
						if nonce % 10 == 0 {
							runtime.GC() 
							time.Sleep(1 * time.Millisecond)
						}
					}
				}(currentJob, prevHash)
			}
		}
	}
}

func printDashboard() {
	for {
		time.Sleep(2 * time.Second)
		hps := float64(atomic.LoadUint64(&hashesDone)) / time.Since(startTime).Seconds()

		fmt.Print("\033[H\033[2J") // Clear screen
		fmt.Printf("STN-MINER | Workers: 2\n")
		fmt.Println("----------------------------------------------------------------")
		fmt.Printf(" Job ID:     %s\n", currentJob)
		fmt.Printf(" Hashrate:   %.2f H/s\n", hps)
		fmt.Printf(" Shares:     A:%d\n", atomic.LoadUint64(&sharesAccepted))
		fmt.Println("----------------------------------------------------------------")
		fmt.Println(" [Status] Monitoring memory pressure...")
	}
}
