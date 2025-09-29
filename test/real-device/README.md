# Real MikroTik Device Testing

This folder contains test code for testing real MikroTik devices. These tests connect to actual RouterOS devices and perform DNS record operations.

## ⚠️ Security Considerations

1. **Test domain prefix**: All tests use the `test-external-dns-` prefix to ensure no impact on production DNS records
2. **Auto cleanup**: Each test includes cleanup mechanisms to ensure test records are properly deleted
3. **Domain filtering**: Strictly limited to test domain scope to prevent accidental modification of production records

## Environment Configuration

Testing requires configuring a `.env` file in the project root directory with the following variables:

```bash
MIKROTIK_BASEURL=http://192.168.0.1:80
MIKROTIK_USERNAME=external-dns
MIKROTIK_PASSWORD=your_password_here
MIKROTIK_SKIP_TLS_VERIFY=true
MIKROTIK_DEFAULT_COMMENT=external-dns-e2e-test
```

## Test Phases

### Phase 1: Connectivity Test (phase1_connectivity_test.go)
- Test device connection
- Verify authentication
- Get system information
- **No DNS modification operations**

### Phase 2: Safe Read-only Test (phase2_read_only_test.go)
- Read existing DNS records
- Test filtering functionality
- Verify record parsing
- **Read-only operations, no modifications**

### Phase 3: Controlled Create/Delete Test (phase3_controlled_write_test.go)
- Use test-specific domain prefix
- Basic create and delete operations
- Verify record correctness
- Auto cleanup

### Phase 4: Full Lifecycle Test (phase4_full_lifecycle_test.go)
- Complete CRUD operations
- Multiple record type testing
- Multi-target record testing
- Update and intelligent sync testing

## Quick Start

Use the provided scripts to automatically run tests:

### Windows (PowerShell)
```powershell
# Show help
.\test\real-device\run_tests.ps1 -Help

# Run all tests
.\test\real-device\run_tests.ps1

# Run specific phase
.\test\real-device\run_tests.ps1 -Phase 1
.\test\real-device\run_tests.ps1 -Phase 2 -Verbose
.\test\real-device\run_tests.ps1 -Phase 3 -Cleanup

# Use debug logging
.\test\real-device\run_tests.ps1 -LogLevel debug
```

### Linux/macOS (Bash)
```bash
# Show help
./test/real-device/run_tests.sh --help

# Run all tests
./test/real-device/run_tests.sh

# Run specific phase
./test/real-device/run_tests.sh --phase 1
./test/real-device/run_tests.sh --phase 2 --verbose
./test/real-device/run_tests.sh --phase 3 --cleanup

# Use debug logging
./test/real-device/run_tests.sh --log-level debug
```

## Manual Test Execution

If you prefer to run tests manually:

```bash
# Run tests for specific phase
go test -v ./test/real-device/ -run TestPhase1
go test -v ./test/real-device/ -run TestPhase2
go test -v ./test/real-device/ -run TestPhase3
go test -v ./test/real-device/ -run TestPhase4

# Run all tests
go test -v ./test/real-device/

# Run tests with verbose logging
LOG_LEVEL=debug go test -v ./test/real-device/
```

## Cleanup Tools

Clean up all test records:

```bash
go run ./test/real-device/tools/cleanup_test_records.go
```

## Test Domain Conventions

- All test domain names must start with `test-external-dns-`
- For example: \`test-external-dns-basic.example.com\`
- This ensures isolation from production DNS records
