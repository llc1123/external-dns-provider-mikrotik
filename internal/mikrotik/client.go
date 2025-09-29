// Rest API Docs: https://help.mikrotik.com/docs/display/ROS/REST+API

package mikrotik

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/publicsuffix"
	"sigs.k8s.io/external-dns/endpoint"
)

type MikrotikDefaults struct {
	DefaultTTL     int64  `env:"MIKROTIK_DEFAULT_TTL" envDefault:"3600"`
	DefaultComment string `env:"MIKROTIK_DEFAULT_COMMENT" envDefault:"Managed By ExternalDNS"`
}

// MikrotikConnectionConfig holds the connection details for the API client
type MikrotikConnectionConfig struct {
	BaseUrl       string `env:"MIKROTIK_BASEURL,notEmpty"`
	Username      string `env:"MIKROTIK_USERNAME,notEmpty"`
	Password      string `env:"MIKROTIK_PASSWORD,notEmpty"`
	SkipTLSVerify bool   `env:"MIKROTIK_SKIP_TLS_VERIFY" envDefault:"false"`
}

// MikrotikApiClient encapsulates the client configuration and HTTP client
type MikrotikApiClient struct {
	*MikrotikDefaults
	*MikrotikConnectionConfig
	*http.Client
	deleteMutex sync.Mutex // Global lock to prevent concurrent delete operations
}

// MikrotikSystemInfo represents MikroTik system information
// https://help.mikrotik.com/docs/display/ROS/Resource
type MikrotikSystemInfo struct {
	ArchitectureName     string `json:"architecture-name"`
	BadBlocks            string `json:"bad-blocks"`
	BoardName            string `json:"board-name"`
	BuildTime            string `json:"build-time"`
	CPU                  string `json:"cpu"`
	CPUCount             string `json:"cpu-count"`
	CPUFrequency         string `json:"cpu-frequency"`
	CPULoad              string `json:"cpu-load"`
	FactorySoftware      string `json:"factory-software"`
	FreeHDDSpace         string `json:"free-hdd-space"`
	FreeMemory           string `json:"free-memory"`
	Platform             string `json:"platform"`
	TotalHDDSpace        string `json:"total-hdd-space"`
	TotalMemory          string `json:"total-memory"`
	Uptime               string `json:"uptime"`
	Version              string `json:"version"`
	WriteSectSinceReboot string `json:"write-sect-since-reboot"`
	WriteSectTotal       string `json:"write-sect-total"`
}

// NewMikrotikClient creates a new instance of MikrotikApiClient
func NewMikrotikClient(config *MikrotikConnectionConfig, defaults *MikrotikDefaults) (*MikrotikApiClient, error) {
	log.Infof("creating a new Mikrotik API Client")

	// Validate that DefaultComment is not empty
	if defaults.DefaultComment == "" {
		return nil, fmt.Errorf("DefaultComment cannot be empty - it's required to identify records managed by external-dns")
	}

	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		log.Errorf("failed to create cookie jar: %v", err)
		return nil, err
	}

	client := &MikrotikApiClient{
		MikrotikDefaults:         defaults,
		MikrotikConnectionConfig: config,
		Client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: config.SkipTLSVerify,
				},
			},
			Jar: jar,
		},
	}

	return client, nil
}

// doRequest sends an HTTP request to the MikroTik API with credentials
// queryParams will be URL-encoded and appended to the path
func (c *MikrotikApiClient) doRequest(method, path string, queryParams url.Values, body io.Reader) (*http.Response, error) {
	// Build URL with query parameters
	baseURL := fmt.Sprintf("%s/rest/%s", c.BaseUrl, path)

	// Add query parameters if provided
	if len(queryParams) > 0 {
		baseURL += "?" + queryParams.Encode()
	}

	log.Debugf("sending %s request to: %s", method, baseURL)

	req, err := http.NewRequest(method, baseURL, body)
	if err != nil {
		log.Errorf("failed to create HTTP request: %v", err)
		return nil, err
	}

	req.SetBasicAuth(c.Username, c.Password)

	resp, err := c.Do(req)
	if err != nil {
		log.Errorf("error sending HTTP request: %v", err)
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		respBody, _ := io.ReadAll(resp.Body)
		log.Errorf("request failed with status %s, response: %s", resp.Status, string(respBody))
		return nil, fmt.Errorf("request failed: %s", resp.Status)
	}
	log.Debugf("request succeeded with status %s", resp.Status)

	return resp, nil
}

// GetSystemInfo fetches system information from the MikroTik API
func (c *MikrotikApiClient) GetSystemInfo() (*MikrotikSystemInfo, error) {
	log.Debugf("fetching system information.")

	// Send the request
	resp, err := c.doRequest(http.MethodGet, "system/resource", nil, nil)
	if err != nil {
		log.Errorf("error fetching system info: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Parse the response
	var info MikrotikSystemInfo
	if err = json.NewDecoder(resp.Body).Decode(&info); err != nil {
		log.Errorf("error decoding response body: %v", err)
		return nil, err
	}
	log.Debugf("got system info: %+v", info)

	return &info, nil
}

// GetDNSRecordsByName fetches DNS records filtered by name and comment from the MikroTik API
// Uses server-side filtering for better performance
// If name is empty, fetches all records managed by external-dns
func (c *MikrotikApiClient) GetDNSRecordsByName(name string) ([]DNSRecord, error) {
	// Build query parameters for server-side filtering
	queryParams := url.Values{}
	queryParams.Set("type", "A,AAAA,CNAME,TXT,MX,SRV,NS")
	queryParams.Set("comment", c.DefaultComment)

	// Add name filter if specified
	if name != "" {
		queryParams.Set("name", name)
		log.Debugf("fetching DNS records for name: %s", name)
	} else {
		log.Debugf("fetching all DNS records managed by external-dns")
	}

	// Send the request
	resp, err := c.doRequest(http.MethodGet, "ip/dns/static", queryParams, nil)
	if err != nil {
		log.Errorf("error fetching DNS records: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Parse the response
	var records []DNSRecord
	if err = json.NewDecoder(resp.Body).Decode(&records); err != nil {
		log.Errorf("error decoding response body: %v", err)
		return nil, err
	}

	log.Debugf("fetched %d DNS records using server-side filtering", len(records))
	return records, nil
}

// DeleteDNSRecords deletes all DNS records associated with an endpoint
func (c *MikrotikApiClient) DeleteDNSRecords(endpoint *endpoint.Endpoint) error {
	// Use global lock to prevent concurrent delete operations
	c.deleteMutex.Lock()
	defer c.deleteMutex.Unlock()

	log.Infof("deleting DNS records for endpoint: %+v", endpoint)

	// Find records that match this endpoint using server-side filtering for better performance
	allRecords, err := c.GetDNSRecordsByName(endpoint.DNSName)
	if err != nil {
		return fmt.Errorf("failed to get DNS records for %s: %w", endpoint.DNSName, err)
	}

	// Find matching records based on name, type, and optionally specific targets
	var recordsToDelete []DNSRecord
	for _, record := range allRecords {
		log.Debugf("Checking record: Name='%s', Type='%s', Comment='%s' against DNSName='%s', RecordType='%s'",
			record.Name, record.Type, record.Comment, endpoint.DNSName, endpoint.RecordType)

		// SECURITY: Strict matching - must match name and type
		if record.Name == endpoint.DNSName && record.Type == endpoint.RecordType {
			log.Debugf("Found matching record: %s (ID: %s, Comment: '%s')", record.Name, record.ID, record.Comment)

			// Only delete records with matching default comment (managed by external-dns)
			if record.Comment == c.DefaultComment {
				// If specific targets are provided, only delete records with matching targets
				if len(endpoint.Targets) > 0 {
					recordTarget := getRecordTarget(&record)
					if recordTarget != "" {
						// Check if this record's target is in the list of targets to delete
						for _, targetToDelete := range endpoint.Targets {
							if recordTarget == targetToDelete {
								log.Debugf("Target matches: '%s', adding to delete list", recordTarget)
								recordsToDelete = append(recordsToDelete, record)
								break
							}
						}
					}
				} else {
					// No specific targets provided, delete all records with matching name/type/comment
					log.Debugf("No specific targets provided, adding all matching records to delete list")
					recordsToDelete = append(recordsToDelete, record)
				}
			} else {
				// Skip records with different comments - they may not be managed by external-dns
				log.Debugf("Skipping record with different comment: %s (expected: '%s', found: '%s')",
					record.Name, c.DefaultComment, record.Comment)
			}
		}
	}

	if len(recordsToDelete) == 0 {
		log.Warnf("No DNS records found to delete for endpoint %s", endpoint.DNSName)
		return nil
	}

	// Delete records one by one with re-verification for each deletion
	// This is necessary because MikroTik reorders IDs after each deletion
	for i, record := range recordsToDelete {
		log.Debugf("deleting DNS record %d/%d: %s", i+1, len(recordsToDelete), record.ID)

		// Before each deletion, re-fetch current records to get updated IDs
		// This is important because previous deletions may have changed the ID numbering
		if i > 0 {
			log.Debugf("re-fetching records to get updated IDs after previous deletions")
			currentRecords, err := c.GetDNSRecordsByName(endpoint.DNSName)
			if err != nil {
				log.Errorf("failed to re-fetch DNS records during deletion: %v", err)
				return fmt.Errorf("failed to re-fetch records during deletion: %w", err)
			}

			// Find the current record that matches the original record we want to delete
			// Since ID may have changed, we match by name, type, and other properties
			var updatedRecord *DNSRecord

			for _, currentRecord := range currentRecords {
				if c.recordsMatch(&record, &currentRecord) {
					updatedRecord = &currentRecord
					break
				}
			}
			if updatedRecord == nil {
				log.Warnf("Record %s no longer exists (may have been deleted already), skipping", record.Name)
				continue
			}

			// Use the updated record ID
			record = *updatedRecord
			log.Debugf("using updated record ID: %s", record.ID)
		}

		// Perform the actual deletion
		resp, err := c.doRequest(http.MethodDelete, fmt.Sprintf("ip/dns/static/%s", record.ID), nil, nil)
		if err != nil {
			log.Errorf("error deleting DNS record %s: %v", record.ID, err)
			return err
		}
		resp.Body.Close()
		log.Debugf("record deleted: %s", record.ID)
	}

	log.Infof("successfully deleted %d DNS records", len(recordsToDelete))
	return nil
}

// recordsMatch checks if two DNS records represent the same logical record
// This is used to find records after ID changes due to deletions
func (c *MikrotikApiClient) recordsMatch(record1, record2 *DNSRecord) bool {
	// Match by name, type, and all record-specific fields
	if record1.Name != record2.Name || record1.Type != record2.Type {
		return false
	}

	// Match by target values based on record type
	switch record1.Type {
	case "A", "AAAA":
		return record1.Address == record2.Address
	case "CNAME":
		return record1.CName == record2.CName
	case "TXT":
		return record1.Text == record2.Text
	case "MX":
		return record1.MXExchange == record2.MXExchange && record1.MXPreference == record2.MXPreference
	case "SRV":
		return record1.SrvTarget == record2.SrvTarget &&
			record1.SrvPort == record2.SrvPort &&
			record1.SrvPriority == record2.SrvPriority &&
			record1.SrvWeight == record2.SrvWeight
	case "NS":
		return record1.NS == record2.NS
	default:
		// For unknown record types, match by comment as well
		return record1.Comment == record2.Comment
	}
}

// CreateDNSRecords creates multiple DNS records in batch (one API call per record)
func (c *MikrotikApiClient) CreateDNSRecords(ep *endpoint.Endpoint) ([]*DNSRecord, error) {
	log.Infof("creating DNS records for endpoint: %+v", ep)

	// Convert endpoint to multiple DNS records
	records, err := NewDNSRecords(ep)
	if err != nil {
		return nil, fmt.Errorf("failed to convert endpoint to DNS records: %w", err)
	}

	// Ensure all records use the DefaultComment (managed by external-dns)
	for _, record := range records {
		record.Comment = c.DefaultComment
		log.Debugf("Set comment to DefaultComment '%s' for record %s", c.DefaultComment, record.Name)
	}

	var createdRecords []*DNSRecord
	for i, record := range records {
		log.Debugf("creating DNS record %d/%d: %+v", i+1, len(records), record)

		createdRecord, err := c.createSingleDNSRecord(record)
		if err != nil {
			// If we've partially created records, we should clean up
			// For now, we'll just log the error and continue
			log.Errorf("failed to create DNS record %d: %v", i+1, err)
			return createdRecords, fmt.Errorf("failed to create record %d: %w", i+1, err)
		}

		createdRecords = append(createdRecords, createdRecord)
	}

	log.Infof("successfully created %d DNS records", len(createdRecords))
	return createdRecords, nil
}

// createSingleDNSRecord creates a single DNS record via API
func (c *MikrotikApiClient) createSingleDNSRecord(record *DNSRecord) (*DNSRecord, error) {
	log.Debugf("creating single DNS record: %+v", record)

	// Serialize the data to JSON to be sent to the API
	jsonBody, err := json.Marshal(record)
	if err != nil {
		log.Errorf("error marshalling DNS record: %v", err)
		return nil, err
	}

	// Send the request
	resp, err := c.doRequest(http.MethodPut, "ip/dns/static", nil, bytes.NewReader(jsonBody))
	if err != nil {
		log.Errorf("error creating DNS record: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Parse the response
	var createdRecord DNSRecord
	if err = json.NewDecoder(resp.Body).Decode(&createdRecord); err != nil {
		log.Errorf("Error decoding response body: %v", err)
		return nil, err
	}
	log.Debugf("created record: %+v", createdRecord)

	return &createdRecord, nil
}

// getRecordTarget extracts the target value from a DNS record based on its type
func getRecordTarget(record *DNSRecord) string {
	switch record.Type {
	case "A", "AAAA":
		return record.Address
	case "CNAME":
		return record.CName
	case "TXT":
		return record.Text
	case "MX":
		return record.MXExchange
	case "SRV":
		return record.SrvTarget
	case "NS":
		return record.NS
	default:
		return ""
	}
}
