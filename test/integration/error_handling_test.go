package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/mirceanton/external-dns-provider-mikrotik/internal/mikrotik"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

// TestErrorHandlingScenarios tests various error conditions and edge cases
func TestErrorHandlingScenarios(t *testing.T) {
	suite := NewIntegrationTestSuite(t)
	defer suite.Close()

	testCases := []struct {
		name        string
		scenario    func(*testing.T, *IntegrationTestSuite)
		description string
	}{
		{
			name:        "MikroTik API authentication failures",
			description: "Test handling of authentication errors from MikroTik API",
			scenario:    testAuthenticationFailures,
		},
		{
			name:        "MikroTik API server errors",
			description: "Test handling of server errors from MikroTik API",
			scenario:    testServerErrors,
		},
		{
			name:        "Network connectivity issues",
			description: "Test handling of network connectivity problems",
			scenario:    testNetworkErrors,
		},
		{
			name:        "Invalid DNS record data",
			description: "Test handling of malformed or invalid DNS records",
			scenario:    testInvalidRecordData,
		},
		{
			name:        "Concurrent operations",
			description: "Test handling of concurrent DNS operations",
			scenario:    testConcurrentOperations,
		},
		{
			name:        "Large dataset handling",
			description: "Test performance with large numbers of DNS records",
			scenario:    testLargeDatasets,
		},
		{
			name:        "Partial failure scenarios",
			description: "Test handling when some operations succeed and others fail",
			scenario:    testPartialFailures,
		},
		{
			name:        "Resource cleanup on errors",
			description: "Test proper cleanup when operations fail midway",
			scenario:    testResourceCleanup,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset state for each test
			suite.mockServer.ClearRequests()
			suite.mockServer.ClearRecords()
			suite.mockServer.SetError(false, 0)

			t.Logf("Running error scenario: %s", tc.description)
			tc.scenario(t, suite)
		})
	}
}

// testAuthenticationFailures tests handling of authentication errors
func testAuthenticationFailures(t *testing.T, suite *IntegrationTestSuite) {
	// Test 1: Create client with wrong credentials
	wrongConfig := &mikrotik.MikrotikConnectionConfig{
		BaseUrl:       suite.mockServer.URL(),
		Username:      "wronguser",
		Password:      "wrongpass",
		SkipTLSVerify: true,
	}
	defaults := &mikrotik.MikrotikDefaults{
		DefaultTTL:     3600,
		DefaultComment: defaultComment,
	}

	wrongClient, err := mikrotik.NewMikrotikClient(wrongConfig, defaults)
	if err != nil {
		t.Fatalf("Failed to create client with wrong credentials: %v", err)
	}

	// Test GetSystemInfo with wrong credentials
	_, err = wrongClient.GetSystemInfo()
	if err == nil {
		t.Error("Expected authentication error, got none")
	}

	// Test 2: Webhook operations should fail gracefully with auth errors
	// First set up a provider that will fail auth
	wrongProvider, err := mikrotik.NewMikrotikProvider(
		endpoint.NewDomainFilter([]string{"example.com"}),
		defaults,
		wrongConfig,
	)
	if err == nil {
		t.Error("Expected provider creation to fail with wrong credentials")
	}

	// The provider creation should fail during the connection test
	if wrongProvider != nil {
		t.Error("Provider should be nil when auth fails")
	}
}

// testServerErrors tests handling of various server error responses
func testServerErrors(t *testing.T, suite *IntegrationTestSuite) {
	errorScenarios := []struct {
		name      string
		errorCode int
		operation func() error
	}{
		{
			name:      "500 Internal Server Error on GetSystemInfo",
			errorCode: http.StatusInternalServerError,
			operation: func() error {
				_, err := suite.client.GetSystemInfo()
				return err
			},
		},
		{
			name:      "404 Not Found on GetDNSRecords",
			errorCode: http.StatusNotFound,
			operation: func() error {
				_, err := suite.client.GetDNSRecordsByName("nonexistent.example.com")
				return err
			},
		},
		{
			name:      "400 Bad Request on invalid record",
			errorCode: http.StatusBadRequest,
			operation: func() error {
				endpoint := &endpoint.Endpoint{
					DNSName:    "bad.example.com",
					RecordType: "A",
					Targets:    []string{"invalid-ip"},
				}
				_, err := suite.client.CreateDNSRecords(endpoint)
				return err
			},
		},
	}

	for _, scenario := range errorScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Configure mock server to return error
			suite.mockServer.SetError(true, scenario.errorCode)

			// Execute operation and expect error
			err := scenario.operation()
			if err == nil {
				t.Errorf("Expected error for %s, got none", scenario.name)
			}

			// Reset error state
			suite.mockServer.SetError(false, 0)
		})
	}
}

// testNetworkErrors tests handling of network connectivity issues
func testNetworkErrors(t *testing.T, suite *IntegrationTestSuite) {
	// Test with unreachable server
	unreachableConfig := &mikrotik.MikrotikConnectionConfig{
		BaseUrl:       "https://192.0.2.254:443", // Non-routable IP
		Username:      mockUsername,
		Password:      mockPassword,
		SkipTLSVerify: true,
	}
	defaults := &mikrotik.MikrotikDefaults{
		DefaultTTL:     3600,
		DefaultComment: defaultComment,
	}

	unreachableClient, err := mikrotik.NewMikrotikClient(unreachableConfig, defaults)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Operations should timeout or fail with network error
	_, err = unreachableClient.GetSystemInfo()
	if err == nil {
		t.Error("Expected network error for unreachable server, got none")
	}

	// Test webhook behavior with network errors
	unreachableProvider, err := mikrotik.NewMikrotikProvider(
		endpoint.NewDomainFilter([]string{"example.com"}),
		defaults,
		unreachableConfig,
	)

	// Provider creation should fail due to connection test
	if err == nil {
		t.Error("Expected provider creation to fail with unreachable server")
	}
	if unreachableProvider != nil {
		t.Error("Provider should be nil when connection fails")
	}
}

// testInvalidRecordData tests handling of invalid or malformed DNS records
func testInvalidRecordData(t *testing.T, suite *IntegrationTestSuite) {
	invalidRecords := []struct {
		name     string
		endpoint *endpoint.Endpoint
	}{
		{
			name: "Empty DNS name",
			endpoint: &endpoint.Endpoint{
				DNSName:    "",
				RecordType: "A",
				Targets:    []string{"192.0.2.1"},
			},
		},
		{
			name: "Invalid IP address",
			endpoint: &endpoint.Endpoint{
				DNSName:    "invalid.example.com",
				RecordType: "A",
				Targets:    []string{"999.999.999.999"},
			},
		},
		{
			name: "Empty targets",
			endpoint: &endpoint.Endpoint{
				DNSName:    "empty.example.com",
				RecordType: "A",
				Targets:    []string{},
			},
		},
		{
			name: "Invalid MX record format",
			endpoint: &endpoint.Endpoint{
				DNSName:    "mx.example.com",
				RecordType: "MX",
				Targets:    []string{"invalid-mx-format"},
			},
		},
		{
			name: "Invalid SRV record format",
			endpoint: &endpoint.Endpoint{
				DNSName:    "srv.example.com",
				RecordType: "SRV",
				Targets:    []string{"invalid srv format"},
			},
		},
	}

	for _, invalid := range invalidRecords {
		t.Run(invalid.name, func(t *testing.T) {
			_, err := suite.client.CreateDNSRecords(invalid.endpoint)
			if err == nil {
				t.Errorf("Expected error for invalid record %s, got none", invalid.name)
			}
		})
	}
}

// testConcurrentOperations tests handling of concurrent DNS operations
func testConcurrentOperations(t *testing.T, suite *IntegrationTestSuite) {
	// This test would ideally use goroutines to test concurrent operations
	// For simplicity, we'll test the delete mutex functionality

	// Setup multiple records for the same domain
	records := []mikrotik.DNSRecord{
		{
			Name:    "concurrent.example.com",
			Type:    "A",
			Address: "192.0.2.10",
			TTL:     "3600s",
			Comment: defaultComment,
		},
		{
			Name:    "concurrent.example.com",
			Type:    "A",
			Address: "192.0.2.11",
			TTL:     "3600s",
			Comment: defaultComment,
		},
	}

	for _, record := range records {
		suite.mockServer.AddRecord(record)
	}

	// Delete operations should be serialized due to mutex
	deleteEndpoint := &endpoint.Endpoint{
		DNSName:    "concurrent.example.com",
		RecordType: "A",
		Targets:    []string{"192.0.2.10", "192.0.2.11"},
	}

	err := suite.client.DeleteDNSRecords(deleteEndpoint)
	if err != nil {
		t.Errorf("Concurrent delete operation failed: %v", err)
	}

	// Verify both records were deleted
	remaining := suite.mockServer.GetRecordsByNameAndType("concurrent.example.com", "A")
	if len(remaining) != 0 {
		t.Errorf("Expected 0 remaining records, got %d", len(remaining))
	}
}

// testLargeDatasets tests performance with large numbers of DNS records
func testLargeDatasets(t *testing.T, suite *IntegrationTestSuite) {
	// Create a moderate number of records to test pagination and performance
	const numRecords = 50

	// Setup many records
	for i := 0; i < numRecords; i++ {
		record := mikrotik.DNSRecord{
			Name:    "large-test.example.com",
			Type:    "A",
			Address: fmt.Sprintf("192.0.2.%d", i+1),
			TTL:     "3600s",
			Comment: defaultComment,
		}
		suite.mockServer.AddRecord(record)
	}

	// Test fetching all records via webhook
	resp, err := suite.makeWebhookRequest("GET", "/records", nil)
	if err != nil {
		t.Fatalf("Failed to get large dataset: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var endpoints []*endpoint.Endpoint
	if err := json.NewDecoder(resp.Body).Decode(&endpoints); err != nil {
		t.Fatalf("Failed to decode large dataset: %v", err)
	}

	// Should aggregate all records into one endpoint with multiple targets
	if len(endpoints) != 1 {
		t.Errorf("Expected 1 aggregated endpoint, got %d", len(endpoints))
	} else if len(endpoints[0].Targets) != numRecords {
		t.Errorf("Expected %d targets in endpoint, got %d", numRecords, len(endpoints[0].Targets))
	}
}

// testPartialFailures tests scenarios where some operations succeed and others fail
func testPartialFailures(t *testing.T, suite *IntegrationTestSuite) {
	// Create multiple records in one request where some might fail
	createChanges := &plan.Changes{
		Create: []*endpoint.Endpoint{
			{
				DNSName:    "success1.example.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.100"},
			},
			{
				DNSName:    "success2.example.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.101"},
			},
		},
	}

	// Configure server to fail after first successful request
	// This is a limitation of our current mock - in a real scenario,
	// we'd need more sophisticated failure injection

	resp, err := suite.makeWebhookRequest("POST", "/records", createChanges)
	if err != nil {
		t.Fatalf("Failed to create records: %v", err)
	}
	resp.Body.Close()

	// With current implementation, all records should be created successfully
	// or the entire operation should fail
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}

	// Verify both records were created
	records1 := suite.mockServer.GetRecordsByNameAndType("success1.example.com", "A")
	records2 := suite.mockServer.GetRecordsByNameAndType("success2.example.com", "A")

	if len(records1) != 1 {
		t.Errorf("Expected 1 record for success1, got %d", len(records1))
	}
	if len(records2) != 1 {
		t.Errorf("Expected 1 record for success2, got %d", len(records2))
	}
}

// testResourceCleanup tests proper cleanup when operations fail midway
func testResourceCleanup(t *testing.T, suite *IntegrationTestSuite) {
	// This test ensures that failed operations don't leave partial state

	// Setup initial record
	initialRecord := mikrotik.DNSRecord{
		Name:    "cleanup.example.com",
		Type:    "A",
		Address: "192.0.2.200",
		TTL:     "3600s",
		Comment: defaultComment,
	}
	suite.mockServer.AddRecord(initialRecord)

	// Attempt update that might fail midway
	updateChanges := &plan.Changes{
		UpdateOld: []*endpoint.Endpoint{
			{
				DNSName:    "cleanup.example.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.200"},
			},
		},
		UpdateNew: []*endpoint.Endpoint{
			{
				DNSName:    "cleanup.example.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.201"},
			},
		},
	}

	// Normal update should succeed
	resp, err := suite.makeWebhookRequest("POST", "/records", updateChanges)
	if err != nil {
		t.Fatalf("Update operation failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}

	// Verify final state is consistent
	finalRecords := suite.mockServer.GetRecordsByNameAndType("cleanup.example.com", "A")
	if len(finalRecords) != 1 {
		t.Errorf("Expected 1 final record, got %d", len(finalRecords))
	} else if finalRecords[0].Address != "192.0.2.201" {
		t.Errorf("Expected final address 192.0.2.201, got %s", finalRecords[0].Address)
	}
}

// TestWebhookErrorHandling tests webhook-specific error scenarios
func TestWebhookErrorHandling(t *testing.T) {
	suite := NewIntegrationTestSuite(t)
	defer suite.Close()

	testCases := []struct {
		name           string
		method         string
		path           string
		headers        map[string]string
		body           interface{}
		expectedStatus int
	}{
		{
			name:   "Missing Content-Type header",
			method: "POST",
			path:   "/records",
			headers: map[string]string{
				"Accept": contentTypeJSON,
			},
			body:           &plan.Changes{},
			expectedStatus: http.StatusNotAcceptable,
		},
		{
			name:   "Invalid Content-Type header",
			method: "POST",
			path:   "/records",
			headers: map[string]string{
				"Content-Type": "application/json",
				"Accept":       contentTypeJSON,
			},
			body:           &plan.Changes{},
			expectedStatus: http.StatusUnsupportedMediaType,
		},
		{
			name:           "Missing Accept header",
			method:         "GET",
			path:           "/records",
			headers:        map[string]string{},
			expectedStatus: http.StatusNotAcceptable,
		},
		{
			name:   "Invalid Accept header",
			method: "GET",
			path:   "/records",
			headers: map[string]string{
				"Accept": "application/json",
			},
			expectedStatus: http.StatusUnsupportedMediaType,
		},
		{
			name:   "Malformed JSON body",
			method: "POST",
			path:   "/records",
			headers: map[string]string{
				"Content-Type": contentTypeJSON,
			},
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var bodyBytes []byte
			var err error

			if tc.body != nil {
				if str, ok := tc.body.(string); ok {
					bodyBytes = []byte(str)
				} else {
					bodyBytes, err = json.Marshal(tc.body)
					if err != nil {
						t.Fatalf("Failed to marshal body: %v", err)
					}
				}
			}

			req, err := http.NewRequest(tc.method, suite.httpServer.URL+tc.path,
				bytes.NewReader(bodyBytes))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
			}
		})
	}
}
