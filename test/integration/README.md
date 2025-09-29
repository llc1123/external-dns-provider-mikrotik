# Integration Tests for External DNS Provider - MikroTik

This directory contains comprehensive integration tests that validate the complete workflow from webhook API endpoints to MikroTik RouterOS API requests.

## Test Coverage

The integration tests cover the following aspects:

### 1. Webhook API Integration (`webhook_test.go`)
- **Negotiate endpoint**: Domain filter negotiation and content type validation
- **Records endpoint**: Fetching DNS records with proper filtering and aggregation
- **ApplyChanges endpoint**: Creating, updating, and deleting DNS records
- **AdjustEndpoints endpoint**: Endpoint adjustment functionality

### 2. MikroTik Client Request Validation (`client_validation_test.go`)
- **HTTP request validation**: Method, path, query parameters, headers
- **Authentication**: Basic Auth header validation
- **Request body validation**: JSON payload structure and content
- **Record type support**: A, AAAA, CNAME, TXT, MX, SRV, NS records
- **Multi-target handling**: Multiple records for single endpoint

### 3. End-to-End Scenarios (`end_to_end_test.go`)
- **Complete record lifecycle**: Create → Read → Update → Delete
- **Multiple record types**: Mixed DNS record type management
- **Multi-target records**: A records with multiple IP addresses
- **Complex updates**: Partial updates and smart differential changes
- **Domain filtering**: Security enforcement across all operations
- **Provider-specific properties**: MikroTik-specific DNS record properties

### 4. Error Handling (`error_handling_test.go`)
- **Authentication failures**: Wrong credentials handling
- **Server errors**: HTTP 4xx/5xx response handling
- **Network errors**: Connectivity issues and timeouts
- **Invalid data**: Malformed DNS records and validation
- **Concurrent operations**: Thread safety and mutex handling
- **Large datasets**: Performance with many DNS records
- **Partial failures**: Mixed success/failure scenarios
- **Resource cleanup**: Proper state management on errors

## Test Architecture

### Mock MikroTik Server
The tests use a sophisticated mock server that:
- Implements the MikroTik RouterOS REST API endpoints
- Captures and validates HTTP requests for verification
- Simulates various error conditions
- Maintains in-memory DNS record state
- Supports authentication validation

### Integration Test Suite
The `IntegrationTestSuite` provides:
- Complete webhook server setup
- MikroTik client and provider initialization
- Mock server management
- Request/response validation utilities
- State management between tests

## Running the Tests

### Prerequisites
```bash
go mod download
```

### Run All Integration Tests
```bash
go test -v ./test/integration/...
```

### Run Specific Test Files
```bash
# Webhook API tests
go test -v ./test/integration/ -run TestWebhook

# Client validation tests  
go test -v ./test/integration/ -run TestMikrotikClient

# End-to-end scenarios
go test -v ./test/integration/ -run TestEndToEnd

# Error handling tests
go test -v ./test/integration/ -run TestErrorHandling
```

### Run with Coverage
```bash
go test -v -coverprofile=coverage.out ./test/integration/...
go tool cover -html=coverage.out -o coverage.html
```

## Test Scenarios Covered

### DNS Record Types
- **A Records**: IPv4 address mapping
- **AAAA Records**: IPv6 address mapping  
- **CNAME Records**: Canonical name aliases
- **TXT Records**: Text records (SPF, DKIM, etc.)
- **MX Records**: Mail exchange records
- **SRV Records**: Service location records
- **NS Records**: Name server delegation

### Operations
- **Create**: Single and multi-target record creation
- **Read**: Record retrieval with domain filtering
- **Update**: Smart differential updates (add/remove targets)
- **Delete**: Precise target-based deletion

### Error Conditions
- **Authentication**: Invalid credentials
- **Authorization**: Domain filter violations
- **Validation**: Invalid DNS data formats
- **Network**: Connection failures and timeouts
- **Server**: HTTP error responses (4xx, 5xx)
- **Concurrency**: Thread safety validation

### Performance
- **Large datasets**: 50+ DNS records handling
- **Multi-target records**: Records with multiple IP addresses
- **Batch operations**: Multiple records in single request
- **Request efficiency**: Minimal API calls for operations

## Request Validation

The tests validate that the MikroTik client sends correct HTTP requests:

### Authentication
```http
Authorization: Basic dGVzdHVzZXI6dGVzdHBhc3M=
```

### Get System Info
```http
GET /rest/system/resource HTTP/1.1
```

### Get DNS Records
```http
GET /rest/ip/dns/static?type=A,AAAA,CNAME,TXT,MX,SRV,NS&comment=external-dns&name=example.com HTTP/1.1
```

### Create DNS Record
```http
PUT /rest/ip/dns/static HTTP/1.1
Content-Type: application/json

{
  "name": "test.example.com",
  "type": "A", 
  "address": "192.0.2.1",
  "ttl": "1h",
  "comment": "external-dns"
}
```

### Delete DNS Record
```http
DELETE /rest/ip/dns/static/*1 HTTP/1.1
```

## Webhook API Validation

The tests validate webhook endpoints according to the external-dns specification:

### Content Type Validation
- Requires: `application/external.dns.webhook+json;version=1`
- Rejects: `application/json` or missing headers

### Domain Security
- Enforces domain filter on all operations
- Prevents unauthorized DNS zone access
- Validates endpoint names against allowed domains

### Response Formats
- Proper JSON serialization of endpoints
- Correct HTTP status codes
- Error message formatting

## Configuration

Tests use these default configurations:

```go
// Mock MikroTik Server
Username: "testuser"
Password: "testpass"  
DefaultComment: "external-dns"
DefaultTTL: 3600

// Domain Filter
AllowedDomains: ["example.com", "test.com"]
```

## Debugging

### Enable Verbose Logging
```bash
go test -v ./test/integration/ -args -log-level=debug
```

### Inspect Request Captures
The mock server captures all HTTP requests which can be inspected in failed tests:
- Request method and path
- Query parameters
- Request headers  
- Request body content
- Timestamp information

### Common Issues
1. **Import errors**: Ensure all dependencies are properly imported
2. **Mock server setup**: Verify server starts and accepts connections
3. **Authentication**: Check mock credentials match client configuration
4. **Domain filtering**: Ensure test domains are in allowed list

## Contributing

When adding new integration tests:

1. Use the existing `IntegrationTestSuite` framework
2. Clear mock server state between tests
3. Validate both webhook behavior and MikroTik API requests
4. Include error case testing
5. Document any new test scenarios in this README

## Test Output Example

```
=== RUN   TestWebhookNegotiate
=== RUN   TestWebhookNegotiate/Valid_accept_header
=== RUN   TestWebhookNegotiate/Missing_accept_header  
=== RUN   TestWebhookNegotiate/Invalid_accept_header
--- PASS: TestWebhookNegotiate (0.01s)

=== RUN   TestMikrotikClientRequestValidation
=== RUN   TestMikrotikClientRequestValidation/GetSystemInfo_request_validation
=== RUN   TestMikrotikClientRequestValidation/CreateDNSRecords_-_single_A_record
--- PASS: TestMikrotikClientRequestValidation (0.05s)

=== RUN   TestEndToEndScenarios
=== RUN   TestEndToEndScenarios/Complete_A_record_lifecycle
=== RUN   TestEndToEndScenarios/Multiple_record_types_management
--- PASS: TestEndToEndScenarios (0.10s)
```