WAIT_TIME_AFTER_KILL=2
WAIT_TIME=2
TERMINAL_WIDTH=$(tput cols)

touch ./log/logs.txt
echo "Starting automated test..." > ./log/logs.txt

# start the view service
printf "%${TERMINAL_WIDTH}s\n" "[Started View Service]"
go run view/view_server.go &
VS_PID=$!

# echo "Started View Service with PID $VS_PID"

sleep $WAIT_TIME

# start a kv server 1
printf "%${TERMINAL_WIDTH}s\n" "[Started KV Server 1]"
go run kv_server_main/kv_server_main.go -addr="localhost:8001" -vs="localhost:8000" &
KV1_PID=$!


# start the client and make a put operation A: 1
sleep $WAIT_TIME
printf "%${TERMINAL_WIDTH}s\n" "[client put 'a', get 'a']"
go run client_main/client.go -vs="localhost:8000" -ops="put,get" -keys="a,a" -values="1,x" &
CL_PID=$!
wait $CL_PID

sleep $WAIT_TIME
# start another kv server
printf "%${TERMINAL_WIDTH}s\n" "[Started KV Server 2]"
go run kv_server_main/kv_server_main.go -addr="localhost:8002" -vs="localhost:8000"  &
KV2_PID=$!

# start the client and make a put operation B: 2
sleep $WAIT_TIME
printf "%${TERMINAL_WIDTH}s\n" "[client put 'b', get 'b']"
go run client_main/client.go -vs="localhost:8000" -ops="put,get" -keys="b,b" -values="2,x" &
CL_PID=$!
wait $CL_PID

# kill kv server 1
# sleep $WAIT_TIME
printf "%${TERMINAL_WIDTH}s\n" "[Killed KV Server 1]"
kill -9 $(lsof -t -i:8001)

sleep $WAIT_TIME_AFTER_KILL

# client get operation for key A
printf "%${TERMINAL_WIDTH}s\n" "[client get 'a']"
go run client_main/client.go -vs="localhost:8000" -op="get" -key="a" &
CL_PID=$!

# start server 3
printf "%${TERMINAL_WIDTH}s\n" "[Started KV Server 3]"
go run kv_server_main/kv_server_main.go -addr="localhost:8003" -vs="localhost:8000"  &
KV3_PID=$!
#wait for state transfer
sleep $WAIT_TIME

# kill kv server 2
kill $KV2_PID
printf "%${TERMINAL_WIDTH}s\n" "[Killed KV Server 2]"
kill -9 $(lsof -t -i:8002)

sleep $WAIT_TIME_AFTER_KILL

# client get operation for key A
sleep $WAIT_TIME
printf "%${TERMINAL_WIDTH}s\n" "[client get 'a','b']"
go run client_main/client.go -vs="localhost:8000" -ops="get,get" -keys="a,b" 

echo "[Starting Auto verification]"

# ger count of "Get(a) = 1" in logs.txt is equal to 3
grep -c "Get(a) = 1" ./log/logs.txt | grep -q "^3$"

if [ $? -eq 0 ]; then
    echo "Test Passed: Key 'a' has correct value '1'"
else
    echo "Test Failed: Key 'a' does not have correct value '1'"
fi

# #cleanup
# printf "%${TERMINAL_WIDTH}s\n" "[Cleaning up]"
# kill $KV3_PID
# kill $VS_PID


# In case any process is still running on port 8000, 8001, 8002, or 8003, kill them
kill -9 $(sudo lsof -t -i:8000)

echo "Automated test completed." >> ./log/logs.txt