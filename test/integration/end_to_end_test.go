package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mirceanton/external-dns-provider-mikrotik/internal/mikrotik"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

// TestEndToEndScenarios tests complete workflows from webhook API to MikroTik API
func TestEndToEndScenarios(t *testing.T) {
	suite := NewIntegrationTestSuite(t)
	defer suite.Close()

	testCases := []struct {
		name        string
		scenario    func(*testing.T, *IntegrationTestSuite)
		description string
	}{
		{
			name:        "Complete A record lifecycle",
			description: "Create, read, update, and delete A records",
			scenario:    testARecordLifecycle,
		},
		{
			name:        "Multiple record types management",
			description: "Create and manage different DNS record types",
			scenario:    testMultipleRecordTypes,
		},
		{
			name:        "Multi-target A record management",
			description: "Manage A records with multiple IP targets",
			scenario:    testMultiTargetARecords,
		},
		{
			name:        "Complex update scenarios",
			description: "Test various update patterns including partial updates",
			scenario:    testComplexUpdateScenarios,
		},
		{
			name:        "Domain filter enforcement",
			description: "Verify domain filtering works across all operations",
			scenario:    testDomainFilterEnforcement,
		},
		{
			name:        "Provider-specific properties",
			description: "Test custom MikroTik-specific properties",
			scenario:    testProviderSpecificProperties,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset state for each test
			suite.mockServer.ClearRequests()
			suite.mockServer.ClearRecords()
			suite.mockServer.SetError(false, 0)

			t.Logf("Running scenario: %s", tc.description)
			tc.scenario(t, suite)
		})
	}
}

// testARecordLifecycle tests the complete lifecycle of A records
func testARecordLifecycle(t *testing.T, suite *IntegrationTestSuite) {
	// Step 1: Verify no records initially
	resp, err := suite.makeWebhookRequest("GET", "/records", nil)
	if err != nil {
		t.Fatalf("Failed to get initial records: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var initialEndpoints []*endpoint.Endpoint
	if err := json.NewDecoder(resp.Body).Decode(&initialEndpoints); err != nil {
		t.Fatalf("Failed to decode initial endpoints: %v", err)
	}

	if len(initialEndpoints) != 0 {
		t.Errorf("Expected 0 initial endpoints, got %d", len(initialEndpoints))
	}

	// Step 2: Create A record via webhook
	createChanges := &plan.Changes{
		Create: []*endpoint.Endpoint{
			{
				DNSName:    "lifecycle.example.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.100"},
				RecordTTL:  endpoint.TTL(3600),
			},
		},
	}

	resp, err = suite.makeWebhookRequest("POST", "/records", createChanges)
	if err != nil {
		t.Fatalf("Failed to create record: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}

	// Verify MikroTik API was called correctly for creation
	suite.assertRequestCaptured("PUT", "/rest/ip/dns/static", nil)

	// Step 3: Verify record was created by reading via webhook
	resp, err = suite.makeWebhookRequest("GET", "/records", nil)
	if err != nil {
		t.Fatalf("Failed to get records after creation: %v", err)
	}
	defer resp.Body.Close()

	var afterCreateEndpoints []*endpoint.Endpoint
	if err := json.NewDecoder(resp.Body).Decode(&afterCreateEndpoints); err != nil {
		t.Fatalf("Failed to decode endpoints after creation: %v", err)
	}

	if len(afterCreateEndpoints) != 1 {
		t.Errorf("Expected 1 endpoint after creation, got %d", len(afterCreateEndpoints))
	} else {
		ep := afterCreateEndpoints[0]
		if ep.DNSName != "lifecycle.example.com" || ep.RecordType != "A" || len(ep.Targets) != 1 || ep.Targets[0] != "192.0.2.100" {
			t.Errorf("Created endpoint doesn't match expected: %+v", ep)
		}
	}

	// Step 4: Update record via webhook
	updateChanges := &plan.Changes{
		UpdateOld: []*endpoint.Endpoint{
			{
				DNSName:    "lifecycle.example.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.100"},
			},
		},
		UpdateNew: []*endpoint.Endpoint{
			{
				DNSName:    "lifecycle.example.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.101"},
				RecordTTL:  endpoint.TTL(7200),
			},
		},
	}

	suite.mockServer.ClearRequests() // Clear to track update requests
	resp, err = suite.makeWebhookRequest("POST", "/records", updateChanges)
	if err != nil {
		t.Fatalf("Failed to update record: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}

	// Verify both GET (for smart update) and DELETE/CREATE were called
	requests := suite.mockServer.GetRequests()
	foundGet := false
	foundDelete := false
	foundCreate := false
	for _, req := range requests {
		if req.Method == "GET" && req.Path == "/rest/ip/dns/static" {
			foundGet = true
		}
		if req.Method == "DELETE" {
			foundDelete = true
		}
		if req.Method == "PUT" && req.Path == "/rest/ip/dns/static" {
			foundCreate = true
		}
	}

	if !foundGet {
		t.Error("Expected GET request for smart update not found")
	}
	if !foundDelete {
		t.Error("Expected DELETE request for update not found")
	}
	if !foundCreate {
		t.Error("Expected CREATE request for update not found")
	}

	// Step 5: Delete record via webhook
	deleteChanges := &plan.Changes{
		Delete: []*endpoint.Endpoint{
			{
				DNSName:    "lifecycle.example.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.101"},
			},
		},
	}

	suite.mockServer.ClearRequests()
	resp, err = suite.makeWebhookRequest("POST", "/records", deleteChanges)
	if err != nil {
		t.Fatalf("Failed to delete record: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}

	// Verify DELETE was called
	suite.assertRequestCaptured("DELETE", "/rest/ip/dns/static/*2", nil)

	// Step 6: Verify no records remain
	resp, err = suite.makeWebhookRequest("GET", "/records", nil)
	if err != nil {
		t.Fatalf("Failed to get final records: %v", err)
	}
	defer resp.Body.Close()

	var finalEndpoints []*endpoint.Endpoint
	if err := json.NewDecoder(resp.Body).Decode(&finalEndpoints); err != nil {
		t.Fatalf("Failed to decode final endpoints: %v", err)
	}

	if len(finalEndpoints) != 0 {
		t.Errorf("Expected 0 endpoints after deletion, got %d", len(finalEndpoints))
	}
}

// testMultipleRecordTypes tests managing different DNS record types
func testMultipleRecordTypes(t *testing.T, suite *IntegrationTestSuite) {
	// Create multiple record types in a single request
	createChanges := &plan.Changes{
		Create: []*endpoint.Endpoint{
			{
				DNSName:    "multi.example.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.200"},
				RecordTTL:  endpoint.TTL(3600),
			},
			{
				DNSName:    "www.example.com",
				RecordType: "CNAME",
				Targets:    []string{"multi.example.com"},
				RecordTTL:  endpoint.TTL(3600),
			},
			{
				DNSName:    "mail.example.com",
				RecordType: "MX",
				Targets:    []string{"10 smtp.example.com"},
				RecordTTL:  endpoint.TTL(3600),
			},
			{
				DNSName:    "text.example.com",
				RecordType: "TXT",
				Targets:    []string{"v=spf1 include:_spf.google.com ~all"},
				RecordTTL:  endpoint.TTL(3600),
			},
			{
				DNSName:    "_sip._tcp.example.com",
				RecordType: "SRV",
				Targets:    []string{"10 20 5060 sip.example.com"},
				RecordTTL:  endpoint.TTL(3600),
			},
		},
	}

	resp, err := suite.makeWebhookRequest("POST", "/records", createChanges)
	if err != nil {
		t.Fatalf("Failed to create multiple record types: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}

	// Verify all record types were created by checking MikroTik API calls
	requests := suite.mockServer.GetRequests()
	createCount := 0
	for _, req := range requests {
		if req.Method == "PUT" && req.Path == "/rest/ip/dns/static" {
			createCount++
		}
	}

	expectedCreateCount := 5
	if createCount != expectedCreateCount {
		t.Errorf("Expected %d CREATE requests, got %d", expectedCreateCount, createCount)
	}

	// Verify all records can be read back
	resp, err = suite.makeWebhookRequest("GET", "/records", nil)
	if err != nil {
		t.Fatalf("Failed to get records: %v", err)
	}
	defer resp.Body.Close()

	var endpoints []*endpoint.Endpoint
	if err := json.NewDecoder(resp.Body).Decode(&endpoints); err != nil {
		t.Fatalf("Failed to decode endpoints: %v", err)
	}

	if len(endpoints) != expectedCreateCount {
		t.Errorf("Expected %d endpoints, got %d", expectedCreateCount, len(endpoints))
	}

	// Verify each record type is present
	recordTypes := make(map[string]bool)
	for _, ep := range endpoints {
		recordTypes[ep.RecordType] = true
	}

	expectedTypes := []string{"A", "CNAME", "MX", "TXT", "SRV"}
	for _, expectedType := range expectedTypes {
		if !recordTypes[expectedType] {
			t.Errorf("Expected record type %s not found", expectedType)
		}
	}
}

// testMultiTargetARecords tests A records with multiple IP addresses
func testMultiTargetARecords(t *testing.T, suite *IntegrationTestSuite) {
	// Create A record with multiple targets
	createChanges := &plan.Changes{
		Create: []*endpoint.Endpoint{
			{
				DNSName:    "multitarget.example.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.10", "192.0.2.11", "192.0.2.12"},
				RecordTTL:  endpoint.TTL(3600),
			},
		},
	}

	resp, err := suite.makeWebhookRequest("POST", "/records", createChanges)
	if err != nil {
		t.Fatalf("Failed to create multi-target record: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}

	// Verify 3 individual records were created
	requests := suite.mockServer.GetRequests()
	createCount := 0
	for _, req := range requests {
		if req.Method == "PUT" && req.Path == "/rest/ip/dns/static" {
			createCount++
		}
	}

	if createCount != 3 {
		t.Errorf("Expected 3 CREATE requests for multi-target, got %d", createCount)
	}

	// Verify all targets are in the aggregated endpoint
	resp, err = suite.makeWebhookRequest("GET", "/records", nil)
	if err != nil {
		t.Fatalf("Failed to get records: %v", err)
	}
	defer resp.Body.Close()

	var endpoints []*endpoint.Endpoint
	if err := json.NewDecoder(resp.Body).Decode(&endpoints); err != nil {
		t.Fatalf("Failed to decode endpoints: %v", err)
	}

	if len(endpoints) != 1 {
		t.Errorf("Expected 1 aggregated endpoint, got %d", len(endpoints))
	} else {
		ep := endpoints[0]
		if len(ep.Targets) != 3 {
			t.Errorf("Expected 3 targets in aggregated endpoint, got %d", len(ep.Targets))
		}

		// Verify all expected targets are present
		targetMap := make(map[string]bool)
		for _, target := range ep.Targets {
			targetMap[target] = true
		}

		expectedTargets := []string{"192.0.2.10", "192.0.2.11", "192.0.2.12"}
		for _, expected := range expectedTargets {
			if !targetMap[expected] {
				t.Errorf("Expected target %s not found in aggregated endpoint", expected)
			}
		}
	}
}

// testComplexUpdateScenarios tests various update patterns
func testComplexUpdateScenarios(t *testing.T, suite *IntegrationTestSuite) {
	// Setup initial multi-target record
	initialRecord := mikrotik.DNSRecord{
		Name:    "complex.example.com",
		Type:    "A",
		Address: "192.0.2.20",
		TTL:     "3600s",
		Comment: defaultComment,
	}
	suite.mockServer.AddRecord(initialRecord)

	anotherRecord := mikrotik.DNSRecord{
		Name:    "complex.example.com",
		Type:    "A",
		Address: "192.0.2.21",
		TTL:     "3600s",
		Comment: defaultComment,
	}
	suite.mockServer.AddRecord(anotherRecord)

	// Test 1: Partial update - remove one target, add two new ones
	updateChanges := &plan.Changes{
		UpdateOld: []*endpoint.Endpoint{
			{
				DNSName:    "complex.example.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.20", "192.0.2.21"},
			},
		},
		UpdateNew: []*endpoint.Endpoint{
			{
				DNSName:    "complex.example.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.21", "192.0.2.22", "192.0.2.23"}, // Keep 21, remove 20, add 22, 23
			},
		},
	}

	suite.mockServer.ClearRequests()
	resp, err := suite.makeWebhookRequest("POST", "/records", updateChanges)
	if err != nil {
		t.Fatalf("Failed to update record: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}

	// Verify smart update logic: should GET current state, DELETE obsolete, CREATE new
	requests := suite.mockServer.GetRequests()
	foundGet := false
	deleteCount := 0
	createCount := 0

	for _, req := range requests {
		if req.Method == "GET" && req.Path == "/rest/ip/dns/static" {
			foundGet = true
		}
		if req.Method == "DELETE" {
			deleteCount++
		}
		if req.Method == "PUT" && req.Path == "/rest/ip/dns/static" {
			createCount++
		}
	}

	if !foundGet {
		t.Error("Expected GET request for smart update not found")
	}
	// Should delete 192.0.2.20 (1 DELETE)
	if deleteCount != 1 {
		t.Errorf("Expected 1 DELETE request, got %d", deleteCount)
	}
	// Should create 192.0.2.22 and 192.0.2.23 (2 CREATEs)
	if createCount != 2 {
		t.Errorf("Expected 2 CREATE requests, got %d", createCount)
	}
}

// testDomainFilterEnforcement tests that domain filtering works correctly
func testDomainFilterEnforcement(t *testing.T, suite *IntegrationTestSuite) {
	// Setup records both inside and outside domain filter
	insideRecord := mikrotik.DNSRecord{
		Name:    "inside.example.com",
		Type:    "A",
		Address: "192.0.2.30",
		TTL:     "3600s",
		Comment: defaultComment,
	}
	suite.mockServer.AddRecord(insideRecord)

	outsideRecord := mikrotik.DNSRecord{
		Name:    "outside.notallowed.com",
		Type:    "A",
		Address: "192.0.2.31",
		TTL:     "3600s",
		Comment: defaultComment,
	}
	suite.mockServer.AddRecord(outsideRecord)

	// Test 1: GET /records should only return allowed domains
	resp, err := suite.makeWebhookRequest("GET", "/records", nil)
	if err != nil {
		t.Fatalf("Failed to get records: %v", err)
	}
	defer resp.Body.Close()

	var endpoints []*endpoint.Endpoint
	if err := json.NewDecoder(resp.Body).Decode(&endpoints); err != nil {
		t.Fatalf("Failed to decode endpoints: %v", err)
	}

	// Should only return the record within allowed domains
	if len(endpoints) != 1 {
		t.Errorf("Expected 1 endpoint (filtered), got %d", len(endpoints))
	} else if endpoints[0].DNSName != "inside.example.com" {
		t.Errorf("Expected inside.example.com, got %s", endpoints[0].DNSName)
	}

	// Test 2: Attempt to create record outside domain filter should fail
	createChanges := &plan.Changes{
		Create: []*endpoint.Endpoint{
			{
				DNSName:    "evil.malicious.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.666"},
			},
		},
	}

	resp, err = suite.makeWebhookRequest("POST", "/records", createChanges)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	resp.Body.Close()

	// Should return error status
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status 500 for domain filter violation, got %d", resp.StatusCode)
	}

	// Test 3: Attempt to delete record outside domain filter should fail
	deleteChanges := &plan.Changes{
		Delete: []*endpoint.Endpoint{
			{
				DNSName:    "evil.malicious.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.666"},
			},
		},
	}

	resp, err = suite.makeWebhookRequest("POST", "/records", deleteChanges)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status 500 for domain filter violation, got %d", resp.StatusCode)
	}
}

// testProviderSpecificProperties tests MikroTik-specific DNS record properties
func testProviderSpecificProperties(t *testing.T, suite *IntegrationTestSuite) {
	// Create record with provider-specific properties
	createChanges := &plan.Changes{
		Create: []*endpoint.Endpoint{
			{
				DNSName:    "custom.example.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.40"},
				RecordTTL:  endpoint.TTL(7200),
				ProviderSpecific: []endpoint.ProviderSpecificProperty{
					{Name: "comment", Value: "Custom Comment"},
					{Name: "disabled", Value: "true"},
					{Name: "address-list", Value: "mylist"},
				},
			},
		},
	}

	resp, err := suite.makeWebhookRequest("POST", "/records", createChanges)
	if err != nil {
		t.Fatalf("Failed to create record with custom properties: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}

	// Verify the request body contained the custom properties
	requests := suite.mockServer.GetRequests()
	var createRequest *RequestCapture
	for _, req := range requests {
		if req.Method == "PUT" && req.Path == "/rest/ip/dns/static" {
			createRequest = &req
			break
		}
	}

	if createRequest == nil {
		t.Fatal("CREATE request not found")
	}

	var record mikrotik.DNSRecord
	if err := json.Unmarshal(createRequest.Body, &record); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	// Verify custom properties were included
	// Note: Comment should be overridden by provider default for security
	if record.Comment != defaultComment {
		t.Errorf("Expected comment to be overridden to '%s', got '%s'", defaultComment, record.Comment)
	}
	if record.Disabled != "true" {
		t.Errorf("Expected disabled='true', got '%s'", record.Disabled)
	}
	if record.AddressList != "mylist" {
		t.Errorf("Expected address-list='mylist', got '%s'", record.AddressList)
	}
}
