// File: main.go (Digits Pi - Stabilization Patch)
// Version 1.9 - 2-Core Pinning

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
	
	// Hard-pin to 2 cores. No more, no less.
	runtime.GOMAXPROCS(2)

	conn, err := net.Dial("tcp", "192.168.20.107:3333")
	if err != nil {
		log.Fatal("[!] Stratumd not found on .107:3333")
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
			jobID := msg.Params[0].(string)
			prevHash := msg.Params[1].(string)
			currentJob = jobID

			// Spawn exactly 2 workers for the 2 pinned cores
			for i := 0; i < 2; i++ {
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
		
		// 1 pass, 64MB, 1 thread. 
		// Combined with 2 workers, this uses ~128MB RAM total.
		hash := argon2.IDKey([]byte(data), []byte("stn-salt"), 1, 64*1024, 1, 32)
		result := fmt.Sprintf("%x", hash)

		if strings.HasPrefix(result, "00000") {
			return nonce, result
		}
		nonce++
	}
}

func printDashboard() {
	for {
		time.Sleep(2 * time.Second)
		elapsed := time.Since(startTime).Seconds()
		hps := float64(atomic.LoadUint64(&hashesDone)) / elapsed

		fmt.Print("\033[H\033[2J")
		fmt.Printf("STN-MINER | Workers: 2 (PINNED) | Arch: %s\n", runtime.GOARCH)
		fmt.Println("----------------------------------------------------------------")
		fmt.Printf(" Job ID:     %s\n", currentJob)
		fmt.Printf(" Hashrate:   %.2f H/s (Argon2id-Stable)\n", hps)
		fmt.Printf(" Shares:     A:%d\n", atomic.LoadUint64(&sharesAccepted))
		fmt.Println("----------------------------------------------------------------")
	}
}
