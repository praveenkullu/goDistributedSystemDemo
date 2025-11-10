# Automated KV Service Test Suite

This directory contains an automated test script for the distributed key-value service that tests primary/backup replication, failure detection, and state transfer.

## Overview

The test script (`test_kv_service.sh`) automates the following test scenarios:

1. **Initial Setup**: Start View Service and first KV server (S1)
2. **Primary Assignment**: Verify S1 becomes Primary
3. **Basic Operations**: Test Put/Get operations
4. **Backup Assignment**: Add S2 and verify it becomes Backup
5. **Primary Failure**: Kill S1 and verify S2 promotes to Primary
6. **Data Persistence**: Verify data survives primary failure
7. **State Transfer**: Add S3 as new Backup and verify state transfer
8. **Backup Promotion**: Kill S2 and verify S3 promotes to Primary
9. **Final Verification**: Ensure all data is still accessible

## Files

- **test_kv_service.sh**: Main automated test script
- **test_helpers/check_view.go**: Helper program to query and display the current view

## Prerequisites

1. Go 1.25.1 or later installed
2. All project dependencies installed (`go mod download`)
3. Project built successfully

## Running the Tests

Simply execute the test script from the project root directory:

```bash
./test_kv_service.sh
```

The script will:
- Start all necessary services
- Run through all test scenarios
- Display color-coded output for each test step
- Clean up all processes on exit (even if tests fail)
- Display a summary of passed/failed tests

## Test Output

The script provides color-coded output:
- 🔵 **Blue**: Test section headers
- 🟡 **Yellow**: Step descriptions
- 🟢 **Green**: Successful tests and operations
- 🔴 **Red**: Failed tests or errors

Example output:
```
========================================
TEST: Step 3: Check View (S1 should be Primary)
========================================
>>> Starting KV Server S1 on localhost:8001
KV Server S1 started (PID: 12345)
View Number: 1
Primary: localhost:8001
Backup: <none>
✓ PASS: S1 is Primary, no Backup
```

## Test Scenarios

### 1. Initial Primary Setup
- Starts View Service
- Starts S1 (localhost:8001)
- Verifies S1 becomes Primary with no Backup

### 2. Basic Client Operations
- Put("a", "1")
- Get("a") → should return "1"

### 3. Backup Assignment
- Starts S2 (localhost:8002)
- Verifies view shows S1 as Primary, S2 as Backup
- Put("b", "2")

### 4. Primary Failure Recovery
- Kills S1
- Waits for failure detection (3 seconds)
- Verifies S2 promoted to Primary
- Get("a") → should return "1"
- Get("b") → should return "2"

### 5. State Transfer
- Starts S3 (localhost:8003)
- Verifies S3 becomes Backup
- Waits for state transfer to complete

### 6. Backup Promotion
- Kills S2
- Waits for failure detection (3 seconds)
- Verifies S3 promoted to Primary
- Get("a") → should return "1"
- Get("b") → should return "2"

## Configuration

The script uses the following default addresses:

```bash
VIEW_SERVICE_ADDR="localhost:8000"
S1_ADDR="localhost:8001"
S2_ADDR="localhost:8002"
S3_ADDR="localhost:8003"
```

To modify these, edit the configuration section at the top of `test_kv_service.sh`.

## Logs

Service logs are written to `/tmp/`:
- `/tmp/view_service.log` - View Service output
- `/tmp/S1.log` - KV Server S1 output
- `/tmp/S2.log` - KV Server S2 output
- `/tmp/S3.log` - KV Server S3 output

These logs are useful for debugging if tests fail.

## Troubleshooting

### Tests fail with "connection refused"
- Ensure no other processes are using ports 8000-8003
- Check that Go modules are properly installed

### View checks fail
- The system needs time to detect failures and update views
- Default wait times are 3 seconds after killing servers
- You may need to adjust sleep times if running on a slow system

### Server startup fails
- Check logs in `/tmp/` directory
- Ensure all dependencies are available
- Verify the project builds successfully: `go build ./...`

## Manual Testing

You can also use the helper tools independently:

### Check Current View
```bash
go run test_helpers/check_view.go -vs localhost:8000
```

### Start Services Manually
```bash
# Start View Service
go run view/view_server.go -addr localhost:8000

# Start KV Server
go run kv_server_main/kv_server_main.go -addr localhost:8001 -vs localhost:8000

# Client Put
go run client_main/client.go -vs localhost:8000 -op put -key foo -value bar

# Client Get
go run client_main/client.go -vs localhost:8000 -op get -key foo
```

## Cleanup

The script automatically cleans up all started processes on exit using trap handlers.

If processes don't clean up properly, you can manually kill them:
```bash
# Find and kill all related processes
pkill -f "view_server.go"
pkill -f "kv_server_main.go"
```

## Exit Codes

- **0**: All tests passed
- **1**: One or more tests failed

## Extending the Tests

To add new test scenarios:

1. Add a new test function following the pattern:
```bash
print_test "Your Test Name"
# Your test code here
assert <condition> "Test description"
```

2. Call your test function from the `main()` function

3. Use the provided helper functions:
   - `start_kv_server <name> <addr> <pid_var>`
   - `kill_server <name> <pid>`
   - `check_view`
   - `client_put <key> <value>`
   - `client_get <key>`
   - `verify_get <key> <expected_value>`
