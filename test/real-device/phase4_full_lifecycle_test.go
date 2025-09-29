package real_device

import (
	"fmt"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	ednsendpoint "sigs.k8s.io/external-dns/endpoint"
)

// TestPhase4_FullLifecycleOperations Phase 4: Full lifecycle testing
// This test performs complete CRUD operations, including complex updates and intelligent synchronization tests
func TestPhase4_FullLifecycleOperations(t *testing.T) {
	log.Info("=== Starting Phase 4: Full Lifecycle Testing ===")

	suite := NewRealDeviceTestSuite(t)

	// Initialize client
	err := suite.InitializeClient()
	if err != nil {
		t.Fatalf("Failed to initialize client: %v", err)
	}

	// Pre-test cleanup
	t.Run("PreTestCleanup", func(t *testing.T) {
		log.Info("Pre-test cleanup...")
		err := suite.CleanupTestRecords()
		if err != nil {
			t.Fatalf("Failed to cleanup test records: %v", err)
		}
		log.Info("✓ Pre-test cleanup complete")
	})

	// Test 1: Complete A record lifecycle
	t.Run("FullARecordLifecycle", func(t *testing.T) {
		log.Info("Testing complete A record lifecycle...")

		testDomain := GenerateTestDomainName("lifecycle")

		// Phase 1: Create single-target record
		initialIP := "192.0.2.120"
		endpoint := &ednsendpoint.Endpoint{
			DNSName:    testDomain,
			RecordType: "A",
			Targets:    []string{initialIP},
			RecordTTL:  ednsendpoint.TTL(3600),
		}

		log.Infof("Creating initial record: %s -> %s", testDomain, initialIP)
		_, err := suite.client.CreateDNSRecords(endpoint)
		if err != nil {
			t.Fatalf("Failed to create initial record: %v", err)
		}

		suite.WaitForDNSPropagation()

		// Verify creation
		err = suite.AssertRecordExists(testDomain, "A", initialIP)
		if err != nil {
			t.Fatalf("Initial record not found: %v", err)
		}
		log.Info("✓ Initial record created successfully")

		// Phase 2: Add more targets (simulate update to multi-target)
		updatedIPs := []string{initialIP, "192.0.2.121", "192.0.2.122"}

		// First delete old record
		err = suite.client.DeleteDNSRecords(endpoint)
		if err != nil {
			t.Fatalf("Failed to delete old record: %v", err)
		}

		suite.WaitForDNSPropagation()

		// Create new multi-target record
		updatedEndpoint := &ednsendpoint.Endpoint{
			DNSName:    testDomain,
			RecordType: "A",
			Targets:    updatedIPs,
			RecordTTL:  ednsendpoint.TTL(7200),
		}

		log.Infof("Updating to multi-target record: %s -> %v", testDomain, updatedIPs)
		_, err = suite.client.CreateDNSRecords(updatedEndpoint)
		if err != nil {
			t.Fatalf("Failed to create updated record: %v", err)
		}

		suite.WaitForDNSPropagation()

		// Verify all targets
		for _, ip := range updatedIPs {
			err = suite.AssertRecordExists(testDomain, "A", ip)
			if err != nil {
				t.Errorf("Updated record target %s not found: %v", ip, err)
			}
		}
		log.Info("✓ Record updated to multi-target successfully")

		// Phase 3: Partial update (remove one target, add one new target)
		finalIPs := []string{updatedIPs[0], updatedIPs[2], "192.0.2.123"} // Keep 1st and 3rd, add new one

		// Delete current record
		err = suite.client.DeleteDNSRecords(updatedEndpoint)
		if err != nil {
			t.Fatalf("Failed to delete for partial update: %v", err)
		}

		suite.WaitForDNSPropagation()

		// Create partially updated record
		finalEndpoint := &ednsendpoint.Endpoint{
			DNSName:    testDomain,
			RecordType: "A",
			Targets:    finalIPs,
			RecordTTL:  ednsendpoint.TTL(3600),
		}

		log.Infof("Partial update record: %s -> %v", testDomain, finalIPs)
		_, err = suite.client.CreateDNSRecords(finalEndpoint)
		if err != nil {
			t.Fatalf("Failed to create partially updated record: %v", err)
		}

		suite.WaitForDNSPropagation()

		// Verify final targets
		for _, ip := range finalIPs {
			err = suite.AssertRecordExists(testDomain, "A", ip)
			if err != nil {
				t.Errorf("Final record target %s not found: %v", ip, err)
			}
		}

		// Verify that removed targets do not exist
		err = suite.AssertRecordNotExists(testDomain, "A", updatedIPs[1])
		if err != nil {
			t.Errorf("Removed target still exists: %v", err)
		}

		log.Info("✓ Partial update successful")

		// Phase 4: Final deletion
		log.Infof("Deleting final record: %s", testDomain)
		err = suite.client.DeleteDNSRecords(finalEndpoint)
		if err != nil {
			t.Fatalf("Failed to delete final record: %v", err)
		}

		suite.WaitForDNSPropagation()

		// Verify all targets are deleted
		for _, ip := range finalIPs {
			err = suite.AssertRecordNotExists(testDomain, "A", ip)
			if err != nil {
				t.Errorf("Final record target %s was not deleted: %v", ip, err)
			}
		}

		log.Info("✓ Complete lifecycle test successful")
	})

	// Test 2: Mixed record types management
	t.Run("MixedRecordTypesManagement", func(t *testing.T) {
		log.Info("Testing mixed record types management...")

		baseDomain := GenerateTestDomainName("mixed")

		// Create multiple types of records
		endpoints := []*ednsendpoint.Endpoint{
			{
				DNSName:    baseDomain,
				RecordType: "A",
				Targets:    []string{"192.0.2.130"},
				RecordTTL:  ednsendpoint.TTL(3600),
			},
			{
				DNSName:    "www." + baseDomain,
				RecordType: "CNAME",
				Targets:    []string{baseDomain},
				RecordTTL:  ednsendpoint.TTL(3600),
			},
			{
				DNSName:    baseDomain,
				RecordType: "TXT",
				Targets:    []string{"v=spf1 include:_spf.example.com ~all"},
				RecordTTL:  ednsendpoint.TTL(3600),
			},
			{
				DNSName:    "mail." + baseDomain,
				RecordType: "MX",
				Targets:    []string{"10 smtp.example.com"},
				RecordTTL:  ednsendpoint.TTL(3600),
			},
		}

		// Batch create
		log.Info("Creating mixed type records...")
		for i, ep := range endpoints {
			log.Infof("  Creating %dth: %s %s -> %s", i+1, ep.RecordType, ep.DNSName, ep.Targets[0])
			_, err := suite.client.CreateDNSRecords(ep)
			if err != nil {
				t.Errorf("Failed to create %s record %s: %v", ep.RecordType, ep.DNSName, err)
				continue
			}

			suite.WaitForDNSPropagation()

			// Verify creation
			err = suite.AssertRecordExists(ep.DNSName, ep.RecordType, ep.Targets[0])
			if err != nil {
				t.Errorf("Created record not found: %v", err)
			}
		}

		log.Info("✓ Mixed type record creation complete")

		// Verify all records exist
		log.Info("Verifying all records existence...")
		for _, ep := range endpoints {
			err = suite.AssertRecordExists(ep.DNSName, ep.RecordType, ep.Targets[0])
			if err != nil {
				t.Errorf("Record verification failed: %v", err)
			}
		}
		log.Info("✓ All records existence verification passed")

		// Batch deletion
		log.Info("Deleting mixed type records...")
		for i, ep := range endpoints {
			log.Infof("  Deleting %dth: %s %s", i+1, ep.RecordType, ep.DNSName)
			err := suite.client.DeleteDNSRecords(ep)
			if err != nil {
				t.Errorf("Failed to delete %s record %s: %v", ep.RecordType, ep.DNSName, err)
				continue
			}

			suite.WaitForDNSPropagation()

			// Verify deletion
			err = suite.AssertRecordNotExists(ep.DNSName, ep.RecordType, ep.Targets[0])
			if err != nil {
				t.Errorf("Record was not deleted: %v", err)
			}
		}

		log.Info("✓ Mixed type record deletion complete")
	})

	// Test 3: Complex SRV record management
	t.Run("SRVRecordManagement", func(t *testing.T) {
		log.Info("Testing SRV record management...")

		srvDomain := "_sip._tcp." + GenerateTestDomainName("srv")

		// Create multiple SRV records (different priorities)
		srvTargets := []string{
			"10 5 5060 sip1.example.com",
			"20 5 5060 sip2.example.com",
			"30 10 5060 sip3.example.com",
		}

		// Create SRV records individually
		var createdEndpoints []*ednsendpoint.Endpoint
		for i, target := range srvTargets {
			ep := &ednsendpoint.Endpoint{
				DNSName:    srvDomain,
				RecordType: "SRV",
				Targets:    []string{target},
				RecordTTL:  ednsendpoint.TTL(3600),
			}

			log.Infof("Creating SRV record %d: %s -> %s", i+1, srvDomain, target)
			_, err := suite.client.CreateDNSRecords(ep)
			if err != nil {
				t.Errorf("Failed to create SRV record %d: %v", i+1, err)
				continue
			}

			createdEndpoints = append(createdEndpoints, ep)
			suite.WaitForDNSPropagation()

			// Verify creation
			err = suite.AssertRecordExists(srvDomain, "SRV", target)
			if err != nil {
				t.Errorf("SRV record %d not found: %v", i+1, err)
			}
		}

		log.Info("✓ SRV record creation complete")

		// Delete SRV records
		log.Info("Deleting SRV records...")
		for i, ep := range createdEndpoints {
			log.Infof("Deleting SRV record %d: %s", i+1, ep.DNSName)
			err := suite.client.DeleteDNSRecords(ep)
			if err != nil {
				t.Errorf("Failed to delete SRV record %d: %v", i+1, err)
				continue
			}

			suite.WaitForDNSPropagation()

			// Verify deletion
			err = suite.AssertRecordNotExists(ep.DNSName, ep.RecordType, ep.Targets[0])
			if err != nil {
				t.Errorf("SRV record %d was not deleted: %v", i+1, err)
			}
		}

		log.Info("✓ SRV record deletion complete")
	})

	// Test 4: Stress test (create many records)
	t.Run("StressTestManyRecords", func(t *testing.T) {
		log.Info("Performing stress test (many records)...")

		recordCount := 20 // Moderate number to avoid excessive pressure on device
		var createdEndpoints []*ednsendpoint.Endpoint

		// Create many records
		log.Infof("Creating %d A records...", recordCount)
		for i := 0; i < recordCount; i++ {
			testDomain := GenerateTestDomainName(fmt.Sprintf("stress%d", i))
			testIP := fmt.Sprintf("192.0.2.%d", 140+i) // Use consecutive IP addresses

			ep := &ednsendpoint.Endpoint{
				DNSName:    testDomain,
				RecordType: "A",
				Targets:    []string{testIP},
				RecordTTL:  ednsendpoint.TTL(3600),
			}

			if i%5 == 0 { // Output progress every 5 records
				log.Infof("  Creating record %d/%d: %s -> %s", i+1, recordCount, testDomain, testIP)
			}

			_, err := suite.client.CreateDNSRecords(ep)
			if err != nil {
				t.Errorf("Failed to create stress test record %d: %v", i+1, err)
				continue
			}

			createdEndpoints = append(createdEndpoints, ep)

			// Reduce wait time to speed up testing
			if i < recordCount-1 {
				time.Sleep(100 * time.Millisecond)
			}
		}

		// Wait for final propagation
		suite.WaitForDNSPropagation()

		log.Infof("✓ Successfully created %d records", len(createdEndpoints))

		// Verify partial records (not all to save time)
		verifyCount := 5
		if len(createdEndpoints) < verifyCount {
			verifyCount = len(createdEndpoints)
		}

		log.Infof("Verifying first %d records...", verifyCount)
		for i := 0; i < verifyCount; i++ {
			ep := createdEndpoints[i]
			err = suite.AssertRecordExists(ep.DNSName, ep.RecordType, ep.Targets[0])
			if err != nil {
				t.Errorf("Stress test record %d verification failed: %v", i+1, err)
			}
		}

		log.Info("✓ Record verification passed")

		// Batch deletion
		log.Infof("Deleting %d records...", len(createdEndpoints))
		for i, ep := range createdEndpoints {
			if i%5 == 0 { // Output progress every 5 records
				log.Infof("  Deleting record %d/%d: %s", i+1, len(createdEndpoints), ep.DNSName)
			}

			err := suite.client.DeleteDNSRecords(ep)
			if err != nil {
				t.Errorf("Failed to delete stress test record %d: %v", i+1, err)
				continue
			}

			// Reduce wait time
			if i < len(createdEndpoints)-1 {
				time.Sleep(50 * time.Millisecond)
			}
		}

		suite.WaitForDNSPropagation()
		log.Info("✓ Stress test record deletion complete")
	})

	// Post-test cleanup
	t.Run("PostTestCleanup", func(t *testing.T) {
		log.Info("Post-test final cleanup...")

		err := suite.CleanupTestRecords()
		if err != nil {
			t.Fatalf("Failed to cleanup test records: %v", err)
		}

		// Verify cleanup
		testRecords, err := suite.GetTestRecords()
		if err != nil {
			t.Fatalf("Failed to verify cleanup: %v", err)
		}

		if len(testRecords) > 0 {
			t.Errorf("Cleanup incomplete: %d test records remain", len(testRecords))
		} else {
			log.Info("✓ Final cleanup complete")
		}
	})

	// Final safety check
	t.Run("FinalSafetyCheck", func(t *testing.T) {
		log.Info("Final safety check...")

		err := suite.ValidateNoProductionImpact()
		if err != nil {
			t.Fatalf("Final safety check failed: %v", err)
		}

		log.Info("✓ Final safety check passed")
	})

	log.Info("=== Phase 4: Full Lifecycle Testing Complete ===")
}
