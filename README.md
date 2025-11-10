# goDistributedSystemDemo


Build the view service:
```go build -o ./bin/viewServer ./view/view_server.go```

Run the View service:
```./bin/viewServer [args]```

View service execution args:

    ./bin/viewServer \
	    -addr		- address for the view server, localhost:8000 (default)



Build the kv server:
```go build -o ./bin/kvServer ./kv_server_main/kv_server_main.go```

Run the kv server:
```./bin/kvServer [args]```

KV server execution ags:

    ./bin/kvServer \
	    -vs				- address of the view service, localhost:8000 (default)
	    -addr			- address of the server(kv server), localhost:8001 (default)
  

Build the client:
```go build -o ./bin/client ./client_main/client.go```

Run the client:
```./bin/client [args]```

KV server execution ags:

    ./bin/client \
	    -vs			- address of the view service, localhost:8000 (default)
	    -op			- operation "put" or "get"
	    -key		- key of the operation
	    -value		- value of the key
	    -ops		- "op1, op2, op3", ops of sequence
	    -keys		- "key1, key2, key3", keys of the sequence of operations
	    -values		- "value1, value2, value3", values of the sequence of operations

Following should be the squence to deploy:

    #In terminal #1
    ./bin/viewServer 
    
    #In terminal #2
    ./bin/kvServer
     
    #In terminal #3
    ./bin/client -ops "put,get" -keys "key1,key1" -values "1,x"


## Run the automated test (Report Requirement)
run the following command:
```./auto_test.sh | tee ./log/logs.txt```

review and verify the logs.txt
