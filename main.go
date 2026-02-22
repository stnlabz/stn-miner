// File: main.go (PC - Safe Muscle Patch)
// Version 2.1 - Stabilized for x86

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"runtime"
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
	
	// PATCH: Leave 2 cores free for the OS to prevent lock-up
	numCores := runtime.NumCPU()
	if numCores > 2 {
		numCores = numCores - 2
	}
	runtime.GOMAXPROCS(numCores)

	// Point to the Stratum Pi (.107)
	conn, err := net.Dial("tcp", "192.168.20.107:3333")
	if err != nil {
		log.Fatal("[!] Stratum Gateway (.107) not found.")
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	encoder := json.NewEncoder(conn)

	// Subscribe
	encoder.Encode(StratumMsg{Method: "mining.subscribe", Params: []interface{}{}, Id: 1})

	go printDashboard(numCores)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		var msg StratumMsg
		json.Unmarshal([]byte(line), &msg)

		if msg.Method == "mining.notify" {
			jobID, _ := msg.Params[0].(string)
			prevHash, _ := msg.Params[1].(string)
			currentJob = jobID

			// Spawn workers based on the 'safe' core count
			for i := 0; i < numCores; i++ {
				go func(id string, prev string) {
					nonce, solution := solve(id, prev)
					submit := StratumMsg{
						Method: "mining.submit",
						Params: []interface{}{"pc-worker", id, nonce, solution},
						Id:     2,
					}
					encoder.Encode(submit)
					atomic.AddUint64(&sharesAccepted, 1)
				}(jobID, prevHash)
			}
		}

		// Catch the confirmation from the .106 Master via .107
		if msg.Id == 2 && msg.Result == true {
			atomic.AddUint64(&sharesConfirmed, 1)
		}
	}
}

func solve(jobID, prevHash string) (int, string) {
	// Offset the PC nonce so it doesn't overlap with the Pi's old work
	var nonce int = 5000000 
	for {
		atomic.AddUint64(&hashesDone, 1)
		data := fmt.Sprintf("%s|%s|%d", jobID, prevHash, nonce)
		
		// 1 pass, 64MB, 1 thread
		hash := argon2.IDKey([]byte(data), []byte("stn-salt"), 1, 64*1024, 1, 32)
		result := fmt.Sprintf("%x", hash)

		if strings.HasPrefix(result, "00000") {
			return nonce, result
		}
		nonce++
		
		// Micro-throttle to keep the system responsive
		if nonce % 500 == 0 {
			time.Sleep(1 * time.Microsecond)
		}
	}
}

func printDashboard(cores int) {
	for {
		time.Sleep(1 * time.Second)
		elapsed := time.Since(startTime).Seconds()
		hps := float64(atomic.LoadUint64(&hashesDone)) / elapsed

		fmt.Print("\033[H\033[2J") // Clear screen
		fmt.Printf("STN-MINER | Workers: %d Cores\n", cores)
		fmt.Println("----------------------------------------------------------------")
		fmt.Printf(" Job: %s\n", currentJob)
		fmt.Printf(" Rate: %.2f H/s | Uptime: %v\n", hps, time.Since(startTime).Round(time.Second))
		fmt.Printf(" Shares: A:%d  C:%d\n", atomic.LoadUint64(&sharesAccepted), atomic.LoadUint64(&sharesConfirmed))
		fmt.Println("----------------------------------------------------------------")
		if atomic.LoadUint64(&sharesConfirmed) > 0 {
			fmt.Println(" [!] BLOCK SEALED: Index 2 is LIVE.")
		}
	}
}
