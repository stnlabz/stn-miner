// File: main.go (Digits Pi - Patched)
// Version 1.8 - Optimized for A76 Memory-Hardness

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
	sharesAccepted uint64
	hashesDone     uint64
	startTime      time.Time
	currentJob     string
)

type StratumMsg struct {
	Method string        `json:"method,omitempty"`
	Params []interface{} `json:"params,omitempty"`
	Id     int           `json:"id"`
}

func main() {
	startTime = time.Now()
	// Set Go threads to match physical cores
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Patch: Point to the Stratum Pi (.107) instead of localhost
	conn, err := net.Dial("tcp", "192.168.20.107:3333")
	if err != nil {
		log.Fatal("[!] Stratumd not found on .107:3333")
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	encoder := json.NewEncoder(conn)

	// mining.subscribe
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
			jobID := msg.Params[0].(string)
			prevHash := msg.Params[1].(string)
			currentJob = jobID

			// Patch: 1 goroutine per core. Argon2 thread count set to 1.
			for i := 0; i < runtime.NumCPU(); i++ {
				go func(id string, prev string) {
					nonce, solution := solve(id, prev)
					submit := StratumMsg{
						Method: "mining.submit",
						Params: []interface{}{"digits-pi", id, nonce, solution},
						Id:     2,
					}
					encoder.Encode(submit)
					atomic.AddUint64(&sharesAccepted, 1)
				}(jobID, prevHash)
			}
		}
	}
}

func solve(jobID, prevHash string) (int, string) {
	var nonce int
	for {
		atomic.AddUint64(&hashesDone, 1)
		data := fmt.Sprintf("%s|%s|%d", jobID, prevHash, nonce)
		
		// Patch: 1 pass, 64MB, 1 thread (prevents bus saturation)
		hash := argon2.IDKey([]byte(data), []byte("stn-salt"), 1, 64*1024, 1, 32)
		result := fmt.Sprintf("%x", hash)

		// 5K Difficulty Check (Sovereign Threshold)
		if strings.HasPrefix(result, "00000") {
			return nonce, result
		}
		nonce++
	}
}

func printDashboard() {
	for {
		time.Sleep(2 * time.Second) // Reduce TUI refresh rate
		elapsed := time.Since(startTime).Seconds()
		hps := float64(atomic.LoadUint64(&hashesDone)) / elapsed

		fmt.Print("\033[H\033[2J") // Clear terminal
		fmt.Printf("STN-MINER | Workers: %d | Arch: %s\n", runtime.NumCPU(), runtime.GOARCH)
		fmt.Println("----------------------------------------------------------------")
		fmt.Printf(" Job ID:     %s\n", currentJob)
		fmt.Printf(" Uptime:     %v\n", time.Since(startTime).Round(time.Second))
		fmt.Printf(" Hashrate:   %.2f H/s (Argon2id-1p-64MB)\n", hps)
		fmt.Printf(" Shares:     A:%d\n", atomic.LoadUint64(&sharesAccepted))
		fmt.Println("----------------------------------------------------------------")
	}
}
