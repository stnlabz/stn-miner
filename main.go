// File: main.go (The Confirmed Iron Suture)
// Version 2.4 - Dashboard + Confirmation Tracker

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
	Result bool          `json:"result"` // The "C" trigger
}

func main() {
	startTime = time.Now()
	runtime.GOMAXPROCS(2)
	debug.SetGCPercent(40) // Even more aggressive cleanup

	conn, err := net.Dial("tcp", "192.168.20.107:3333")
	if err != nil {
		fmt.Println("[!] Failed to connect to Stratum gateway.")
		return
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	encoder := json.NewEncoder(conn)

	// Subscribe
	encoder.Encode(StratumMsg{Method: "mining.subscribe", Params: []interface{}{}, Id: 1})

	go printDashboard()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		var msg StratumMsg
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		// Handle job from Stratum
		if msg.Method == "mining.notify" {
			currentJob = msg.Params[0].(string)
			prevHash := msg.Params[1].(string)

			for i := 0; i < 2; i++ {
				go func(id string, prev string) {
					var nonce int
					for {
						atomic.AddUint64(&hashesDone, 1)
						data := fmt.Sprintf("%s|%s|%d", id, prev, nonce)
						
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

						// Memory stabilization: Prevents the "Killed" signal
						if nonce % 20 == 0 {
							runtime.GC()
						}
					}
				}(currentJob, prevHash)
			}
		}

		// THE TRACKER: Catch the Result from the Master
		if msg.Id == 2 && msg.Result == true {
			atomic.AddUint64(&sharesConfirmed, 1)
		}
	}
}

func printDashboard() {
	for {
		time.Sleep(1 * time.Second)
		hps := float64(atomic.LoadUint64(&hashesDone)) / time.Since(startTime).Seconds()

		fmt.Print("\033[H\033[2J") // Clear screen
		fmt.Printf("STN-MINER | Madam M.R. Sovereign Node | Workers: 2\n")
		fmt.Println("----------------------------------------------------------------")
		fmt.Printf(" Job ID:     %s\n", currentJob)
		fmt.Printf(" Hashrate:   %.2f H/s\n", hps)
		fmt.Printf(" Shares:     A:%d (Accepted) | C:%d (Confirmed)\n", 
			atomic.LoadUint64(&sharesAccepted), 
			atomic.LoadUint64(&sharesConfirmed))
		fmt.Println("----------------------------------------------------------------")
		if atomic.LoadUint64(&sharesConfirmed) > 0 {
			fmt.Println(" [!] SUCCESS: BLOCK VERIFIED ON INDEX 2")
		}
	}
}
