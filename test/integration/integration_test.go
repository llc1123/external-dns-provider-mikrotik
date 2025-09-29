package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mirceanton/external-dns-provider-mikrotik/internal/mikrotik"
	"github.com/mirceanton/external-dns-provider-mikrotik/pkg/webhook"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/external-dns/endpoint"
)

const (
	mockUsername    = "testuser"
	mockPassword    = "testpass"
	defaultComment  = "external-dns"
	contentTypeJSON = "application/external.dns.webhook+json;version=1"
)

// RequestCapture captures HTTP requests for verification
type RequestCapture struct {
	Method    string
	Path      string
	Query     url.Values
	Headers   http.Header
	Body      []byte
	Timestamp time.Time
}

// MockMikrotikServer provides a mock MikroTik RouterOS API server
type MockMikrotikServer struct {
	server      *httptest.Server
	requests    []RequestCapture
	records     map[string]mikrotik.DNSRecord // ID -> Record
	mu          sync.RWMutex
	nextID      int
	systemInfo  mikrotik.MikrotikSystemInfo
	returnError bool
	errorCode   int
}

// NewMockMikrotikServer creates a new mock MikroTik server
func NewMockMikrotikServer() *MockMikrotikServer {
	mock := &MockMikrotikServer{
		records: make(map[string]mikrotik.DNSRecord),
		nextID:  1,
		systemInfo: mikrotik.MikrotikSystemInfo{
			ArchitectureName:     "arm64",
			BadBlocks:            "0.1",
			BoardName:            "RB5009UG+S+",
			BuildTime:            "2024-09-20 13:00:27",
			CPU:                  "ARM64",
			CPUCount:             "4",
			CPUFrequency:         "1400",
			CPULoad:              "0",
			FactorySoftware:      "7.4.1",
			FreeHDDSpace:         "1019346944",
			FreeMemory:           "916791296",
			Platform:             "MikroTik",
			TotalHDDSpace:        "1073741824",
			TotalMemory:          "1073741824",
			Uptime:               "4d19h9m34s",
			Version:              "7.16 (stable)",
			WriteSectSinceReboot: "5868",
			WriteSectTotal:       "131658",
		},
	}

	mock.server = httptest.NewTLSServer(http.HandlerFunc(mock.handler))
	return mock
}

// Close shuts down the mock server
func (m *MockMikrotikServer) Close() {
	if m.server != nil {
		m.server.Close()
	}
}

// URL returns the mock server URL
func (m *MockMikrotikServer) URL() string {
	return m.server.URL
}

// GetRequests returns captured requests
func (m *MockMikrotikServer) GetRequests() []RequestCapture {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]RequestCapture{}, m.requests...)
}

// ClearRequests clears captured requests
func (m *MockMikrotikServer) ClearRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = nil
}

// SetError configures the server to return errors
func (m *MockMikrotikServer) SetError(returnError bool, errorCode int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.returnError = returnError
	m.errorCode = errorCode
}

// AddRecord adds a DNS record to the mock server
func (m *MockMikrotikServer) AddRecord(record mikrotik.DNSRecord) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := fmt.Sprintf("*%d", m.nextID)
	m.nextID++
	record.ID = id
	m.records[id] = record
	return id
}

// GetRecord retrieves a DNS record by ID
func (m *MockMikrotikServer) GetRecord(id string) (mikrotik.DNSRecord, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record, exists := m.records[id]
	return record, exists
}

// DeleteRecord removes a DNS record by ID
func (m *MockMikrotikServer) DeleteRecord(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.records, id)
}

// GetRecordsByNameAndType retrieves records by name and type
func (m *MockMikrotikServer) GetRecordsByNameAndType(name, recordType string) []mikrotik.DNSRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var results []mikrotik.DNSRecord
	for _, record := range m.records {
		if (name == "" || record.Name == name) &&
			(recordType == "" || record.Type == recordType) &&
			record.Comment == defaultComment {
			results = append(results, record)
		}
	}
	return results
}

// ClearRecords removes all records
func (m *MockMikrotikServer) ClearRecords() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = make(map[string]mikrotik.DNSRecord)
	m.nextID = 1
}

// handler handles HTTP requests to the mock server
func (m *MockMikrotikServer) handler(w http.ResponseWriter, r *http.Request) {
	// Capture request
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body)) // Restore body for processing

	m.mu.Lock()
	m.requests = append(m.requests, RequestCapture{
		Method:    r.Method,
		Path:      r.URL.Path,
		Query:     r.URL.Query(),
		Headers:   r.Header.Clone(),
		Body:      append([]byte{}, body...),
		Timestamp: time.Now(),
	})

	returnError := m.returnError
	errorCode := m.errorCode
	m.mu.Unlock()

	// Check authentication
	username, password, ok := r.BasicAuth()
	if !ok || username != mockUsername || password != mockPassword {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Return configured error if set
	if returnError {
		http.Error(w, http.StatusText(errorCode), errorCode)
		return
	}

	// Route requests
	switch {
	case r.Method == "GET" && r.URL.Path == "/rest/system/resource":
		m.handleGetSystemInfo(w, r)
	case r.Method == "GET" && r.URL.Path == "/rest/ip/dns/static":
		m.handleGetDNSRecords(w, r)
	case r.Method == "PUT" && r.URL.Path == "/rest/ip/dns/static":
		m.handleCreateDNSRecord(w, r)
	case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/rest/ip/dns/static/"):
		m.handleDeleteDNSRecord(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (m *MockMikrotikServer) handleGetSystemInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m.systemInfo)
}

func (m *MockMikrotikServer) handleGetDNSRecords(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	recordType := r.URL.Query().Get("type")
	comment := r.URL.Query().Get("comment")

	m.mu.RLock()
	var results []mikrotik.DNSRecord
	for _, record := range m.records {
		// Apply filters
		if name != "" && record.Name != name {
			continue
		}
		if comment != "" && record.Comment != comment {
			continue
		}
		if recordType != "" {
			// Check if record type matches any of the types in the comma-separated list
			types := strings.Split(recordType, ",")
			matched := false
			for _, t := range types {
				if strings.TrimSpace(t) == record.Type {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		results = append(results, record)
	}
	m.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (m *MockMikrotikServer) handleCreateDNSRecord(w http.ResponseWriter, r *http.Request) {
	var record mikrotik.DNSRecord
	if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	id := fmt.Sprintf("*%d", m.nextID)
	m.nextID++
	record.ID = id
	m.records[id] = record
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(record)
}

func (m *MockMikrotikServer) handleDeleteDNSRecord(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/rest/ip/dns/static/")

	m.mu.Lock()
	delete(m.records, id)
	m.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

// IntegrationTestSuite contains the test suite setup
type IntegrationTestSuite struct {
	mockServer   *MockMikrotikServer
	client       *mikrotik.MikrotikApiClient
	provider     *mikrotik.MikrotikProvider
	webhookSuite *webhook.Webhook
	httpServer   *httptest.Server
	t            *testing.T
}

// NewIntegrationTestSuite creates a new integration test suite
func NewIntegrationTestSuite(t *testing.T) *IntegrationTestSuite {
	// Disable logging during tests to reduce noise
	log.SetLevel(log.FatalLevel)

	suite := &IntegrationTestSuite{
		mockServer: NewMockMikrotikServer(),
		t:          t,
	}

	// Create MikroTik client
	config := &mikrotik.MikrotikConnectionConfig{
		BaseUrl:       suite.mockServer.URL(),
		Username:      mockUsername,
		Password:      mockPassword,
		SkipTLSVerify: true,
	}
	defaults := &mikrotik.MikrotikDefaults{
		DefaultTTL:     3600,
		DefaultComment: defaultComment,
	}

	var err error
	suite.client, err = mikrotik.NewMikrotikClient(config, defaults)
	if err != nil {
		t.Fatalf("Failed to create MikroTik client: %v", err)
	}

	// Create provider
	domainFilter := endpoint.NewDomainFilter([]string{"example.com", "test.com"})
	provider, err := mikrotik.NewMikrotikProvider(domainFilter, defaults, config)
	if err != nil {
		t.Fatalf("Failed to create MikroTik provider: %v", err)
	}
	suite.provider = provider.(*mikrotik.MikrotikProvider)

	// Create webhook
	suite.webhookSuite = webhook.New(suite.provider)

	// Create HTTP server for webhook
	mux := http.NewServeMux()
	mux.HandleFunc("/", suite.webhookSuite.Negotiate)
	mux.HandleFunc("/records", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			suite.webhookSuite.Records(w, r)
		} else if r.Method == "POST" {
			suite.webhookSuite.ApplyChanges(w, r)
		}
	})
	mux.HandleFunc("/adjustendpoints", suite.webhookSuite.AdjustEndpoints)

	suite.httpServer = httptest.NewServer(mux)

	return suite
}

// Close cleans up the test suite
func (s *IntegrationTestSuite) Close() {
	if s.httpServer != nil {
		s.httpServer.Close()
	}
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

// makeWebhookRequest makes an HTTP request to the webhook server
func (s *IntegrationTestSuite) makeWebhookRequest(method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, s.httpServer.URL+path, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentTypeJSON)
	req.Header.Set("Accept", contentTypeJSON)

	return http.DefaultClient.Do(req)
}

// assertRequestCaptured verifies that a specific request was captured
func (s *IntegrationTestSuite) assertRequestCaptured(method, path string, queryParams map[string]string) {
	requests := s.mockServer.GetRequests()
	for _, req := range requests {
		if req.Method == method && req.Path == path {
			// Check query parameters if provided
			if queryParams != nil {
				allMatch := true
				for key, expected := range queryParams {
					if actual := req.Query.Get(key); actual != expected {
						allMatch = false
						break
					}
				}
				if allMatch {
					return // Found matching request
				}
			} else {
				return // Found matching request without query check
			}
		}
	}
	s.t.Errorf("Expected request not found: %s %s with params %v", method, path, queryParams)
}
