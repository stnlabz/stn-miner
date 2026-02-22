// File: main.go (The "Steady Heart" Fix)
// Version 2.2 - Anti-OOM / Anti-Kill

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
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
)

func main() {
	startTime = time.Now()
	
	// FIX: Use exactly 1 core. This prevents the "concurrency multiplier" 
	// that spikes memory usage and triggers the kernel kill.
	runtime.GOMAXPROCS(1)

	conn, err := net.Dial("tcp", "192.168.20.107:3333")
	if err != nil {
		return
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	encoder := json.NewEncoder(conn)

	// Subscribe
	encoder.Encode(map[string]interface{}{"method": "mining.subscribe", "params": []interface{}{}, "id": 1})

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		var msg map[string]interface{}
		json.Unmarshal([]byte(line), &msg)

		if msg["method"] == "mining.notify" {
			params := msg["params"].([]interface{})
			jobID := params[0].(string)
			prevHash := params[1].(string)

			// Solve loop
			go func() {
				var nonce int
				for {
					atomic.AddUint64(&hashesDone, 1)
					data := fmt.Sprintf("%s|%s|%d", jobID, prevHash, nonce)
					
					// FIX: Standard Argon2id call
					hash := argon2.IDKey([]byte(data), []byte("stn-salt"), 1, 64*1024, 1, 32)
					
					if strings.HasPrefix(fmt.Sprintf("%x", hash), "00000") {
						encoder.Encode(map[string]interface{}{
							"method": "mining.submit",
							"params": []interface{}{"pi-stable", jobID, nonce, fmt.Sprintf("%x", hash)},
							"id":     2,
						})
						atomic.AddUint64(&sharesAccepted, 1)
					}
					nonce++

					// THE CRITICAL FIX: 
					// A 1ms rest gives the Linux Garbage Collector time to breathe.
					// This prevents the "Signal: Killed" by keeping RAM usage flat.
					time.Sleep(1 * time.Millisecond)
				}
			}()
		}
	}
}
