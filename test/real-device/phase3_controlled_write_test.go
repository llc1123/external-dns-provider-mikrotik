package real_device

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/external-dns/endpoint"
)

// TestPhase3_ControlledWriteOperations Phase 3: Controlled Create/Delete Tests
// This test uses specific test domain prefixes for safe create and delete operations
func TestPhase3_ControlledWriteOperations(t *testing.T) {
	log.Info("=== Starting Phase 3: Controlled Create/Delete Tests ===")

	suite := NewRealDeviceTestSuite(t)

	// Initialize client
	err := suite.InitializeClient()
	if err != nil {
		t.Fatalf("Failed to initialize client: %v", err)
	}

	// Clean up any existing test records before testing
	t.Run("PreTestCleanup", func(t *testing.T) {
		log.Info("Cleaning existing test records before testing...")

		err := suite.CleanupTestRecords()
		if err != nil {
			t.Fatalf("Failed to cleanup test records: %v", err)
		}

		log.Info("✓ Pre-test cleanup completed")
	})

	// Test 1: Basic A record creation and deletion
	t.Run("BasicARecordCreateDelete", func(t *testing.T) {
		log.Info("Testing basic A record creation and deletion...")

		testDomain := GenerateTestDomainName("basic-a")
		testIP := "192.0.2.100" // Use TEST-NET-1 address block

		// Create record
		endpoint := &endpoint.Endpoint{
			DNSName:    testDomain,
			RecordType: "A",
			Targets:    []string{testIP},
			RecordTTL:  endpoint.TTL(3600),
		}

		log.Infof("Create A record: %s -> %s", testDomain, testIP)

		_, err := suite.client.CreateDNSRecords(endpoint)
		if err != nil {
			t.Fatalf("Failed to create A record: %v", err)
		}

		// Wait for DNS propagation
		suite.WaitForDNSPropagation()

		// Verify if record was created
		err = suite.AssertRecordExists(testDomain, "A", testIP)
		if err != nil {
			t.Fatalf("Created record not found: %v", err)
		}

		log.Info("✓ A record created successfully, verification passed")

		// Delete record
		log.Infof("Delete A record: %s", testDomain)

		err = suite.client.DeleteDNSRecords(endpoint)
		if err != nil {
			t.Fatalf("Failed to delete A record: %v", err)
		}

		// Wait for deletion propagation
		suite.WaitForDNSPropagation()

		// Verify if record was deleted
		err = suite.AssertRecordNotExists(testDomain, "A", testIP)
		if err != nil {
			t.Fatalf("Record was not deleted: %v", err)
		}

		log.Info("✓ A record deleted successfully, verification passed")
	})

	// Test 2: Multiple record types creation and deletion
	t.Run("MultipleRecordTypesCreateDelete", func(t *testing.T) {
		log.Info("Testing multiple record types creation and deletion...")

		testCases := []struct {
			name       string
			recordType string
			target     string
		}{
			{"basic-a2", "A", "192.0.2.101"},
			{"basic-aaaa", "AAAA", "2001:db8::1"},
			{"basic-cname", "CNAME", "target.example.com"},
			{"basic-txt", "TXT", "v=spf1 include:_spf.example.com ~all"},
		}

		var createdEndpoints []*endpoint.Endpoint

		// Create all records
		for _, tc := range testCases {
			testDomain := GenerateTestDomainName(tc.name)

			ep := &endpoint.Endpoint{
				DNSName:    testDomain,
				RecordType: tc.recordType,
				Targets:    []string{tc.target},
				RecordTTL:  endpoint.TTL(3600),
			}

			log.Infof("Create %s record: %s -> %s", tc.recordType, testDomain, tc.target)

			_, err := suite.client.CreateDNSRecords(ep)
			if err != nil {
				t.Errorf("Failed to create %s record for %s: %v", tc.recordType, testDomain, err)
				continue
			}

			createdEndpoints = append(createdEndpoints, ep)

			// Wait for propagation
			suite.WaitForDNSPropagation()

			// Verify creation
			err = suite.AssertRecordExists(testDomain, tc.recordType, tc.target)
			if err != nil {
				t.Errorf("Created %s record not found: %v", tc.recordType, err)
			} else {
				log.Infof("✓ %s record creation verification passed", tc.recordType)
			}
		}

		log.Infof("Successfully created %d records", len(createdEndpoints))

		// Delete all created records
		log.Info("Deleting all created records...")

		for _, ep := range createdEndpoints {
			err := suite.client.DeleteDNSRecords(ep)
			if err != nil {
				t.Errorf("Failed to delete record %s: %v", ep.DNSName, err)
				continue
			}

			// Wait for propagation
			suite.WaitForDNSPropagation()

			// Verify deletion
			err = suite.AssertRecordNotExists(ep.DNSName, ep.RecordType, ep.Targets[0])
			if err != nil {
				t.Errorf("Record was not deleted %s: %v", ep.DNSName, err)
			} else {
				log.Infof("✓ %s record deletion verification passed", ep.RecordType)
			}
		}

		log.Info("✓ Multiple record types test completed")
	})

	// Test 3: Multi-target A record test
	t.Run("MultiTargetARecord", func(t *testing.T) {
		log.Info("Testing multi-target A record...")

		testDomain := GenerateTestDomainName("multi-target")
		testIPs := []string{"192.0.2.110", "192.0.2.111", "192.0.2.112"}

		// Create multi-target record
		endpoint := &endpoint.Endpoint{
			DNSName:    testDomain,
			RecordType: "A",
			Targets:    testIPs,
			RecordTTL:  endpoint.TTL(3600),
		}

		log.Infof("Create multi-target A record: %s -> %v", testDomain, testIPs)

		_, err := suite.client.CreateDNSRecords(endpoint)
		if err != nil {
			t.Fatalf("Failed to create multi-target A record: %v", err)
		}

		// Wait for propagation
		suite.WaitForDNSPropagation()

		// Verify all targets are created
		for _, ip := range testIPs {
			err = suite.AssertRecordExists(testDomain, "A", ip)
			if err != nil {
				t.Errorf("Multi-target record not found for IP %s: %v", ip, err)
			}
		}

		log.Info("✓ Multi-target A record creation verification passed")

		// Delete multi-target record
		log.Infof("Delete multi-target A record: %s", testDomain)

		err = suite.client.DeleteDNSRecords(endpoint)
		if err != nil {
			t.Fatalf("Failed to delete multi-target A record: %v", err)
		}

		// Wait for propagation
		suite.WaitForDNSPropagation()

		// Verify all targets are deleted
		for _, ip := range testIPs {
			err = suite.AssertRecordNotExists(testDomain, "A", ip)
			if err != nil {
				t.Errorf("Multi-target record was not deleted for IP %s: %v", ip, err)
			}
		}

		log.Info("✓ Multi-target A record deletion verification passed")
	})

	// Test 4: Error handling test
	t.Run("ErrorHandling", func(t *testing.T) {
		log.Info("Testing error handling...")

		// Attempt to delete non-existent record
		nonExistentEndpoint := &endpoint.Endpoint{
			DNSName:    GenerateTestDomainName("non-existent"),
			RecordType: "A",
			Targets:    []string{"192.0.2.200"},
		}

		err := suite.client.DeleteDNSRecords(nonExistentEndpoint)
		// Deleting non-existent record should succeed (or at least not crash)
		if err != nil {
			log.Warnf("Error returned when deleting non-existent record (this might be normal): %v", err)
		} else {
			log.Info("✓ Deleting non-existent record handled normally")
		}
	})

	// Post-test cleanup
	t.Run("PostTestCleanup", func(t *testing.T) {
		log.Info("Post-test cleanup of all test records...")

		err := suite.CleanupTestRecords()
		if err != nil {
			t.Fatalf("Failed to cleanup test records: %v", err)
		}

		// Verify cleanup result
		testRecords, err := suite.GetTestRecords()
		if err != nil {
			t.Fatalf("Failed to check cleanup result: %v", err)
		}

		if len(testRecords) > 0 {
			t.Errorf("Cleanup incomplete: %d test records remain", len(testRecords))
			for _, record := range testRecords {
				log.Warnf("  Remaining: %s %s", record.Type, record.Name)
			}
		} else {
			log.Info("✓ Post-test cleanup completed, no remaining test records")
		}
	})

	// Final safety verification
	t.Run("FinalSafetyCheck", func(t *testing.T) {
		log.Info("Final safety check...")

		err := suite.ValidateNoProductionImpact()
		if err != nil {
			t.Fatalf("Final safety check failed: %v", err)
		}

		log.Info("✓ Final safety check passed")
	})

	log.Info("=== Phase 3: Controlled Create/Delete Tests Completed ===")
}
