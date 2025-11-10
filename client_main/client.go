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
package client_main

import (
	"flag"
	"fmt"
	"time"

	"goDistributedSystemDemo/client_main/client"
)

func main() {
	vsAddr := flag.String("vs", "localhost:8000", "View service address (host:port)")
	clientOp := flag.String("op", "get", "Client operation: get or put")
	key := flag.String("key", "foo", "Key for get/put operation")
	value := flag.String("value", "bar", "Value for put operation")

	flag.Parse()

	fmt.Printf("Starting test client\n")
	fmt.Printf("View Service at %s\n", *vsAddr)

	ck := client.MakeClient(*vsAddr)
	defer ck.Close()

	//retry 5 times to connect to primary
	i := 0
	for next := true; next; next = i < 5 {
		ck.updatePrimary()
		if ck.currentPrimary != "" {
			break
		}
		time.Sleep(1 * time.Second)
		i++
	}

	fmt.Printf("Connected to primary server at %s\n", ck.currentPrimary)

	if *clientOp == "get" {
		val := ck.Get(*key)
		fmt.Printf("Get(%s) = %s\n", *key, val)
	} else if *clientOp == "put" {
		ck.Put(*key, *value)
		fmt.Printf("Put(%s, %s) completed\n", *key, *value)
	} else {
		fmt.Printf("Unknown client operation: %s\n", *clientOp)
	}

}
