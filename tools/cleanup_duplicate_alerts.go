//go:build ignore
package main

import (
	"fmt"
	"log"
	"time"

	"superview/internal/database"
	"superview/internal/models"
	"gorm.io/gorm"
)

func main() {
	// 初始化数据库连接
	if err := database.InitDB(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	
	db := database.DB
	if db == nil {
		log.Fatalf("Database connection is nil")
	}

	// 清理重复的节点离线告警
	if err := cleanupDuplicateNodeAlerts(db); err != nil {
		log.Fatalf("Failed to cleanup duplicate node alerts: %v", err)
	}

	// 清理重复的进程停止告警
	if err := cleanupDuplicateProcessAlerts(db); err != nil {
		log.Fatalf("Failed to cleanup duplicate process alerts: %v", err)
	}

	fmt.Println("Duplicate alerts cleanup completed successfully!")
}

// cleanupDuplicateNodeAlerts 清理重复的节点离线告警
func cleanupDuplicateNodeAlerts(db *gorm.DB) error {
	fmt.Println("Cleaning up duplicate node offline alerts...")

	// 查找所有节点的重复告警
	var nodes []string
	if err := db.Model(&models.Alert{}).
		Where("rule_id = ? AND status IN (?, ?)", 1, models.AlertStatusActive, models.AlertStatusAcknowledged).
		Distinct("node_name").
		Pluck("node_name", &nodes).Error; err != nil {
		return err
	}

	for _, nodeName := range nodes {
		if err := mergeDuplicateNodeAlerts(db, nodeName); err != nil {
			return fmt.Errorf("failed to merge alerts for node %s: %v", nodeName, err)
		}
	}

	return nil
}

// cleanupDuplicateProcessAlerts 清理重复的进程停止告警
func cleanupDuplicateProcessAlerts(db *gorm.DB) error {
	fmt.Println("Cleaning up duplicate process stopped alerts...")

	// 查找所有进程的重复告警
	type ProcessKey struct {
		NodeName    string
		ProcessName string
	}

	var processes []ProcessKey
	if err := db.Model(&models.Alert{}).
		Where("rule_id = ? AND status IN (?, ?) AND process_name IS NOT NULL", 
			2, models.AlertStatusActive, models.AlertStatusAcknowledged).
		Select("DISTINCT node_name, process_name").
		Scan(&processes).Error; err != nil {
		return err
	}

	for _, proc := range processes {
		if err := mergeDuplicateProcessAlerts(db, proc.NodeName, proc.ProcessName); err != nil {
			return fmt.Errorf("failed to merge alerts for process %s on node %s: %v", 
				proc.ProcessName, proc.NodeName, err)
		}
	}

	return nil
}

// mergeDuplicateNodeAlerts 合并同一节点的重复告警
func mergeDuplicateNodeAlerts(db *gorm.DB, nodeName string) error {
	var alerts []models.Alert
	if err := db.Where("rule_id = ? AND node_name = ? AND status IN (?, ?)",
		1, nodeName, models.AlertStatusActive, models.AlertStatusAcknowledged).
		Order("created_at ASC").
		Find(&alerts).Error; err != nil {
		return err
	}

	if len(alerts) <= 1 {
		return nil // 没有重复告警
	}

	fmt.Printf("Found %d duplicate alerts for node '%s', merging...\n", len(alerts), nodeName)

	// 保留最早的告警，删除其他重复告警
	keepAlert := alerts[0]
	duplicateIDs := make([]uint, 0, len(alerts)-1)
	
	// 更新保留的告警为最新状态
	latestStartTime := keepAlert.StartTime
	for _, alert := range alerts[1:] {
		duplicateIDs = append(duplicateIDs, alert.ID)
		if alert.StartTime.After(latestStartTime) {
			latestStartTime = alert.StartTime
		}
	}

	// 更新保留的告警
	if err := db.Model(&keepAlert).Updates(map[string]interface{}{
		"start_time": latestStartTime,
		"updated_at": time.Now(),
	}).Error; err != nil {
		return err
	}

	// 删除重复的告警
	if err := db.Delete(&models.Alert{}, duplicateIDs).Error; err != nil {
		return err
	}

	fmt.Printf("Merged %d duplicate alerts for node '%s' into alert ID %d\n", 
		len(duplicateIDs), nodeName, keepAlert.ID)

	return nil
}

// mergeDuplicateProcessAlerts 合并同一进程的重复告警
func mergeDuplicateProcessAlerts(db *gorm.DB, nodeName, processName string) error {
	var alerts []models.Alert
	if err := db.Where("rule_id = ? AND node_name = ? AND process_name = ? AND status IN (?, ?)",
		2, nodeName, processName, models.AlertStatusActive, models.AlertStatusAcknowledged).
		Order("created_at ASC").
		Find(&alerts).Error; err != nil {
		return err
	}

	if len(alerts) <= 1 {
		return nil // 没有重复告警
	}

	fmt.Printf("Found %d duplicate alerts for process '%s' on node '%s', merging...\n", 
		len(alerts), processName, nodeName)

	// 保留最早的告警，删除其他重复告警
	keepAlert := alerts[0]
	duplicateIDs := make([]uint, 0, len(alerts)-1)
	
	// 更新保留的告警为最新状态
	latestStartTime := keepAlert.StartTime
	for _, alert := range alerts[1:] {
		duplicateIDs = append(duplicateIDs, alert.ID)
		if alert.StartTime.After(latestStartTime) {
			latestStartTime = alert.StartTime
		}
	}

	// 更新保留的告警
	if err := db.Model(&keepAlert).Updates(map[string]interface{}{
		"start_time": latestStartTime,
		"updated_at": time.Now(),
	}).Error; err != nil {
		return err
	}

	// 删除重复的告警
	if err := db.Delete(&models.Alert{}, duplicateIDs).Error; err != nil {
		return err
	}

	fmt.Printf("Merged %d duplicate alerts for process '%s' on node '%s' into alert ID %d\n", 
		len(duplicateIDs), processName, nodeName, keepAlert.ID)

	return nil
}