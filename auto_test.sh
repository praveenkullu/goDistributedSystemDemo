TRANSFER_TIME=2
WAIT_TIME=2
TERMINAL_WIDTH=$(tput cols)

touch ./log/logs.txt
echo "Starting automated test..." > ./log/logs.txt

# start the view service
go run view/view_server.go &
VS_PID=$!

printf "%${TERMINAL_WIDTH}s\n" "Started View Service with PID $VS_PID"
# echo "Started View Service with PID $VS_PID"

sleep $WAIT_TIME

# start a kv server 1
go run kv_server_main/kv_server_main.go -addr="localhost:8001" -vs="localhost:8000" &
KV1_PID=$!
echo "Started KV Server 1 with PID $KV1_PID"


# start the client and make a put operation A: 1
sleep $WAIT_TIME
go run client_main/client.go -vs="localhost:8000" -ops="put,get" -keys="a,a" -values="1,x"  

sleep $WAIT_TIME
# start another kv server
go run kv_server_main/kv_server_main.go -addr="localhost:8002" -vs="localhost:8000"  &
KV2_PID=$!
echo "Started KV Server 2 with PID $KV2_PID"

# start the client and make a put operation B: 2
sleep $WAIT_TIME
go run client_main/client.go -vs="localhost:8000" -op="put,get" -keys="b,b" -values="2,x"  

# kill kv server 1
sleep $WAIT_TIME
kill $KV1_PID
echo "Killed KV Server 1 with PID $KV1_PID"

# client get operation for key A
go run client_main/client.go -vs="localhost:8000" -op="get" -key="a" 

# start server 3
go run kv_server_main/kv_server_main.go -addr="localhost:8003" -vs="localhost:8000"  &
KV3_PID=$!
echo "Started KV Server 3 with PID $KV3_PID"
#wait for state transfer
sleep $TRANSFER_TIME

# kill kv server 2
kill $KV2_PID
echo "Killed KV Server 2 with PID $KV2_PID"

# client get operation for key A
sleep $WAIT_TIME
go run client_main/client.go -vs="localhost:8000" -op="get" -key="a" 

#cleanup
echo "Cleaning up..."
kill $KV3_PID
kill $VS_PID
# In case any process is still running on port 8000, 8001, 8002, or 8003, kill them
# kill -9 $(sudo lsof -t -i:8000)

echo "Starting Auto verification..."

grep "Get(a) = 1" ./log/logs.txt > /dev/null
if [ $? -eq 0 ]; then
    echo "Test Passed: Key 'a' has correct value '1'"
else
    echo "Test Failed: Key 'a' does not have correct value '1'"
fi

exit 0