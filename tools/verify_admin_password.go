//go:build ignore
package main

import (
	"fmt"
	"superview/internal/models"
	"golang.org/x/crypto/bcrypt"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"log"
	"os"
	"path/filepath"
)

func main() {
	// 获取环境变量中的密码
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		log.Fatal("ADMIN_PASSWORD environment variable not set")
	}
	fmt.Printf("Environment ADMIN_PASSWORD: %s\n", adminPassword)

	// 连接到数据库
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	dbPath := filepath.Join(cwd, "data", "superview.db")
	fmt.Printf("Connecting to database: %s\n", dbPath)
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// 查询admin用户
	var adminUser models.User
	if err := db.Where("username = ? AND is_admin = ?", "admin", true).First(&adminUser).Error; err != nil {
		log.Fatalf("Failed to find admin user: %v", err)
	}

	fmt.Printf("Found admin user: %s\n", adminUser.Username)
	fmt.Printf("Current password hash: %s\n", adminUser.Password)

	// 验证密码
	if adminUser.VerifyPassword(adminPassword) {
		fmt.Println("✅ Password verification SUCCESSFUL - Environment password matches database hash")
	} else {
		fmt.Println("❌ Password verification FAILED - Environment password does NOT match database hash")
		
		// 生成新的密码哈希用于比较
		newHash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("Failed to generate new hash: %v", err)
		}
		fmt.Printf("Expected hash for '%s': %s\n", adminPassword, string(newHash))
	}
}