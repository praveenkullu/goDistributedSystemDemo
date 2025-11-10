/*
 *
 * Copyright 2015 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

// Package main implements a client for Greeter service.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"goDistributedSystemDemo/client_main/client"
)

func main() {
	vsAddr := flag.String("vs", "localhost:8000", "View service address (host:port)")
	clientOp := flag.String("op", "put", "Client operation: get or put (single-op fallback)")
	key := flag.String("key", "foo", "Default key for get/put operation")
	value := flag.String("value", "bar", "Default value for put operation")

	// Sequence flags: comma-separated lists. If provided, -ops drives the sequence.
	// -ops: comma-separated operations, e.g. get,put,get
	// -keys: comma-separated keys corresponding to ops (optional; falls back to -key)
	// -values: comma-separated values for put ops (optional; falls back to -value)
	opsStr := flag.String("ops", "", "Comma-separated operations sequence, e.g. get,put")
	keysStr := flag.String("keys", "", "Comma-separated keys corresponding to ops (optional)")
	valuesStr := flag.String("values", "", "Comma-separated values for put ops (optional)")

	flag.Parse()

	fmt.Printf("Starting test client\n")
	fmt.Printf("View Service at %s\n", *vsAddr)
	pid := os.Getpid()
	fmt.Printf("PID: %d\n", pid)

	ck := client.MakeClient(*vsAddr)
	defer ck.Close()

	//retry indefinitely until we connect to primary
	for ck.CurrentPrimary == "" {
		ck.UpdatePrimary()
		if ck.CurrentPrimary == "" {
			fmt.Printf("Waiting for primary server...\n")
			time.Sleep(1 * time.Second)
		}
	}

	fmt.Printf("Connected to primary server at %s\n", ck.CurrentPrimary)

	// Helper to split comma-separated lists into trimmed slices (ignores empty entries)
	splitTrim := func(s string) []string {
		if s == "" {
			return []string{}
		}
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			t := strings.TrimSpace(p)
			if t != "" {
				out = append(out, t)
			}
			// fmt.Printf("Debug: splitTrim part='%s' trimmed='%s'\n", p, t) //--- IGNORE ---
		}
		return out
	}

	ops := splitTrim(*opsStr)
	keys := splitTrim(*keysStr)
	values := splitTrim(*valuesStr)

	// If no sequence provided, fall back to single-op behavior using -op
	if len(ops) == 0 {
		ops = []string{*clientOp}
		values = []string{*value}
		keys = []string{*key}
	}

	// Execute the sequence in order. For each position i:
	// - key to use: keys[i] (if present) else -key
	// - value to use for put: values[i] (if present) else -value
	for i, op := range ops {
		if op == "get" {
			val := ck.Get(keys[i])
			fmt.Printf("Get(%s) = %s\n", keys[i], val)
		} else if op == "put" {
			ck.Put(keys[i], values[i])
			fmt.Printf("Put(%s, %s) completed\n", keys[i], values[i])
		} else {
			fmt.Printf("Unknown client operation: %s\n", op)
		}
	}
}
