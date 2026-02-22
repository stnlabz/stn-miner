// File: main.go (The Sovereign Suture - Zero-Waste)
// Version 3.0 - Manual Buffer Reuse

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
	runtime.GOMAXPROCS(1) // Absolute limit for 4GB Pi

	// Hard memory clamp: 512MB. If it hits this, it dies or cleans.
	debug.SetMemoryLimit(512 * 1024 * 1024)

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
					
					// FIXED: We call the keyer but immediately follow with 
					// a forceful cleanup to prevent the "640MiB" creep.
					hash := argon2.IDKey([]byte(data), []byte("stn-salt"), 1, 64*1024, 1, 32)
					result := fmt.Sprintf("%x", hash)

					if strings.HasPrefix(result, "00000") {
						submit := StratumMsg{
							Method: "mining.submit",
							Params: []interface{}{"pi-4gb", id, nonce, result},
							Id:     2,
						}
						encoder.Encode(submit)
						atomic.AddUint64(&sharesAccepted, 1)
					}
					nonce++

					// THE RECLAIMER:
					// Instead of waiting, we zero out the hash and force 
					// the OS to take the memory back EVERY single time.
					hash = nil 
					if nonce % 1 == 0 {
						runtime.GC()
						debug.FreeOSMemory()
					}
					
					// Pacing: prevents the RAM bus from saturating 
					// which is likely what triggered the "Killed" at 640MB.
					time.Sleep(100 * time.Millisecond) 
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
		fmt.Printf("STN-MINER | M.R. | RAM: %d MiB\n", m.Alloc/1024/1024)
		fmt.Println("----------------------------------------------------------------")
		fmt.Printf(" Job: %s\n", currentJob)
		fmt.Printf(" Rate: %.2f H/s | Confirmations: %d\n", hps, atomic.LoadUint64(&sharesConfirmed))
		fmt.Printf(" Shares Accepted: %d\n", atomic.LoadUint64(&sharesAccepted))
		fmt.Println("----------------------------------------------------------------")
		if m.Alloc/1024/1024 < 200 {
			fmt.Println(" [STABLE] Memory within Sovereign tolerances.")
		}
	}
}
