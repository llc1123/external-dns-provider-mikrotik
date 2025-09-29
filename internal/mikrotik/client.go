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

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/publicsuffix"
	"sigs.k8s.io/external-dns/endpoint"
)

type MikrotikDefaults struct {
	DefaultTTL     int64  `env:"MIKROTIK_DEFAULT_TTL" envDefault:"3600"`
	DefaultComment string `env:"MIKROTIK_DEFAULT_COMMENT" envDefault:""`
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
func (c *MikrotikApiClient) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	endpoint_url := fmt.Sprintf("%s/rest/%s", c.BaseUrl, path)
	log.Debugf("sending %s request to: %s", method, endpoint_url)

	req, err := http.NewRequest(method, endpoint_url, body)
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
	resp, err := c.doRequest(http.MethodGet, "system/resource", nil)
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

// GetAllDNSRecords fetches all DNS records from the MikroTik API
func (c *MikrotikApiClient) GetAllDNSRecords() ([]DNSRecord, error) {
	log.Debugf("fetching all DNS records")

	// Send the request
	resp, err := c.doRequest(http.MethodGet, "ip/dns/static?type=A,AAAA,CNAME,TXT,MX,SRV,NS", nil)
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

	log.Debugf("fetched %d DNS records: %v", len(records), records)

	return records, nil
}

// DeleteDNSRecords deletes all DNS records associated with an endpoint
func (c *MikrotikApiClient) DeleteDNSRecords(endpoint *endpoint.Endpoint) error {
	log.Infof("deleting DNS records for endpoint: %+v", endpoint)

	// Find all records that match this endpoint
	allRecords, err := c.GetAllDNSRecords()
	if err != nil {
		return fmt.Errorf("failed to get all DNS records: %w", err)
	}

	// Find matching records based on name, type, and default comment
	var recordsToDelete []DNSRecord
	for _, record := range allRecords {
		log.Debugf("Checking record: Name='%s', Type='%s', Comment='%s' against DNSName='%s', RecordType='%s', DefaultComment='%s'",
			record.Name, record.Type, record.Comment, endpoint.DNSName, endpoint.RecordType, c.DefaultComment)

		// SECURITY: Strict matching - must match name, type AND default comment exactly
		if record.Name == endpoint.DNSName && record.Type == endpoint.RecordType {
			log.Debugf("Found matching record: %s (ID: %s, Comment: '%s')", record.Name, record.ID, record.Comment)

			// Only delete records with matching default comment (managed by external-dns)
			if record.Comment == c.DefaultComment {
				log.Debugf("Comment matches default comment: '%s', adding to delete list", record.Comment)
				recordsToDelete = append(recordsToDelete, record)
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

	// Delete each record
	for _, record := range recordsToDelete {
		log.Debugf("deleting DNS record: %s", record.ID)

		resp, err := c.doRequest(http.MethodDelete, fmt.Sprintf("ip/dns/static/%s", record.ID), nil)
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

// CreateDNSRecords creates multiple DNS records in batch (one API call per record)
func (c *MikrotikApiClient) CreateDNSRecords(ep *endpoint.Endpoint) ([]*DNSRecord, error) {
	log.Infof("creating DNS records for endpoint: %+v", ep)

	// Convert endpoint to multiple DNS records
	records, err := NewDNSRecords(ep)
	if err != nil {
		return nil, fmt.Errorf("failed to convert endpoint to DNS records: %w", err)
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
	resp, err := c.doRequest(http.MethodPut, "ip/dns/static", bytes.NewReader(jsonBody))
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

// DeleteDNSRecordByID deletes a single DNS record by its ID
func (c *MikrotikApiClient) DeleteDNSRecordByID(recordID string) error {
	log.Debugf("deleting DNS record by ID: %s", recordID)

	resp, err := c.doRequest("DELETE", fmt.Sprintf("ip/dns/static/%s", recordID), nil)
	if err != nil {
		log.Errorf("failed to delete record %s: %v", recordID, err)
		return fmt.Errorf("failed to delete record %s: %w", recordID, err)
	}
	defer resp.Body.Close()

	log.Debugf("successfully deleted record: %s", recordID)
	return nil
}
