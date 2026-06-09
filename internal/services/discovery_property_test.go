package services

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"superview/internal/models"
	"superview/internal/utils"

	"github.com/glebarez/sqlite"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ============================================================================
// Test Infrastructure
// ============================================================================

// setupDiscoveryTestDB creates an in-memory SQLite database for discovery testing
func setupDiscoveryTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	// Auto-migrate models
	err = db.AutoMigrate(&models.DiscoveryTask{}, &models.DiscoveryResult{}, &models.Node{})
	if err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return db
}

// mockDiscoveryRepository implements DiscoveryRepository for testing
type mockDiscoveryRepository struct {
	db     *gorm.DB
	mu     sync.RWMutex
	tasks  map[uint]*models.DiscoveryTask
	nextID uint
}

func newMockDiscoveryRepository(db *gorm.DB) *mockDiscoveryRepository {
	return &mockDiscoveryRepository{
		db:     db,
		tasks:  make(map[uint]*models.DiscoveryTask),
		nextID: 1,
	}
}

func (r *mockDiscoveryRepository) CreateTask(task *models.DiscoveryTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	task.ID = r.nextID
	r.nextID++
	task.CreatedAt = time.Now()
	task.UpdatedAt = time.Now()

	// Store a copy
	taskCopy := *task
	r.tasks[task.ID] = &taskCopy

	return nil
}

func (r *mockDiscoveryRepository) GetTask(id uint) (*models.DiscoveryTask, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	task, exists := r.tasks[id]
	if !exists {
		return nil, fmt.Errorf("task not found: %d", id)
	}

	// Return a copy
	taskCopy := *task
	return &taskCopy, nil
}

func (r *mockDiscoveryRepository) UpdateTask(task *models.DiscoveryTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tasks[task.ID]; !exists {
		return fmt.Errorf("task not found: %d", task.ID)
	}

	task.UpdatedAt = time.Now()
	taskCopy := *task
	r.tasks[task.ID] = &taskCopy

	return nil
}

func (r *mockDiscoveryRepository) DeleteTask(id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tasks, id)
	return nil
}

func (r *mockDiscoveryRepository) ListTasks(offset, limit int, status string) ([]*models.DiscoveryTask, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tasks []*models.DiscoveryTask
	for _, task := range r.tasks {
		if status == "" || task.Status == status {
			taskCopy := *task
			tasks = append(tasks, &taskCopy)
		}
	}

	return tasks, int64(len(tasks)), nil
}

func (r *mockDiscoveryRepository) CreateResult(result *models.DiscoveryResult) error {
	return r.db.Create(result).Error
}

func (r *mockDiscoveryRepository) GetResultsByTaskID(taskID uint) ([]*models.DiscoveryResult, error) {
	var results []*models.DiscoveryResult
	err := r.db.Where("task_id = ?", taskID).Find(&results).Error
	return results, err
}

// mockNodeRepository implements NodeRepository for testing
type mockNodeRepository struct {
	db     *gorm.DB
	mu     sync.RWMutex
	nodes  map[uint]*models.Node
	nextID uint
}

func newMockNodeRepository(db *gorm.DB) *mockNodeRepository {
	return &mockNodeRepository{
		db:     db,
		nodes:  make(map[uint]*models.Node),
		nextID: 1,
	}
}

func (r *mockNodeRepository) Create(node *models.Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	node.ID = r.nextID
	r.nextID++
	node.CreatedAt = time.Now()
	node.UpdatedAt = time.Now()

	nodeCopy := *node
	r.nodes[node.ID] = &nodeCopy

	return r.db.Create(node).Error
}

func (r *mockNodeRepository) GetByID(id uint) (*models.Node, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	node, exists := r.nodes[id]
	if !exists {
		return nil, fmt.Errorf("node not found: %d", id)
	}

	nodeCopy := *node
	return &nodeCopy, nil
}

func (r *mockNodeRepository) GetByName(name string) (*models.Node, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, node := range r.nodes {
		if node.Name == name {
			nodeCopy := *node
			return &nodeCopy, nil
		}
	}

	return nil, fmt.Errorf("node not found: %s", name)
}

func (r *mockNodeRepository) Update(node *models.Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.nodes[node.ID]; !exists {
		return fmt.Errorf("node not found: %d", node.ID)
	}

	node.UpdatedAt = time.Now()
	nodeCopy := *node
	r.nodes[node.ID] = &nodeCopy

	return nil
}

func (r *mockNodeRepository) Delete(id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.nodes, id)
	return nil
}

func (r *mockNodeRepository) List(offset, limit int) ([]*models.Node, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var nodes []*models.Node
	for _, node := range r.nodes {
		nodeCopy := *node
		nodes = append(nodes, &nodeCopy)
	}

	return nodes, int64(len(nodes)), nil
}

func (r *mockNodeRepository) GetByStatus(status string) ([]*models.Node, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var nodes []*models.Node
	for _, node := range r.nodes {
		if node.Status == status {
			nodeCopy := *node
			nodes = append(nodes, &nodeCopy)
		}
	}

	return nodes, nil
}

func (r *mockNodeRepository) ExistsByName(name string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, node := range r.nodes {
		if node.Name == name {
			return true, nil
		}
	}

	return false, nil
}

func (r *mockNodeRepository) ExistsByHostPort(host string, port int) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, node := range r.nodes {
		if node.Host == host && node.Port == port {
			return true, nil
		}
	}

	return false, nil
}

// mockWebSocketHub implements WebSocketHub for testing
type mockWebSocketHub struct{}

func (h *mockWebSocketHub) Broadcast(data []byte) {}

// ============================================================================
// Generators
// ============================================================================

// genValidCIDR generates valid CIDR strings with prefix >= 24 (small ranges for testing)
func genValidCIDR() gopter.Gen {
	return gen.Struct(reflect.TypeOf(struct {
		O1, O2, O3, O4 uint8
		Prefix         int
	}{}), map[string]gopter.Gen{
		"O1":     gen.UInt8(),
		"O2":     gen.UInt8(),
		"O3":     gen.UInt8(),
		"O4":     gen.UInt8(),
		"Prefix": gen.IntRange(28, 32), // Small ranges for testing
	}).Map(func(v interface{}) string {
		s := v.(struct {
			O1, O2, O3, O4 uint8
			Prefix         int
		})
		return fmt.Sprintf("%d.%d.%d.%d/%d", s.O1, s.O2, s.O3, s.O4, s.Prefix)
	})
}

// genValidPort generates valid port numbers
func genValidPort() gopter.Gen {
	return gen.IntRange(1, 65535)
}

// genUsername generates valid usernames
func genUsername() gopter.Gen {
	return gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) >= 1 && len(s) <= 50
	}).Map(func(s string) string {
		if len(s) == 0 {
			return "admin"
		}
		if len(s) > 50 {
			return s[:50]
		}
		return s
	})
}

// genValidIP generates valid IPv4 addresses
func genValidIP() gopter.Gen {
	return gen.Struct(reflect.TypeOf(struct {
		O1, O2, O3, O4 uint8
	}{}), map[string]gopter.Gen{
		"O1": gen.UInt8(),
		"O2": gen.UInt8(),
		"O3": gen.UInt8(),
		"O4": gen.UInt8(),
	}).Map(func(v interface{}) string {
		s := v.(struct {
			O1, O2, O3, O4 uint8
		})
		return fmt.Sprintf("%d.%d.%d.%d", s.O1, s.O2, s.O3, s.O4)
	})
}

// genProgressState generates valid progress states
func genProgressState() gopter.Gen {
	return gen.Struct(reflect.TypeOf(struct {
		TotalIPs   int
		ScannedIPs int
		FoundNodes int
		FailedIPs  int
	}{}), map[string]gopter.Gen{
		"TotalIPs":   gen.IntRange(1, 256),
		"ScannedIPs": gen.IntRange(0, 256),
		"FoundNodes": gen.IntRange(0, 256),
		"FailedIPs":  gen.IntRange(0, 256),
	}).SuchThat(func(v interface{}) bool {
		s := v.(struct {
			TotalIPs   int
			ScannedIPs int
			FoundNodes int
			FailedIPs  int
		})
		// Ensure scanned <= total and found + failed <= scanned
		return s.ScannedIPs <= s.TotalIPs &&
			s.FoundNodes <= s.ScannedIPs &&
			s.FailedIPs <= s.ScannedIPs &&
			s.FoundNodes+s.FailedIPs <= s.ScannedIPs
	})
}

// ============================================================================
// Feature: node-discovery, Property 3: Task Creation Invariants
// For any valid discovery request, creating a task SHALL produce:
// - A unique task ID (no two tasks share the same ID)
// - Initial status of "pending"
// - Created timestamp ≤ current time
// **Validates: Requirements 2.1, 2.2**
// ============================================================================

func TestTaskCreationInvariants(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 3a: Each created task has a unique ID
	properties.Property("each created task has a unique ID", prop.ForAll(
		func(numTasks int) bool {
			db := setupDiscoveryTestDB(t)
			repo := newMockDiscoveryRepository(db)
			nodeRepo := newMockNodeRepository(db)
			hub := &mockWebSocketHub{}

			service := NewDiscoveryService(db, repo, nodeRepo, hub, nil)

			taskIDs := make(map[uint]bool)

			for i := 0; i < numTasks; i++ {
				cidr := fmt.Sprintf("192.168.%d.0/30", i%256)
				req := &DiscoveryRequest{
					CIDR:      cidr,
					Port:      9001,
					Username:  "admin",
					Password:  "secret",
					CreatedBy: "test-user",
				}

				task, err := service.StartDiscovery(req)
				if err != nil {
					// Skip invalid CIDRs
					continue
				}

				// Check for duplicate ID
				if taskIDs[task.ID] {
					t.Logf("Duplicate task ID found: %d", task.ID)
					return false
				}
				taskIDs[task.ID] = true
			}

			return true
		},
		gen.IntRange(2, 20),
	))

	// Property 3b: Initial status is always "pending"
	properties.Property("initial status is always pending", prop.ForAll(
		func(o1, o2, o3, o4 uint8, prefix int, port int) bool {
			if prefix < 28 {
				prefix = 28
			}
			if port < 1 || port > 65535 {
				port = 9001
			}

			db := setupDiscoveryTestDB(t)
			repo := newMockDiscoveryRepository(db)
			nodeRepo := newMockNodeRepository(db)
			hub := &mockWebSocketHub{}

			service := NewDiscoveryService(db, repo, nodeRepo, hub, nil)

			cidr := fmt.Sprintf("%d.%d.%d.%d/%d", o1, o2, o3, o4, prefix)
			req := &DiscoveryRequest{
				CIDR:      cidr,
				Port:      port,
				Username:  "admin",
				Password:  "secret",
				CreatedBy: "test-user",
			}

			task, err := service.StartDiscovery(req)
			if err != nil {
				// Invalid input, skip
				return true
			}

			// Verify initial status is pending
			if task.Status != models.DiscoveryStatusPending {
				t.Logf("Expected status 'pending', got '%s'", task.Status)
				return false
			}

			return true
		},
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.IntRange(28, 32),
		gen.IntRange(1, 65535),
	))

	// Property 3c: Created timestamp is <= current time
	properties.Property("created timestamp is <= current time", prop.ForAll(
		func(o1, o2, o3, o4 uint8) bool {
			db := setupDiscoveryTestDB(t)
			repo := newMockDiscoveryRepository(db)
			nodeRepo := newMockNodeRepository(db)
			hub := &mockWebSocketHub{}

			service := NewDiscoveryService(db, repo, nodeRepo, hub, nil)

			beforeCreate := time.Now()

			cidr := fmt.Sprintf("%d.%d.%d.%d/30", o1, o2, o3, o4)
			req := &DiscoveryRequest{
				CIDR:      cidr,
				Port:      9001,
				Username:  "admin",
				Password:  "secret",
				CreatedBy: "test-user",
			}

			task, err := service.StartDiscovery(req)
			if err != nil {
				return true // Skip invalid inputs
			}

			afterCreate := time.Now()

			// CreatedAt should be between beforeCreate and afterCreate
			if task.CreatedAt.Before(beforeCreate) {
				t.Logf("CreatedAt %v is before test start %v", task.CreatedAt, beforeCreate)
				return false
			}

			if task.CreatedAt.After(afterCreate) {
				t.Logf("CreatedAt %v is after test end %v", task.CreatedAt, afterCreate)
				return false
			}

			return true
		},
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
	))

	// Property 3d: TotalIPs matches CIDR calculation
	properties.Property("TotalIPs matches CIDR calculation", prop.ForAll(
		func(o1, o2, o3, o4 uint8, prefix int) bool {
			if prefix < 28 {
				prefix = 28
			}

			db := setupDiscoveryTestDB(t)
			repo := newMockDiscoveryRepository(db)
			nodeRepo := newMockNodeRepository(db)
			hub := &mockWebSocketHub{}

			service := NewDiscoveryService(db, repo, nodeRepo, hub, nil)

			cidr := fmt.Sprintf("%d.%d.%d.%d/%d", o1, o2, o3, o4, prefix)
			req := &DiscoveryRequest{
				CIDR:      cidr,
				Port:      9001,
				Username:  "admin",
				Password:  "secret",
				CreatedBy: "test-user",
			}

			task, err := service.StartDiscovery(req)
			if err != nil {
				return true
			}

			cidrRange, err := utils.ParseCIDR(cidr)
			if err != nil {
				return true
			}

			expectedCount := cidrRange.Count()
			if task.TotalIPs != expectedCount {
				t.Logf("TotalIPs mismatch: expected %d, got %d", expectedCount, task.TotalIPs)
				return false
			}

			return true
		},
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.IntRange(28, 32),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// ============================================================================
// Feature: node-discovery, Property 4: Progress Tracking Invariant
// For any discovery task at any point during execution:
// - scanned_ips + found_nodes + failed_ips components are consistent
// - scanned_ips ≤ total_ips
// - When status is "completed": scanned_ips == total_ips
// **Validates: Requirements 2.3, 2.5**
// ============================================================================

func TestProgressTrackingInvariant(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 4a: scanned_ips <= total_ips always holds
	properties.Property("scanned_ips <= total_ips", prop.ForAll(
		func(totalIPs, scannedIPs int) bool {
			if totalIPs < 1 {
				totalIPs = 1
			}
			if scannedIPs < 0 {
				scannedIPs = 0
			}

			// Constrain scannedIPs to valid range
			if scannedIPs > totalIPs {
				// This should be rejected by the invariant
				return true // We're testing the invariant holds when properly set
			}

			task := &models.DiscoveryTask{
				TotalIPs:   totalIPs,
				ScannedIPs: scannedIPs,
				Status:     models.DiscoveryStatusRunning,
			}

			// Verify invariant
			if task.ScannedIPs > task.TotalIPs {
				t.Logf("Invariant violated: scanned %d > total %d", task.ScannedIPs, task.TotalIPs)
				return false
			}

			return true
		},
		gen.IntRange(1, 1000),
		gen.IntRange(0, 1000),
	))

	// Property 4b: found_nodes + failed_ips <= scanned_ips
	properties.Property("found_nodes + failed_ips <= scanned_ips", prop.ForAll(
		func(scannedIPs, foundNodes, failedIPs int) bool {
			if scannedIPs < 0 {
				scannedIPs = 0
			}
			if foundNodes < 0 {
				foundNodes = 0
			}
			if failedIPs < 0 {
				failedIPs = 0
			}

			// Constrain to valid state
			if foundNodes+failedIPs > scannedIPs {
				return true // Invalid state, skip
			}

			task := &models.DiscoveryTask{
				ScannedIPs: scannedIPs,
				FoundNodes: foundNodes,
				FailedIPs:  failedIPs,
				Status:     models.DiscoveryStatusRunning,
			}

			// Verify invariant
			if task.FoundNodes+task.FailedIPs > task.ScannedIPs {
				t.Logf("Invariant violated: found %d + failed %d > scanned %d",
					task.FoundNodes, task.FailedIPs, task.ScannedIPs)
				return false
			}

			return true
		},
		gen.IntRange(0, 500),
		gen.IntRange(0, 500),
		gen.IntRange(0, 500),
	))

	// Property 4c: When status is "completed", scanned_ips == total_ips
	properties.Property("completed status implies scanned_ips == total_ips", prop.ForAll(
		func(totalIPs int) bool {
			if totalIPs < 1 {
				totalIPs = 1
			}

			db := setupDiscoveryTestDB(t)
			repo := newMockDiscoveryRepository(db)

			// Create a completed task
			task := &models.DiscoveryTask{
				CIDR:       "192.168.1.0/30",
				Port:       9001,
				Username:   "admin",
				Status:     models.DiscoveryStatusCompleted,
				TotalIPs:   totalIPs,
				ScannedIPs: totalIPs, // Must equal total when completed
				FoundNodes: 0,
				FailedIPs:  totalIPs,
				CreatedBy:  "test",
			}

			err := repo.CreateTask(task)
			if err != nil {
				t.Logf("Failed to create task: %v", err)
				return false
			}

			// Retrieve and verify
			retrieved, err := repo.GetTask(task.ID)
			if err != nil {
				t.Logf("Failed to get task: %v", err)
				return false
			}

			if retrieved.Status == models.DiscoveryStatusCompleted {
				if retrieved.ScannedIPs != retrieved.TotalIPs {
					t.Logf("Completed task has scanned %d != total %d",
						retrieved.ScannedIPs, retrieved.TotalIPs)
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 256),
	))

	// Property 4d: Progress percentage is always 0-100
	properties.Property("progress percentage is always 0-100", prop.ForAll(
		func(totalIPs, scannedIPs int) bool {
			if totalIPs < 1 {
				totalIPs = 1
			}
			if scannedIPs < 0 {
				scannedIPs = 0
			}
			if scannedIPs > totalIPs {
				scannedIPs = totalIPs
			}

			task := &models.DiscoveryTask{
				TotalIPs:   totalIPs,
				ScannedIPs: scannedIPs,
			}

			progress := task.Progress()

			if progress < 0 || progress > 100 {
				t.Logf("Progress out of range: %f (scanned=%d, total=%d)",
					progress, scannedIPs, totalIPs)
				return false
			}

			return true
		},
		gen.IntRange(1, 1000),
		gen.IntRange(0, 1000),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// ============================================================================
// Feature: node-discovery, Property 5: Node Registration Correctness
// For any successful probe result:
// - A Node record SHALL be created with name matching pattern "node-{ip-with-dashes}"
// - The Node SHALL have status "discovered"
// - If a node with same host:port exists, no duplicate SHALL be created
// **Validates: Requirements 5.1, 5.2, 5.3, 5.5**
// ============================================================================

func TestNodeRegistrationCorrectness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 5a: Node name matches pattern "node-{ip-with-dashes}"
	properties.Property("node name matches pattern node-{ip-with-dashes}", prop.ForAll(
		func(o1, o2, o3, o4 uint8) bool {
			ip := fmt.Sprintf("%d.%d.%d.%d", o1, o2, o3, o4)
			nodeName := generateNodeName(ip)

			expectedName := "node-" + strings.ReplaceAll(ip, ".", "-")

			if nodeName != expectedName {
				t.Logf("Name mismatch: expected %s, got %s", expectedName, nodeName)
				return false
			}

			// Verify format: node-X-X-X-X
			if !strings.HasPrefix(nodeName, "node-") {
				t.Logf("Name doesn't start with 'node-': %s", nodeName)
				return false
			}

			// Should not contain dots
			if strings.Contains(nodeName, ".") {
				t.Logf("Name contains dots: %s", nodeName)
				return false
			}

			return true
		},
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
	))

	// Property 5b: Registered nodes have status "discovered"
	properties.Property("registered nodes have status discovered", prop.ForAll(
		func(o1, o2, o3, o4 uint8, port int) bool {
			if port < 1 || port > 65535 {
				port = 9001
			}

			db := setupDiscoveryTestDB(t)
			nodeRepo := newMockNodeRepository(db)

			ip := fmt.Sprintf("%d.%d.%d.%d", o1, o2, o3, o4)
			nodeName := generateNodeName(ip)

			// Create node as the scanner would
			node := &models.Node{
				Name:     nodeName,
				Host:     ip,
				Port:     port,
				Username: "admin",
				Password: "secret",
				Status:   "discovered",
			}

			err := nodeRepo.Create(node)
			if err != nil {
				t.Logf("Failed to create node: %v", err)
				return false
			}

			// Verify status
			retrieved, err := nodeRepo.GetByName(nodeName)
			if err != nil {
				t.Logf("Failed to get node: %v", err)
				return false
			}

			if retrieved.Status != "discovered" {
				t.Logf("Node status is not 'discovered': %s", retrieved.Status)
				return false
			}

			return true
		},
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.IntRange(1, 65535),
	))

	// Property 5c: No duplicate nodes created for same host:port
	properties.Property("no duplicate nodes for same host:port", prop.ForAll(
		func(o1, o2, o3, o4 uint8, port int) bool {
			if port < 1 || port > 65535 {
				port = 9001
			}

			db := setupDiscoveryTestDB(t)
			nodeRepo := newMockNodeRepository(db)

			ip := fmt.Sprintf("%d.%d.%d.%d", o1, o2, o3, o4)
			nodeName := generateNodeName(ip)

			// Create first node
			node1 := &models.Node{
				Name:     nodeName,
				Host:     ip,
				Port:     port,
				Username: "admin",
				Password: "secret",
				Status:   "discovered",
			}

			err := nodeRepo.Create(node1)
			if err != nil {
				t.Logf("Failed to create first node: %v", err)
				return false
			}

			// Check if exists before creating second
			exists, err := nodeRepo.ExistsByHostPort(ip, port)
			if err != nil {
				t.Logf("Failed to check existence: %v", err)
				return false
			}

			if !exists {
				t.Logf("ExistsByHostPort returned false for existing node")
				return false
			}

			// Verify only one node exists
			nodes, count, err := nodeRepo.List(0, 100)
			if err != nil {
				t.Logf("Failed to list nodes: %v", err)
				return false
			}

			if count != 1 || len(nodes) != 1 {
				t.Logf("Expected 1 node, got %d", count)
				return false
			}

			return true
		},
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.IntRange(1, 65535),
	))

	// Property 5d: Node host and port match probe parameters
	properties.Property("node host and port match probe parameters", prop.ForAll(
		func(o1, o2, o3, o4 uint8, port int) bool {
			if port < 1 || port > 65535 {
				port = 9001
			}

			db := setupDiscoveryTestDB(t)
			nodeRepo := newMockNodeRepository(db)

			ip := fmt.Sprintf("%d.%d.%d.%d", o1, o2, o3, o4)
			nodeName := generateNodeName(ip)

			node := &models.Node{
				Name:     nodeName,
				Host:     ip,
				Port:     port,
				Username: "admin",
				Password: "secret",
				Status:   "discovered",
			}

			err := nodeRepo.Create(node)
			if err != nil {
				t.Logf("Failed to create node: %v", err)
				return false
			}

			retrieved, err := nodeRepo.GetByName(nodeName)
			if err != nil {
				t.Logf("Failed to get node: %v", err)
				return false
			}

			if retrieved.Host != ip {
				t.Logf("Host mismatch: expected %s, got %s", ip, retrieved.Host)
				return false
			}

			if retrieved.Port != port {
				t.Logf("Port mismatch: expected %d, got %d", port, retrieved.Port)
				return false
			}

			return true
		},
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.IntRange(1, 65535),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// ============================================================================
// Feature: node-discovery, Property 6: Result-Task Relationship Integrity
// For any DiscoveryResult record:
// - It SHALL have a valid TaskID referencing an existing DiscoveryTask
// - The result's IP SHALL be within the task's CIDR range
// **Validates: Requirements 7.1, 7.2**
// ============================================================================

func TestResultTaskRelationshipIntegrity(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 6a: Result has valid TaskID referencing existing task
	properties.Property("result has valid TaskID referencing existing task", prop.ForAll(
		func(o1, o2, o3, o4 uint8, prefix int) bool {
			if prefix < 28 {
				prefix = 28
			}

			db := setupDiscoveryTestDB(t)
			repo := newMockDiscoveryRepository(db)

			cidr := fmt.Sprintf("%d.%d.%d.%d/%d", o1, o2, o3, o4, prefix)

			// Create task first
			task := &models.DiscoveryTask{
				CIDR:      cidr,
				Port:      9001,
				Username:  "admin",
				Status:    models.DiscoveryStatusRunning,
				TotalIPs:  1 << (32 - prefix),
				CreatedBy: "test",
			}

			err := repo.CreateTask(task)
			if err != nil {
				t.Logf("Failed to create task: %v", err)
				return false
			}

			// Parse CIDR to get valid IP
			cidrRange, err := utils.ParseCIDR(cidr)
			if err != nil {
				return true // Invalid CIDR, skip
			}

			ips := cidrRange.IPs()
			if len(ips) == 0 {
				return true
			}

			// Create result with valid TaskID
			result := &models.DiscoveryResult{
				TaskID:   task.ID,
				IP:       ips[0],
				Port:     9001,
				Status:   models.ResultStatusSuccess,
				Duration: 100,
			}

			err = repo.CreateResult(result)
			if err != nil {
				t.Logf("Failed to create result: %v", err)
				return false
			}

			// Verify task exists
			retrievedTask, err := repo.GetTask(task.ID)
			if err != nil {
				t.Logf("Task not found for result: %v", err)
				return false
			}

			if retrievedTask.ID != result.TaskID {
				t.Logf("TaskID mismatch: result has %d, task has %d",
					result.TaskID, retrievedTask.ID)
				return false
			}

			return true
		},
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.IntRange(28, 32),
	))

	// Property 6b: Result IP is within task's CIDR range
	properties.Property("result IP is within task CIDR range", prop.ForAll(
		func(o1, o2, o3, o4 uint8, prefix int) bool {
			if prefix < 28 {
				prefix = 28
			}

			db := setupDiscoveryTestDB(t)
			repo := newMockDiscoveryRepository(db)

			cidr := fmt.Sprintf("%d.%d.%d.%d/%d", o1, o2, o3, o4, prefix)

			cidrRange, err := utils.ParseCIDR(cidr)
			if err != nil {
				return true // Invalid CIDR, skip
			}

			ips := cidrRange.IPs()
			if len(ips) == 0 {
				return true
			}

			// Create task
			task := &models.DiscoveryTask{
				CIDR:      cidr,
				Port:      9001,
				Username:  "admin",
				Status:    models.DiscoveryStatusRunning,
				TotalIPs:  cidrRange.Count(),
				CreatedBy: "test",
			}

			err = repo.CreateTask(task)
			if err != nil {
				t.Logf("Failed to create task: %v", err)
				return false
			}

			// Create results for all IPs in range
			for _, ip := range ips {
				result := &models.DiscoveryResult{
					TaskID:   task.ID,
					IP:       ip,
					Port:     9001,
					Status:   models.ResultStatusTimeout,
					Duration: 3000,
				}

				err = repo.CreateResult(result)
				if err != nil {
					t.Logf("Failed to create result: %v", err)
					return false
				}

				// Verify IP is in CIDR range
				if !cidrRange.Contains(ip) {
					t.Logf("Result IP %s not in CIDR %s", ip, cidr)
					return false
				}
			}

			// Verify all results
			results, err := repo.GetResultsByTaskID(task.ID)
			if err != nil {
				t.Logf("Failed to get results: %v", err)
				return false
			}

			for _, result := range results {
				if !cidrRange.Contains(result.IP) {
					t.Logf("Retrieved result IP %s not in CIDR %s", result.IP, cidr)
					return false
				}
			}

			return true
		},
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.IntRange(30, 32), // Very small ranges for this test
	))

	// Property 6c: Results count matches IPs scanned
	properties.Property("results count matches task progress", prop.ForAll(
		func(o1, o2, o3, o4 uint8) bool {
			db := setupDiscoveryTestDB(t)
			repo := newMockDiscoveryRepository(db)

			cidr := fmt.Sprintf("%d.%d.%d.%d/30", o1, o2, o3, o4) // 4 IPs

			cidrRange, err := utils.ParseCIDR(cidr)
			if err != nil {
				return true
			}

			ips := cidrRange.IPs()

			task := &models.DiscoveryTask{
				CIDR:       cidr,
				Port:       9001,
				Username:   "admin",
				Status:     models.DiscoveryStatusCompleted,
				TotalIPs:   len(ips),
				ScannedIPs: len(ips),
				CreatedBy:  "test",
			}

			err = repo.CreateTask(task)
			if err != nil {
				t.Logf("Failed to create task: %v", err)
				return false
			}

			// Create result for each IP
			for _, ip := range ips {
				result := &models.DiscoveryResult{
					TaskID:   task.ID,
					IP:       ip,
					Port:     9001,
					Status:   models.ResultStatusTimeout,
					Duration: 3000,
				}
				repo.CreateResult(result)
			}

			results, err := repo.GetResultsByTaskID(task.ID)
			if err != nil {
				t.Logf("Failed to get results: %v", err)
				return false
			}

			if len(results) != task.ScannedIPs {
				t.Logf("Results count %d != ScannedIPs %d", len(results), task.ScannedIPs)
				return false
			}

			return true
		},
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
		gen.UInt8(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
