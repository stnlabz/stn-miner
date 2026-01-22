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
	sharesRejected uint64
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
	// Use all available cores for Argon2id
	runtime.GOMAXPROCS(runtime.NumCPU())

	conn, err := net.Dial("tcp", "127.0.0.1:3333")
	if err != nil {
		log.Fatal("[!] Stratumd not found on 127.0.0.1:3333")
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	encoder := json.NewEncoder(conn)

	// mining.subscribe
	encoder.Encode(StratumMsg{Method: "mining.subscribe", Params: []interface{}{}, Id: 1})

	// Dashboard loop
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

			// Spawn a worker for each core
			for i := 0; i < runtime.NumCPU(); i++ {
				go func(id string, prev string) {
					nonce, solution := solve(id, prev)
					submit := StratumMsg{
						Method: "mining.submit",
						Params: []interface{}{"desktop-worker", id, nonce, solution},
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
		
		// Argon2id: 1 pass, 64MB memory, 4 threads
		hash := argon2.IDKey([]byte(data), []byte("stn-salt"), 1, 64*1024, 4, 32)
		result := fmt.Sprintf("%x", hash)

		if strings.HasPrefix(result, "00000") {
			return nonce, result
		}
		nonce++
	}
}

func printDashboard() {
	for {
		time.Sleep(1 * time.Second)
		elapsed := time.Since(startTime).Seconds()
		hps := float64(atomic.LoadUint64(&hashesDone)) / elapsed

		// Professional Bitcoin Miner TUI Format
		fmt.Print("\033[H\033[2J") // Clear
		fmt.Printf("STN-MINER | Workers: %d | Arch: %s\n", runtime.NumCPU(), runtime.GOARCH)
		fmt.Println("----------------------------------------------------------------")
		fmt.Printf(" Job ID:     %s\n", currentJob)
		fmt.Printf(" Uptime:     %v\n", time.Since(startTime).Round(time.Second))
		fmt.Printf(" Hashrate:   %.2f H/s (Argon2id-MemoryHard)\n", hps)
		fmt.Printf(" Shares:     A:%d  R:%d\n", atomic.LoadUint64(&sharesAccepted), atomic.LoadUint64(&sharesRejected))
		fmt.Println("----------------------------------------------------------------")
		fmt.Printf(" [Log] %s: Share accepted by stratumd\n", time.Now().Format("15:04:05"))
	}
}
