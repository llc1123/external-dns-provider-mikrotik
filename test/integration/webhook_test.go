package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/mirceanton/external-dns-provider-mikrotik/internal/mikrotik"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

// TestWebhookNegotiate tests the webhook negotiate endpoint
func TestWebhookNegotiate(t *testing.T) {
	suite := NewIntegrationTestSuite(t)
	defer suite.Close()

	testCases := []struct {
		name           string
		acceptHeader   string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "Valid accept header",
			acceptHeader:   contentTypeJSON,
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Missing accept header",
			acceptHeader:   "",
			expectedStatus: http.StatusNotAcceptable,
			expectError:    true,
		},
		{
			name:           "Invalid accept header",
			acceptHeader:   "application/json",
			expectedStatus: http.StatusUnsupportedMediaType,
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", suite.httpServer.URL+"/", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			if tc.acceptHeader != "" {
				req.Header.Set("Accept", tc.acceptHeader)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
			}

			if !tc.expectError && resp.StatusCode == http.StatusOK {
				// Verify response contains domain filter
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Errorf("Failed to read response body: %v", err)
				}

				var domainFilter endpoint.DomainFilter
				if err := json.Unmarshal(body, &domainFilter); err != nil {
					t.Errorf("Failed to unmarshal domain filter: %v", err)
				}

				// Should contain our configured domains
				if len(domainFilter.Filters) != 2 {
					t.Errorf("Expected 2 domain filters, got %d", len(domainFilter.Filters))
				}
			}
		})
	}
}

// TestWebhookRecords tests the webhook records endpoint
func TestWebhookRecords(t *testing.T) {
	suite := NewIntegrationTestSuite(t)
	defer suite.Close()

	// Add some test records to the mock server
	testRecords := []struct {
		name   string
		record mikrotik.DNSRecord
	}{
		{
			"A record",
			mikrotik.DNSRecord{
				Name:    "test.example.com",
				Type:    "A",
				Address: "192.0.2.1",
				TTL:     "3600s",
				Comment: defaultComment,
			},
		},
		{
			"CNAME record",
			mikrotik.DNSRecord{
				Name:    "www.example.com",
				Type:    "CNAME",
				CName:   "test.example.com",
				TTL:     "3600s",
				Comment: defaultComment,
			},
		},
		{
			"Record outside domain filter",
			mikrotik.DNSRecord{
				Name:    "test.other.com",
				Type:    "A",
				Address: "192.0.2.2",
				TTL:     "3600s",
				Comment: defaultComment,
			},
		},
	}

	for _, tr := range testRecords {
		suite.mockServer.AddRecord(tr.record)
	}

	testCases := []struct {
		name           string
		acceptHeader   string
		expectedStatus int
		expectError    bool
		expectedCount  int // Expected number of endpoints returned
	}{
		{
			name:           "Valid request",
			acceptHeader:   contentTypeJSON,
			expectedStatus: http.StatusOK,
			expectError:    false,
			expectedCount:  2, // Only records within domain filter
		},
		{
			name:           "Missing accept header",
			acceptHeader:   "",
			expectedStatus: http.StatusNotAcceptable,
			expectError:    true,
		},
		{
			name:           "Invalid accept header",
			acceptHeader:   "application/json",
			expectedStatus: http.StatusUnsupportedMediaType,
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			suite.mockServer.ClearRequests()

			req, err := http.NewRequest("GET", suite.httpServer.URL+"/records", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			if tc.acceptHeader != "" {
				req.Header.Set("Accept", tc.acceptHeader)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
			}

			if !tc.expectError && resp.StatusCode == http.StatusOK {
				// Verify MikroTik API was called
				suite.assertRequestCaptured("GET", "/rest/ip/dns/static", map[string]string{
					"comment": defaultComment,
					"type":    "A,AAAA,CNAME,TXT,MX,SRV,NS",
				})

				// Verify response contains endpoints
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Errorf("Failed to read response body: %v", err)
				}

				var endpoints []*endpoint.Endpoint
				if err := json.Unmarshal(body, &endpoints); err != nil {
					t.Errorf("Failed to unmarshal endpoints: %v", err)
				}

				if len(endpoints) != tc.expectedCount {
					t.Errorf("Expected %d endpoints, got %d", tc.expectedCount, len(endpoints))
				}

				// Verify all returned endpoints are within domain filter
				for _, ep := range endpoints {
					if !suite.provider.GetDomainFilter().Match(ep.DNSName) {
						t.Errorf("Endpoint %s is outside domain filter", ep.DNSName)
					}
				}
			}
		})
	}
}

// TestWebhookApplyChanges tests the webhook apply changes endpoint
func TestWebhookApplyChanges(t *testing.T) {
	suite := NewIntegrationTestSuite(t)
	defer suite.Close()

	testCases := []struct {
		name           string
		contentType    string
		changes        *plan.Changes
		expectedStatus int
		expectError    bool
		preSetup       func(*IntegrationTestSuite)
		verifyResult   func(*testing.T, *IntegrationTestSuite)
	}{
		{
			name:        "Create single A record",
			contentType: contentTypeJSON,
			changes: &plan.Changes{
				Create: []*endpoint.Endpoint{
					{
						DNSName:    "new.example.com",
						RecordType: "A",
						Targets:    []string{"192.0.2.10"},
						RecordTTL:  endpoint.TTL(3600),
					},
				},
			},
			expectedStatus: http.StatusNoContent,
			expectError:    false,
			verifyResult: func(t *testing.T, s *IntegrationTestSuite) {
				// Verify CREATE request was made
				s.assertRequestCaptured("PUT", "/rest/ip/dns/static", nil)

				// Verify record was created
				records := s.mockServer.GetRecordsByNameAndType("new.example.com", "A")
				if len(records) != 1 {
					t.Errorf("Expected 1 record created, got %d", len(records))
				}
			},
		},
		{
			name:        "Create multiple A records",
			contentType: contentTypeJSON,
			changes: &plan.Changes{
				Create: []*endpoint.Endpoint{
					{
						DNSName:    "multi.example.com",
						RecordType: "A",
						Targets:    []string{"192.0.2.11", "192.0.2.12"},
						RecordTTL:  endpoint.TTL(3600),
					},
				},
			},
			expectedStatus: http.StatusNoContent,
			expectError:    false,
			verifyResult: func(t *testing.T, s *IntegrationTestSuite) {
				records := s.mockServer.GetRecordsByNameAndType("multi.example.com", "A")
				if len(records) != 2 {
					t.Errorf("Expected 2 records created, got %d", len(records))
				}
			},
		},
		{
			name:        "Delete records",
			contentType: contentTypeJSON,
			changes: &plan.Changes{
				Delete: []*endpoint.Endpoint{
					{
						DNSName:    "delete.example.com",
						RecordType: "A",
						Targets:    []string{"192.0.2.20"},
					},
				},
			},
			expectedStatus: http.StatusNoContent,
			expectError:    false,
			preSetup: func(s *IntegrationTestSuite) {
				// Add record to be deleted
				s.mockServer.AddRecord(mikrotik.DNSRecord{
					Name:    "delete.example.com",
					Type:    "A",
					Address: "192.0.2.20",
					TTL:     "3600s",
					Comment: defaultComment,
				})
			},
			verifyResult: func(t *testing.T, s *IntegrationTestSuite) {
				// Verify DELETE request was made
				s.assertRequestCaptured("DELETE", "/rest/ip/dns/static/*1", nil)

				// Verify record was deleted
				records := s.mockServer.GetRecordsByNameAndType("delete.example.com", "A")
				if len(records) != 0 {
					t.Errorf("Expected 0 records after deletion, got %d", len(records))
				}
			},
		},
		{
			name:        "Update records",
			contentType: contentTypeJSON,
			changes: &plan.Changes{
				UpdateOld: []*endpoint.Endpoint{
					{
						DNSName:    "update.example.com",
						RecordType: "A",
						Targets:    []string{"192.0.2.30"},
					},
				},
				UpdateNew: []*endpoint.Endpoint{
					{
						DNSName:    "update.example.com",
						RecordType: "A",
						Targets:    []string{"192.0.2.31"},
					},
				},
			},
			expectedStatus: http.StatusNoContent,
			expectError:    false,
			preSetup: func(s *IntegrationTestSuite) {
				// Add record to be updated
				s.mockServer.AddRecord(mikrotik.DNSRecord{
					Name:    "update.example.com",
					Type:    "A",
					Address: "192.0.2.30",
					TTL:     "3600s",
					Comment: defaultComment,
				})
			},
			verifyResult: func(t *testing.T, s *IntegrationTestSuite) {
				// Verify both DELETE and CREATE requests were made
				requests := s.mockServer.GetRequests()
				foundDelete := false
				foundCreate := false
				for _, req := range requests {
					if req.Method == "DELETE" && req.Path == "/rest/ip/dns/static/*1" {
						foundDelete = true
					}
					if req.Method == "PUT" && req.Path == "/rest/ip/dns/static" {
						foundCreate = true
					}
				}
				if !foundDelete {
					t.Error("Expected DELETE request not found")
				}
				if !foundCreate {
					t.Error("Expected CREATE request not found")
				}

				// Verify record was updated
				records := s.mockServer.GetRecordsByNameAndType("update.example.com", "A")
				if len(records) != 1 {
					t.Errorf("Expected 1 record after update, got %d", len(records))
				}
				if len(records) > 0 && records[0].Address != "192.0.2.31" {
					t.Errorf("Expected address 192.0.2.31, got %s", records[0].Address)
				}
			},
		},
		{
			name:           "Missing content type",
			contentType:    "",
			changes:        &plan.Changes{},
			expectedStatus: http.StatusNotAcceptable,
			expectError:    true,
		},
		{
			name:           "Invalid content type",
			contentType:    "application/json",
			changes:        &plan.Changes{},
			expectedStatus: http.StatusUnsupportedMediaType,
			expectError:    true,
		},
		{
			name:           "Invalid JSON body",
			contentType:    contentTypeJSON,
			changes:        nil, // Will cause JSON decode error
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:        "Domain filter violation",
			contentType: contentTypeJSON,
			changes: &plan.Changes{
				Create: []*endpoint.Endpoint{
					{
						DNSName:    "evil.malicious.com", // Outside allowed domains
						RecordType: "A",
						Targets:    []string{"192.0.2.666"},
					},
				},
			},
			expectedStatus: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			suite.mockServer.ClearRequests()
			suite.mockServer.ClearRecords()

			// Run pre-setup if provided
			if tc.preSetup != nil {
				tc.preSetup(suite)
			}

			var body io.Reader
			if tc.changes != nil {
				bodyBytes, err := json.Marshal(tc.changes)
				if err != nil {
					t.Fatalf("Failed to marshal changes: %v", err)
				}
				body = bytes.NewReader(bodyBytes)
			} else {
				body = bytes.NewReader([]byte(`invalid json`))
			}

			req, err := http.NewRequest("POST", suite.httpServer.URL+"/records", body)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
			}

			// Run verification if provided
			if tc.verifyResult != nil && !tc.expectError {
				tc.verifyResult(t, suite)
			}
		})
	}
}

// TestWebhookAdjustEndpoints tests the webhook adjust endpoints endpoint
func TestWebhookAdjustEndpoints(t *testing.T) {
	suite := NewIntegrationTestSuite(t)
	defer suite.Close()

	testCases := []struct {
		name           string
		contentType    string
		acceptHeader   string
		endpoints      []*endpoint.Endpoint
		expectedStatus int
		expectError    bool
	}{
		{
			name:         "Valid adjust request",
			contentType:  contentTypeJSON,
			acceptHeader: contentTypeJSON,
			endpoints: []*endpoint.Endpoint{
				{
					DNSName:    "adjust.example.com",
					RecordType: "A",
					Targets:    []string{"192.0.2.100"},
				},
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Missing content type",
			contentType:    "",
			acceptHeader:   contentTypeJSON,
			endpoints:      []*endpoint.Endpoint{},
			expectedStatus: http.StatusNotAcceptable,
			expectError:    true,
		},
		{
			name:           "Missing accept header",
			contentType:    contentTypeJSON,
			acceptHeader:   "",
			endpoints:      []*endpoint.Endpoint{},
			expectedStatus: http.StatusNotAcceptable,
			expectError:    true,
		},
		{
			name:           "Invalid content type",
			contentType:    "application/json",
			acceptHeader:   contentTypeJSON,
			endpoints:      []*endpoint.Endpoint{},
			expectedStatus: http.StatusUnsupportedMediaType,
			expectError:    true,
		},
		{
			name:           "Invalid accept header",
			contentType:    contentTypeJSON,
			acceptHeader:   "application/json",
			endpoints:      []*endpoint.Endpoint{},
			expectedStatus: http.StatusUnsupportedMediaType,
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var body io.Reader
			if tc.endpoints != nil {
				bodyBytes, err := json.Marshal(tc.endpoints)
				if err != nil {
					t.Fatalf("Failed to marshal endpoints: %v", err)
				}
				body = bytes.NewReader(bodyBytes)
			} else {
				body = bytes.NewReader([]byte(`invalid json`))
			}

			req, err := http.NewRequest("POST", suite.httpServer.URL+"/adjustendpoints", body)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			if tc.acceptHeader != "" {
				req.Header.Set("Accept", tc.acceptHeader)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
			}

			if !tc.expectError && resp.StatusCode == http.StatusOK {
				// Verify response contains adjusted endpoints
				bodyBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Errorf("Failed to read response body: %v", err)
				}

				var adjustedEndpoints []*endpoint.Endpoint
				if err := json.Unmarshal(bodyBytes, &adjustedEndpoints); err != nil {
					t.Errorf("Failed to unmarshal adjusted endpoints: %v", err)
				}

				// Should return the same endpoints (no adjustment logic in current implementation)
				if len(adjustedEndpoints) != len(tc.endpoints) {
					t.Errorf("Expected %d adjusted endpoints, got %d", len(tc.endpoints), len(adjustedEndpoints))
				}
			}
		})
	}
}
