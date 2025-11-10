#!/bin/bash

# Automated Test Script for Distributed KV Service
# Tests: Primary/Backup setup, Primary failure, State transfer, Backup promotion

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
VIEW_SERVICE_ADDR="localhost:8000"
S1_ADDR="localhost:8001"
S2_ADDR="localhost:8002"
S3_ADDR="localhost:8003"

# PIDs for cleanup
VIEW_PID=""
S1_PID=""
S2_PID=""
S3_PID=""

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0

# Cleanup function
cleanup() {
    echo ""
    echo -e "${YELLOW}Cleaning up processes...${NC}"

    if [ ! -z "$S1_PID" ] && kill -0 "$S1_PID" 2>/dev/null; then
        kill "$S1_PID" 2>/dev/null || true
    fi
    if [ ! -z "$S2_PID" ] && kill -0 "$S2_PID" 2>/dev/null; then
        kill "$S2_PID" 2>/dev/null || true
    fi
    if [ ! -z "$S3_PID" ] && kill -0 "$S3_PID" 2>/dev/null; then
        kill "$S3_PID" 2>/dev/null || true
    fi
    if [ ! -z "$VIEW_PID" ] && kill -0 "$VIEW_PID" 2>/dev/null; then
        kill "$VIEW_PID" 2>/dev/null || true
    fi

    # Wait a bit for graceful shutdown
    sleep 1

    echo -e "${GREEN}Cleanup complete!${NC}"
    echo ""
    echo "================================"
    echo -e "${GREEN}Tests Passed: $TESTS_PASSED${NC}"
    if [ $TESTS_FAILED -gt 0 ]; then
        echo -e "${RED}Tests Failed: $TESTS_FAILED${NC}"
    fi
    echo "================================"
}

trap cleanup EXIT INT TERM

# Helper function to print test headers
print_test() {
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}TEST: $1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

# Helper function to print steps
print_step() {
    echo -e "${YELLOW}>>> $1${NC}"
}

# Helper function to assert test result
assert() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✓ PASS: $2${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ FAIL: $2${NC}"
        ((TESTS_FAILED++))
        if [ "${3:-}" != "continue" ]; then
            exit 1
        fi
    fi
}

# Start View Service
start_view_service() {
    print_step "Starting View Service on $VIEW_SERVICE_ADDR"
    go run view/view_server.go -addr "$VIEW_SERVICE_ADDR" > /tmp/view_service.log 2>&1 &
    VIEW_PID=$!
    sleep 2
    if kill -0 "$VIEW_PID" 2>/dev/null; then
        echo -e "${GREEN}View Service started (PID: $VIEW_PID)${NC}"
    else
        echo -e "${RED}Failed to start View Service${NC}"
        exit 1
    fi
}

# Start KV Server
start_kv_server() {
    local name=$1
    local addr=$2
    local pid_var=$3

    print_step "Starting KV Server $name on $addr"
    go run kv_server_main/kv_server_main.go -addr "$addr" -vs "$VIEW_SERVICE_ADDR" > /tmp/${name}.log 2>&1 &
    local pid=$!
    eval "$pid_var=$pid"
    sleep 2
    if kill -0 "$pid" 2>/dev/null; then
        echo -e "${GREEN}KV Server $name started (PID: $pid)${NC}"
    else
        echo -e "${RED}Failed to start KV Server $name${NC}"
        exit 1
    fi
}

# Kill a server
kill_server() {
    local name=$1
    local pid=$2

    print_step "Killing $name (PID: $pid)"
    kill "$pid" 2>/dev/null || true
    sleep 1
    echo -e "${GREEN}$name killed${NC}"
}

# Check view using Go client library
check_view() {
    local output
    output=$(go run test_helpers/check_view.go -vs "$VIEW_SERVICE_ADDR" 2>&1)
    echo "$output"
}

# Perform PUT operation
client_put() {
    local key=$1
    local value=$2

    print_step "Client: Put(\"$key\", \"$value\")"
    local output
    output=$(go run client_main/client.go -vs "$VIEW_SERVICE_ADDR" -op put -key "$key" -value "$value" 2>&1)
    echo "$output"
}

# Perform GET operation
client_get() {
    local key=$1

    print_step "Client: Get(\"$key\")"
    local output
    output=$(go run client_main/client.go -vs "$VIEW_SERVICE_ADDR" -op get -key "$key" 2>&1)
    echo "$output"
}

# Verify GET result
verify_get() {
    local key=$1
    local expected=$2

    local result
    result=$(go run client_main/client.go -vs "$VIEW_SERVICE_ADDR" -op get -key "$key" 2>&1)

    if echo "$result" | grep -q "$expected"; then
        assert 0 "Get(\"$key\") returned \"$expected\""
        return 0
    else
        echo -e "${RED}Expected: $expected, Got: $result${NC}"
        assert 1 "Get(\"$key\") returned \"$expected\""
        return 1
    fi
}

# Main test execution
main() {
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}Distributed KV Service Test Suite${NC}"
    echo -e "${GREEN}========================================${NC}"

    # Step 1: Start View Service
    print_test "Step 1: Start View Service"
    start_view_service

    # Step 2: Start KV server S1
    print_test "Step 2: Start KV Server S1"
    start_kv_server "S1" "$S1_ADDR" "S1_PID"

    # Step 3: Check view (S1 should be Primary)
    print_test "Step 3: Check View (S1 should be Primary)"
    sleep 1  # Wait for view to stabilize
    view_output=$(check_view)
    echo "$view_output"
    if echo "$view_output" | grep -q "Primary: $S1_ADDR" && echo "$view_output" | grep -q "Backup: <none>"; then
        assert 0 "S1 is Primary, no Backup"
    else
        assert 1 "S1 is Primary, no Backup"
    fi

    # Step 4: Client operations - Put and Get
    print_test "Step 4: Client Operations - Put(\"a\", \"1\") and Get(\"a\")"
    client_put "a" "1"
    sleep 0.5
    verify_get "a" "1"

    # Step 5: Start KV server S2
    print_test "Step 5: Start KV Server S2"
    start_kv_server "S2" "$S2_ADDR" "S2_PID"

    # Step 6: Check view (S1 Primary, S2 Backup)
    print_test "Step 6: Check View (S1 Primary, S2 Backup)"
    sleep 1.5  # Wait for view to update
    view_output=$(check_view)
    echo "$view_output"
    if echo "$view_output" | grep -q "Primary: $S1_ADDR" && echo "$view_output" | grep -q "Backup: $S2_ADDR"; then
        assert 0 "S1 is Primary, S2 is Backup"
    else
        assert 1 "S1 is Primary, S2 is Backup"
    fi

    # Step 7: Client Put
    print_test "Step 7: Client Put(\"b\", \"2\")"
    client_put "b" "2"
    sleep 0.5

    # Step 8: Test Primary Failure - Kill S1
    print_test "Step 8: Test Primary Failure - Kill S1"
    kill_server "S1" "$S1_PID"

    # Step 9: Wait 2-3 seconds
    print_step "Waiting 3 seconds for failure detection..."
    sleep 3

    # Step 10: Check view (S2 should be Primary, no Backup)
    print_test "Step 10: Check View (S2 should be Primary, no Backup)"
    view_output=$(check_view)
    echo "$view_output"
    if echo "$view_output" | grep -q "Primary: $S2_ADDR" && echo "$view_output" | grep -q "Backup: <none>"; then
        assert 0 "S2 is Primary, no Backup"
    else
        assert 1 "S2 is Primary, no Backup"
    fi

    # Step 11: Client Get operations
    print_test "Step 11: Verify Data After Primary Failure"
    verify_get "a" "1"
    verify_get "b" "2"

    # Step 12: Test State Transfer - Start S3
    print_test "Step 12: Test State Transfer - Start KV Server S3"
    start_kv_server "S3" "$S3_ADDR" "S3_PID"

    # Step 13: Check view (S2 Primary, S3 Backup)
    print_test "Step 13: Check View (S2 Primary, S3 Backup)"
    sleep 2  # Wait for state transfer
    view_output=$(check_view)
    echo "$view_output"
    if echo "$view_output" | grep -q "Primary: $S2_ADDR" && echo "$view_output" | grep -q "Backup: $S3_ADDR"; then
        assert 0 "S2 is Primary, S3 is Backup"
    else
        assert 1 "S2 is Primary, S3 is Backup"
    fi

    # Step 14: Wait for state transfer to complete
    print_step "Waiting for state transfer to complete..."
    sleep 2

    # Step 15: Test New Backup - Kill S2
    print_test "Step 15: Test Backup Promotion - Kill S2"
    kill_server "S2" "$S2_PID"

    # Wait for failure detection
    print_step "Waiting 3 seconds for failure detection..."
    sleep 3

    # Step 16: Check view (S3 Primary, no Backup)
    print_test "Step 16: Check View (S3 should be Primary, no Backup)"
    view_output=$(check_view)
    echo "$view_output"
    if echo "$view_output" | grep -q "Primary: $S3_ADDR" && echo "$view_output" | grep -q "Backup: <none>"; then
        assert 0 "S3 is Primary, no Backup"
    else
        assert 1 "S3 is Primary, no Backup"
    fi

    # Step 17: Final verification
    print_test "Step 17: Final Data Verification"
    verify_get "a" "1"
    verify_get "b" "2"

    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}ALL TESTS COMPLETED SUCCESSFULLY!${NC}"
    echo -e "${GREEN}========================================${NC}"
}

# Run main test
main
