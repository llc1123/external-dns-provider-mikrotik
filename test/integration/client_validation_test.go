package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mirceanton/external-dns-provider-mikrotik/internal/mikrotik"
	"sigs.k8s.io/external-dns/endpoint"
)

// TestMikrotikClientRequestValidation tests that the MikroTik client sends correct HTTP requests
func TestMikrotikClientRequestValidation(t *testing.T) {
	suite := NewIntegrationTestSuite(t)
	defer suite.Close()

	testCases := []struct {
		name            string
		operation       func() error
		expectedRequest RequestValidation
	}{
		{
			name: "GetSystemInfo request validation",
			operation: func() error {
				_, err := suite.client.GetSystemInfo()
				return err
			},
			expectedRequest: RequestValidation{
				Method:      "GET",
				Path:        "/rest/system/resource",
				Query:       nil,
				RequireAuth: true,
			},
		},
		{
			name: "GetDNSRecordsByName - all records",
			operation: func() error {
				_, err := suite.client.GetDNSRecordsByName("")
				return err
			},
			expectedRequest: RequestValidation{
				Method: "GET",
				Path:   "/rest/ip/dns/static",
				Query: map[string]string{
					"type":    "A,AAAA,CNAME,TXT,MX,SRV,NS",
					"comment": defaultComment,
				},
				RequireAuth: true,
			},
		},
		{
			name: "GetDNSRecordsByName - specific name",
			operation: func() error {
				_, err := suite.client.GetDNSRecordsByName("test.example.com")
				return err
			},
			expectedRequest: RequestValidation{
				Method: "GET",
				Path:   "/rest/ip/dns/static",
				Query: map[string]string{
					"type":    "A,AAAA,CNAME,TXT,MX,SRV,NS",
					"comment": defaultComment,
					"name":    "test.example.com",
				},
				RequireAuth: true,
			},
		},
		{
			name: "CreateDNSRecords - single A record",
			operation: func() error {
				endpoint := &endpoint.Endpoint{
					DNSName:    "create.example.com",
					RecordType: "A",
					Targets:    []string{"192.0.2.100"},
					RecordTTL:  endpoint.TTL(7200),
				}
				_, err := suite.client.CreateDNSRecords(endpoint)
				return err
			},
			expectedRequest: RequestValidation{
				Method:      "PUT",
				Path:        "/rest/ip/dns/static",
				Query:       nil,
				RequireAuth: true,
				ExpectedBody: mikrotik.DNSRecord{
					Name:    "create.example.com",
					Type:    "A",
					Address: "192.0.2.100",
					TTL:     "2h",
					Comment: defaultComment,
				},
			},
		},
		{
			name: "CreateDNSRecords - CNAME record",
			operation: func() error {
				endpoint := &endpoint.Endpoint{
					DNSName:    "alias.example.com",
					RecordType: "CNAME",
					Targets:    []string{"target.example.com"},
					RecordTTL:  endpoint.TTL(1800),
				}
				_, err := suite.client.CreateDNSRecords(endpoint)
				return err
			},
			expectedRequest: RequestValidation{
				Method:      "PUT",
				Path:        "/rest/ip/dns/static",
				Query:       nil,
				RequireAuth: true,
				ExpectedBody: mikrotik.DNSRecord{
					Name:    "alias.example.com",
					Type:    "CNAME",
					CName:   "target.example.com",
					TTL:     "30m",
					Comment: defaultComment,
				},
			},
		},
		{
			name: "CreateDNSRecords - MX record",
			operation: func() error {
				endpoint := &endpoint.Endpoint{
					DNSName:    "mail.example.com",
					RecordType: "MX",
					Targets:    []string{"10 smtp.example.com"},
					RecordTTL:  endpoint.TTL(3600),
				}
				_, err := suite.client.CreateDNSRecords(endpoint)
				return err
			},
			expectedRequest: RequestValidation{
				Method:      "PUT",
				Path:        "/rest/ip/dns/static",
				Query:       nil,
				RequireAuth: true,
				ExpectedBody: mikrotik.DNSRecord{
					Name:         "mail.example.com",
					Type:         "MX",
					MXExchange:   "smtp.example.com",
					MXPreference: "10",
					TTL:          "1h",
					Comment:      defaultComment,
				},
			},
		},
		{
			name: "CreateDNSRecords - SRV record",
			operation: func() error {
				endpoint := &endpoint.Endpoint{
					DNSName:    "_sip._tcp.example.com",
					RecordType: "SRV",
					Targets:    []string{"10 20 5060 sip.example.com"},
					RecordTTL:  endpoint.TTL(3600),
				}
				_, err := suite.client.CreateDNSRecords(endpoint)
				return err
			},
			expectedRequest: RequestValidation{
				Method:      "PUT",
				Path:        "/rest/ip/dns/static",
				Query:       nil,
				RequireAuth: true,
				ExpectedBody: mikrotik.DNSRecord{
					Name:        "_sip._tcp.example.com",
					Type:        "SRV",
					SrvTarget:   "sip.example.com",
					SrvPort:     "5060",
					SrvPriority: "10",
					SrvWeight:   "20",
					TTL:         "1h",
					Comment:     defaultComment,
				},
			},
		},
		{
			name: "CreateDNSRecords - TXT record",
			operation: func() error {
				endpoint := &endpoint.Endpoint{
					DNSName:    "txt.example.com",
					RecordType: "TXT",
					Targets:    []string{"v=spf1 include:_spf.google.com ~all"},
					RecordTTL:  endpoint.TTL(3600),
				}
				_, err := suite.client.CreateDNSRecords(endpoint)
				return err
			},
			expectedRequest: RequestValidation{
				Method:      "PUT",
				Path:        "/rest/ip/dns/static",
				Query:       nil,
				RequireAuth: true,
				ExpectedBody: mikrotik.DNSRecord{
					Name:    "txt.example.com",
					Type:    "TXT",
					Text:    "v=spf1 include:_spf.google.com ~all",
					TTL:     "1h",
					Comment: defaultComment,
				},
			},
		},
		{
			name: "CreateDNSRecords - NS record",
			operation: func() error {
				endpoint := &endpoint.Endpoint{
					DNSName:    "subdomain.example.com",
					RecordType: "NS",
					Targets:    []string{"ns1.example.com"},
					RecordTTL:  endpoint.TTL(86400),
				}
				_, err := suite.client.CreateDNSRecords(endpoint)
				return err
			},
			expectedRequest: RequestValidation{
				Method:      "PUT",
				Path:        "/rest/ip/dns/static",
				Query:       nil,
				RequireAuth: true,
				ExpectedBody: mikrotik.DNSRecord{
					Name:    "subdomain.example.com",
					Type:    "NS",
					NS:      "ns1.example.com",
					TTL:     "1d",
					Comment: defaultComment,
				},
			},
		},
		{
			name: "CreateDNSRecords - multiple A records",
			operation: func() error {
				endpoint := &endpoint.Endpoint{
					DNSName:    "multi.example.com",
					RecordType: "A",
					Targets:    []string{"192.0.2.10", "192.0.2.11"},
					RecordTTL:  endpoint.TTL(3600),
				}
				_, err := suite.client.CreateDNSRecords(endpoint)
				return err
			},
			expectedRequest: RequestValidation{
				Method:      "PUT",
				Path:        "/rest/ip/dns/static",
				Query:       nil,
				RequireAuth: true,
				// For multiple records, we expect multiple PUT requests
				ExpectMultipleRequests: true,
				ExpectedBodies: []interface{}{
					mikrotik.DNSRecord{
						Name:    "multi.example.com",
						Type:    "A",
						Address: "192.0.2.10",
						TTL:     "1h",
						Comment: defaultComment,
					},
					mikrotik.DNSRecord{
						Name:    "multi.example.com",
						Type:    "A",
						Address: "192.0.2.11",
						TTL:     "1h",
						Comment: defaultComment,
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			suite.mockServer.ClearRequests()

			// Execute operation
			err := tc.operation()
			if err != nil {
				t.Fatalf("Operation failed: %v", err)
			}

			// Validate request(s)
			tc.expectedRequest.Validate(t, suite.mockServer.GetRequests())
		})
	}
}

// TestDeleteDNSRecordsRequestValidation tests delete operation request validation
func TestDeleteDNSRecordsRequestValidation(t *testing.T) {
	suite := NewIntegrationTestSuite(t)
	defer suite.Close()

	testCases := []struct {
		name                string
		setupRecords        []mikrotik.DNSRecord
		deleteEndpoint      *endpoint.Endpoint
		expectedGetRequest  RequestValidation
		expectedDelRequests []RequestValidation
	}{
		{
			name: "Delete single A record",
			setupRecords: []mikrotik.DNSRecord{
				{
					Name:    "delete.example.com",
					Type:    "A",
					Address: "192.0.2.50",
					TTL:     "3600s",
					Comment: defaultComment,
				},
			},
			deleteEndpoint: &endpoint.Endpoint{
				DNSName:    "delete.example.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.50"},
			},
			expectedGetRequest: RequestValidation{
				Method: "GET",
				Path:   "/rest/ip/dns/static",
				Query: map[string]string{
					"type":    "A,AAAA,CNAME,TXT,MX,SRV,NS",
					"comment": defaultComment,
					"name":    "delete.example.com",
				},
				RequireAuth: true,
			},
			expectedDelRequests: []RequestValidation{
				{
					Method:      "DELETE",
					Path:        "/rest/ip/dns/static/*1",
					RequireAuth: true,
				},
			},
		},
		{
			name: "Delete multiple A records",
			setupRecords: []mikrotik.DNSRecord{
				{
					Name:    "multi-del.example.com",
					Type:    "A",
					Address: "192.0.2.60",
					TTL:     "3600s",
					Comment: defaultComment,
				},
				{
					Name:    "multi-del.example.com",
					Type:    "A",
					Address: "192.0.2.61",
					TTL:     "3600s",
					Comment: defaultComment,
				},
			},
			deleteEndpoint: &endpoint.Endpoint{
				DNSName:    "multi-del.example.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.60", "192.0.2.61"},
			},
			expectedGetRequest: RequestValidation{
				Method: "GET",
				Path:   "/rest/ip/dns/static",
				Query: map[string]string{
					"type":    "A,AAAA,CNAME,TXT,MX,SRV,NS",
					"comment": defaultComment,
					"name":    "multi-del.example.com",
				},
				RequireAuth: true,
			},
			expectedDelRequests: []RequestValidation{
				{
					Method:      "DELETE",
					RequireAuth: true,
					// Path validation will be more flexible - just check it's a delete for a static record
					ExpectPathPrefix: "/rest/ip/dns/static/*",
				},
				{
					Method:           "DELETE",
					RequireAuth:      true,
					ExpectPathPrefix: "/rest/ip/dns/static/*",
				},
			},
		},
		{
			name: "Partial delete - only specific targets",
			setupRecords: []mikrotik.DNSRecord{
				{
					Name:    "partial.example.com",
					Type:    "A",
					Address: "192.0.2.70",
					TTL:     "3600s",
					Comment: defaultComment,
				},
				{
					Name:    "partial.example.com",
					Type:    "A",
					Address: "192.0.2.71",
					TTL:     "3600s",
					Comment: defaultComment,
				},
			},
			deleteEndpoint: &endpoint.Endpoint{
				DNSName:    "partial.example.com",
				RecordType: "A",
				Targets:    []string{"192.0.2.70"}, // Only delete this one
			},
			expectedGetRequest: RequestValidation{
				Method: "GET",
				Path:   "/rest/ip/dns/static",
				Query: map[string]string{
					"type":    "A,AAAA,CNAME,TXT,MX,SRV,NS",
					"comment": defaultComment,
					"name":    "partial.example.com",
				},
				RequireAuth: true,
			},
			expectedDelRequests: []RequestValidation{
				{
					Method:      "DELETE",
					Path:        "/rest/ip/dns/static/*1", // Only one should be deleted
					RequireAuth: true,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			suite.mockServer.ClearRequests()
			suite.mockServer.ClearRecords()

			// Setup records
			for _, record := range tc.setupRecords {
				suite.mockServer.AddRecord(record)
			}

			// Execute delete
			err := suite.client.DeleteDNSRecords(tc.deleteEndpoint)
			if err != nil {
				t.Fatalf("Delete operation failed: %v", err)
			}

			requests := suite.mockServer.GetRequests()

			// Validate initial GET request (should be first)
			if len(requests) < 1 {
				t.Fatal("Expected at least 1 request (GET)")
			}
			tc.expectedGetRequest.Validate(t, requests[:1])

			// Filter out DELETE requests (no intermediate GET requests needed since IDs are stable)
			var deleteRequests []RequestCapture
			for i := 1; i < len(requests); i++ { // Skip first GET request
				if requests[i].Method == "DELETE" {
					deleteRequests = append(deleteRequests, requests[i])
				}
			}

			// Validate DELETE requests
			if len(deleteRequests) != len(tc.expectedDelRequests) {
				t.Errorf("Expected %d DELETE requests, got %d", len(tc.expectedDelRequests), len(deleteRequests))
				// Print all requests for debugging
				for i, req := range requests {
					t.Logf("Request %d: %s %s", i, req.Method, req.Path)
				}
			}

			for i, expectedDel := range tc.expectedDelRequests {
				if i < len(deleteRequests) {
					expectedDel.Validate(t, deleteRequests[i:i+1])
				}
			}
		})
	}
}

// RequestValidation defines expected request properties for validation
type RequestValidation struct {
	Method                 string
	Path                   string
	ExpectPathPrefix       string // For flexible path matching
	Query                  map[string]string
	RequireAuth            bool
	ExpectedBody           interface{}
	ExpectedBodies         []interface{}
	ExpectMultipleRequests bool
}

// Validate validates captured requests against expectations
func (rv RequestValidation) Validate(t *testing.T, requests []RequestCapture) {
	if rv.ExpectMultipleRequests {
		// Filter requests by method and path first
		var matchingRequests []RequestCapture
		for _, req := range requests {
			if req.Method == rv.Method && req.Path == rv.Path {
				matchingRequests = append(matchingRequests, req)
			}
		}

		if len(matchingRequests) < len(rv.ExpectedBodies) {
			t.Errorf("Expected at least %d %s %s requests, got %d", len(rv.ExpectedBodies), rv.Method, rv.Path, len(matchingRequests))
			return
		}

		// Check that we can find all expected bodies in the requests
		matchedCount := 0
		for _, expectedBody := range rv.ExpectedBodies {
			for _, req := range matchingRequests {
				if rv.matchesRequestBody(req, expectedBody) && rv.matchesRequestMetadata(req) {
					matchedCount++
					break
				}
			}
		}

		if matchedCount != len(rv.ExpectedBodies) {
			t.Errorf("Expected %d matching request bodies, found %d", len(rv.ExpectedBodies), matchedCount)
			// Log what we actually got for debugging
			for i, req := range matchingRequests {
				t.Logf("Request %d body: %s", i, string(req.Body))
			}
		}
	} else {
		// Validate single request
		if len(requests) == 0 {
			t.Fatal("No requests captured")
		}

		req := requests[0]
		rv.matchesRequest(t, req, rv.ExpectedBody)
	}
}

// matchesRequestBody checks if a request body matches expected JSON
func (rv RequestValidation) matchesRequestBody(req RequestCapture, expectedBody interface{}) bool {
	if expectedBody == nil {
		return len(req.Body) == 0
	}

	if len(req.Body) == 0 {
		return false
	}

	expectedBytes, err := json.Marshal(expectedBody)
	if err != nil {
		return false
	}

	var expected, actual interface{}
	if err := json.Unmarshal(expectedBytes, &expected); err != nil {
		return false
	}
	if err := json.Unmarshal(req.Body, &actual); err != nil {
		return false
	}

	expectedJSON, _ := json.Marshal(expected)
	actualJSON, _ := json.Marshal(actual)

	return string(expectedJSON) == string(actualJSON)
}

// matchesRequestMetadata checks if a request matches metadata (method, path, auth, etc)
func (rv RequestValidation) matchesRequestMetadata(req RequestCapture) bool {
	// Check authentication
	if rv.RequireAuth {
		authHeader := req.Headers.Get("Authorization")
		if authHeader == "" {
			return false
		}
	}

	// Check query parameters
	if rv.Query != nil {
		for key, expectedValue := range rv.Query {
			actualValue := req.Query.Get(key)
			if actualValue != expectedValue {
				return false
			}
		}
	}

	return true
}

// matchesRequest checks if a request matches expected properties
func (rv RequestValidation) matchesRequest(t *testing.T, req RequestCapture, expectedBody interface{}) bool {
	matches := true

	// Validate method
	if req.Method != rv.Method {
		t.Errorf("Expected method %s, got %s", rv.Method, req.Method)
		matches = false
	}

	// Validate path - either exact match or prefix match
	if rv.Path != "" {
		if req.Path != rv.Path {
			t.Errorf("Expected path %s, got %s", rv.Path, req.Path)
			matches = false
		}
	} else if rv.ExpectPathPrefix != "" {
		if !strings.HasPrefix(req.Path, rv.ExpectPathPrefix) {
			t.Errorf("Expected path to start with %s, got %s", rv.ExpectPathPrefix, req.Path)
			matches = false
		}
	}

	// Validate authentication
	if rv.RequireAuth {
		authHeader := req.Headers.Get("Authorization")
		if authHeader == "" {
			t.Error("Expected Authorization header, but not found")
			matches = false
		}
	}

	// Validate query parameters
	if rv.Query != nil {
		for key, expectedValue := range rv.Query {
			actualValue := req.Query.Get(key)
			if actualValue != expectedValue {
				t.Errorf("Expected query param %s=%s, got %s=%s", key, expectedValue, key, actualValue)
				matches = false
			}
		}
	}

	// Validate request body
	if expectedBody != nil && len(req.Body) > 0 {
		expectedBytes, err := json.Marshal(expectedBody)
		if err != nil {
			t.Errorf("Failed to marshal expected body: %v", err)
			matches = false
		} else {
			var expected, actual interface{}
			json.Unmarshal(expectedBytes, &expected)
			json.Unmarshal(req.Body, &actual)

			expectedJSON, _ := json.Marshal(expected)
			actualJSON, _ := json.Marshal(actual)

			if string(expectedJSON) != string(actualJSON) {
				// For multiple requests test, don't log individual mismatches
				// The validation logic handles it at higher level
				if !rv.ExpectMultipleRequests {
					t.Errorf("Request body mismatch.\nExpected: %s\nActual: %s", expectedJSON, actualJSON)
				}
				matches = false
			}
		}
	}

	return matches
}
