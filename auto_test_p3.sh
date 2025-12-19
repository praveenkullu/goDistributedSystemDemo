#!/bin/bash

# Multi-Paxos KV Store - Automated Test Script
# Project 3: MSC5703/MCS4993
# Tests: Mid-Commit Crash, State Transfer, Leader Failover

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
NUM_SERVERS=5
BASE_PORT=8000
DATA_DIR="./bin/test_data"
LOG_DIR="./bin/test_logs"
SERVER_PIDS=()

# Test results
TESTS_PASSED=0
TESTS_FAILED=0

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up...${NC}"

    # Kill all server processes
    for pid in "${SERVER_PIDS[@]}"; do
        if ps -p $pid > /dev/null 2>&1; then
            kill $pid 2>/dev/null || true
        fi
    done

    # Wait for processes to die
    sleep 2

    # Force kill if still alive
    for pid in "${SERVER_PIDS[@]}"; do
        if ps -p $pid > /dev/null 2>&1; then
            kill -9 $pid 2>/dev/null || true
        fi
    done

    # Clean up data and log directories
    rm -rf "$DATA_DIR"
    rm -rf "$LOG_DIR"

    echo -e "${GREEN}Cleanup complete${NC}"
}

# Trap exit to ensure cleanup
trap cleanup EXIT INT TERM

# Function to start a server
start_server() {
    local server_id=$1
    local port=$((BASE_PORT + server_id + 1))
    local is_leader=$2
    local data_dir="$DATA_DIR/server_$server_id"
    local log_file="$LOG_DIR/server_$server_id.log"

    mkdir -p "$data_dir"
    mkdir -p "$LOG_DIR"

    local leader_flag=""
    if [ "$is_leader" = "true" ]; then
        leader_flag="-role_leader"
    fi

    ./bin/paxos_server -id $server_id -port $port $leader_flag -peers $NUM_SERVERS -data "$data_dir" > "$log_file" 2>&1 &

    local pid=$!
    SERVER_PIDS[$server_id]=$pid

    echo -e "${BLUE}Started server $server_id (PID: $pid) on port $port${NC}"

    # Wait for server to be ready
    sleep 1
}

# Function to stop a server
stop_server() {
    local server_id=$1
    local pid=${SERVER_PIDS[$server_id]}

    if [ -n "$pid" ] && ps -p $pid > /dev/null 2>&1; then
        echo -e "${YELLOW}Stopping server $server_id (PID: $pid)${NC}"
        kill $pid 2>/dev/null || true
        sleep 1
    fi
}

# Function to check if server is running
is_server_running() {
    local server_id=$1
    local pid=${SERVER_PIDS[$server_id]}

    if [ -n "$pid" ] && ps -p $pid > /dev/null 2>&1; then
        return 0
    else
        return 1
    fi
}

# Function to send client request
client_request() {
    local server_id=$1
    local op=$2
    local key=$3
    local value=$4
    local port=$((BASE_PORT + server_id + 1))

    if [ "$op" = "put" ]; then
        ./bin/paxos_client -server "localhost:$port" -op put -key "$key" -value "$value" 2>&1
    elif [ "$op" = "get" ]; then
        ./bin/paxos_client -server "localhost:$port" -op get -key "$key" 2>&1
    elif [ "$op" = "delete" ]; then
        ./bin/paxos_client -server "localhost:$port" -op delete -key "$key" 2>&1
    fi
}

# Function to wait for leader election
wait_for_leader() {
    echo -e "${YELLOW}Waiting for leader election...${NC}"
    sleep 3
}

# Function to check test result
check_result() {
    local test_name=$1
    local expected=$2
    local actual=$3

    if [ "$expected" = "$actual" ]; then
        echo -e "${GREEN}✓ PASS: $test_name${NC}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${RED}✗ FAIL: $test_name${NC}"
        echo -e "  Expected: $expected"
        echo -e "  Actual: $actual"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

# ============================================================================
# Test 1: Basic Functionality
# ============================================================================
test_basic_functionality() {
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE}Test 1: Basic Functionality${NC}"
    echo -e "${BLUE}========================================${NC}\n"

    # Start cluster
    echo "Starting 5-node cluster..."
    for i in {0..4}; do
        if [ $i -eq 0 ]; then
            start_server $i true
        else
            start_server $i false
        fi
    done

    wait_for_leader

    # Test Put
    echo "Testing Put operation..."
    result=$(client_request 0 put "test_key" "test_value" | grep -i "success\|ok" || echo "FAIL")
    if [[ "$result" == *"FAIL"* ]]; then
        echo -e "${RED}Put operation failed${NC}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
    echo -e "${GREEN}Put operation successful${NC}"

    # Test Get
    echo "Testing Get operation..."
    result=$(client_request 0 get "test_key" 2>&1)
    if [[ "$result" == *"test_value"* ]]; then
        echo -e "${GREEN}✓ PASS: Basic Put/Get${NC}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        echo -e "${RED}✗ FAIL: Basic Put/Get${NC}"
        echo "  Result: $result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi

    # Test replication
    echo "Testing replication across servers..."
    sleep 2
    result=$(client_request 1 get "test_key" 2>&1)
    if [[ "$result" == *"test_value"* ]]; then
        echo -e "${GREEN}✓ PASS: Data Replication${NC}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        echo -e "${RED}✗ FAIL: Data Replication${NC}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi

    # Cleanup
    for i in {0..4}; do
        stop_server $i
    done
    sleep 2
}

# ============================================================================
# Test 2: Mid-Commit Crash (CRITICAL)
# ============================================================================
test_mid_commit_crash() {
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE}Test 2: Mid-Commit Crash (CRITICAL)${NC}"
    echo -e "${BLUE}========================================${NC}\n"

    # Start cluster
    echo "Starting 5-node cluster..."
    for i in {0..4}; do
        if [ $i -eq 0 ]; then
            start_server $i true
        else
            start_server $i false
        fi
    done

    wait_for_leader

    # Add initial data
    echo "Adding initial data..."
    client_request 0 put "crash_key" "initial_value"
    sleep 1

    # Put critical value
    echo "Sending critical Put operation..."
    client_request 0 put "data" "final_value" &
    PUT_PID=$!

    # Wait briefly for Accept phase to complete
    sleep 0.5

    # CRASH THE LEADER (Server 0)
    echo -e "${RED}CRASHING Leader (Server 0) mid-commit...${NC}"
    stop_server 0

    # Wait for Put to complete (will fail due to leader crash)
    wait $PUT_PID 2>/dev/null || true

    # Wait for new leader election
    wait_for_leader

    # Try to get the value from the new leader
    echo "Attempting to retrieve value from new leader..."
    sleep 2

    # Try multiple servers to find new leader
    result=""
    for i in {1..4}; do
        echo "Trying server $i..."
        result=$(client_request $i get "data" 2>&1 || echo "")
        if [[ "$result" == *"final_value"* ]]; then
            break
        fi
        sleep 1
    done

    # Validate result
    if [[ "$result" == *"final_value"* ]]; then
        echo -e "${GREEN}✓ PASS: Mid-Commit Crash Recovery${NC}"
        echo -e "${GREEN}  Value successfully recovered: 'final_value'${NC}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        echo -e "${RED}✗ FAIL: Mid-Commit Crash Recovery${NC}"
        echo -e "${RED}  Expected: 'final_value', Got: '$result'${NC}"
        TESTS_FAILED=$((TESTS_FAILED + 1))

        # Show logs for debugging
        echo -e "\n${YELLOW}Server logs for debugging:${NC}"
        for i in {1..4}; do
            if [ -f "$LOG_DIR/server_$i.log" ]; then
                echo -e "${YELLOW}=== Server $i (last 10 lines) ===${NC}"
                tail -10 "$LOG_DIR/server_$i.log"
            fi
        done
    fi

    # Cleanup
    for i in {1..4}; do
        stop_server $i
    done
    sleep 2
}

# ============================================================================
# Test 3: State Transfer
# ============================================================================
test_state_transfer() {
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE}Test 3: State Transfer${NC}"
    echo -e "${BLUE}========================================${NC}\n"

    # Start cluster
    echo "Starting 5-node cluster..."
    for i in {0..4}; do
        if [ $i -eq 0 ]; then
            start_server $i true
        else
            start_server $i false
        fi
    done

    wait_for_leader

    # Add initial data
    echo "Adding initial data..."
    client_request 0 put "st_key1" "value1"
    client_request 0 put "st_key2" "value2"
    sleep 2

    # Stop Server 4
    echo "Stopping Server 4..."
    stop_server 4
    sleep 1

    # Add more data while Server 4 is down
    echo "Adding data while Server 4 is down..."
    client_request 0 put "st_key3" "value3"
    client_request 0 put "st_key4" "value4"
    sleep 2

    # Restart Server 4
    echo "Restarting Server 4..."
    start_server 4 false
    sleep 3

    # Check if Server 4 caught up
    echo "Checking if Server 4 caught up..."
    result=$(client_request 4 get "st_key3" 2>&1 || echo "")

    if [[ "$result" == *"value3"* ]]; then
        echo -e "${GREEN}✓ PASS: State Transfer${NC}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        echo -e "${RED}✗ FAIL: State Transfer${NC}"
        echo "  Result: $result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi

    # Cleanup
    for i in {0..4}; do
        stop_server $i
    done
    sleep 2
}

# ============================================================================
# Test 4: Leader Failover
# ============================================================================
test_leader_failover() {
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE}Test 4: Leader Failover${NC}"
    echo -e "${BLUE}========================================${NC}\n"

    # Start cluster
    echo "Starting 5-node cluster..."
    for i in {0..4}; do
        if [ $i -eq 0 ]; then
            start_server $i true
        else
            start_server $i false
        fi
    done

    wait_for_leader

    # Add data via leader
    echo "Adding data via initial leader (Server 0)..."
    client_request 0 put "failover_key" "failover_value"
    sleep 2

    # Kill leader
    echo "Killing leader (Server 0)..."
    stop_server 0

    # Wait for new leader election
    wait_for_leader

    # Try to access data from new leader
    echo "Accessing data from new leader..."
    result=""
    for i in {1..4}; do
        result=$(client_request $i get "failover_key" 2>&1 || echo "")
        if [[ "$result" == *"failover_value"* ]]; then
            echo -e "${GREEN}✓ PASS: Leader Failover${NC}"
            echo -e "  New leader is Server $i"
            TESTS_PASSED=$((TESTS_PASSED + 1))
            break
        fi
        sleep 1
    done

    if [[ "$result" != *"failover_value"* ]]; then
        echo -e "${RED}✗ FAIL: Leader Failover${NC}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi

    # Cleanup
    for i in {1..4}; do
        stop_server $i
    done
    sleep 2
}

# ============================================================================
# Main Test Runner
# ============================================================================
main() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}Multi-Paxos KV Store - Test Suite${NC}"
    echo -e "${BLUE}Project 3: MSC5703/MCS4993${NC}"
    echo -e "${BLUE}========================================${NC}\n"

    # Check if binaries exist
    if [ ! -f "./bin/paxos_server" ] || [ ! -f "./bin/paxos_client" ]; then
        echo -e "${RED}Error: Binaries not found. Please run ./build_p3.sh first${NC}"
        exit 1
    fi

    # Create directories
    mkdir -p "$DATA_DIR"
    mkdir -p "$LOG_DIR"

    # Run tests
    test_basic_functionality
    test_mid_commit_crash
    test_state_transfer
    test_leader_failover

    # Print summary
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE}Test Summary${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo -e "${GREEN}Tests Passed: $TESTS_PASSED${NC}"
    echo -e "${RED}Tests Failed: $TESTS_FAILED${NC}"
    echo -e "${BLUE}========================================${NC}\n"

    if [ $TESTS_FAILED -eq 0 ]; then
        echo -e "${GREEN}ALL TESTS PASSED! ✓${NC}"
        exit 0
    else
        echo -e "${RED}SOME TESTS FAILED ✗${NC}"
        echo -e "${YELLOW}Check logs in $LOG_DIR for details${NC}"
        exit 1
    fi
}

# Run main function
main
