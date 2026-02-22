// File: main.go (4GB Pi Stabilization)
// Version 2.7 - Low-Memory Lockdown

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
	
	// CRITICAL: On a 4GB Pi, 1 worker is the only way to keep Alloc under 500MB.
	runtime.GOMAXPROCS(1) 
	debug.SetGCPercent(10) // Hyper-aggressive cleanup

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

			// Single worker goroutine to respect the 4GB RAM limit
			go func(id string, prev string) {
				var nonce int
				for {
					atomic.AddUint64(&hashesDone, 1)
					data := fmt.Sprintf("%s|%s|%d", id, prev, nonce)
					
					// 1 pass, 64MB, 1 thread
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

					// Forced cleanup every hash
					runtime.GC()
					// Small breather to allow the OS to catch up
					time.Sleep(10 * time.Millisecond)
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
		fmt.Printf("STN-MINER | Madam M.R. | 4GB Pi Fix | RAM: %d MiB\n", m.Alloc/1024/1024)
		fmt.Println("----------------------------------------------------------------")
		fmt.Printf(" Job: %s\n", currentJob)
		fmt.Printf(" Rate: %.2f H/s | Target: Index 2\n", hps)
		fmt.Printf(" Shares: A:%d  C:%d\n", atomic.LoadUint64(&sharesAccepted), atomic.LoadUint64(&sharesConfirmed))
		fmt.Println("----------------------------------------------------------------")
		if m.Alloc/1024/1024 > 1000 {
			fmt.Println(" [!] WARNING: Memory pressure rising.")
		}
	}
}
