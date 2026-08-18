//go:build ignore
package main

import (
	"fmt"
	"superview/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"log"
	"os"
	"path/filepath"
)

func main() {
	// 直接连接到数据库
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	// 连接SQLite数据库
	dbPath := filepath.Join(cwd, "data", "superview.db")
	fmt.Printf("Connecting to database: %s\n", dbPath)
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// 检查表是否存在
	hasTable := db.Migrator().HasTable(&models.User{})
	fmt.Printf("Users table exists: %v\n", hasTable)

	// 显示表结构
	var tableInfo string
	db.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name='users'").Scan(&tableInfo)
	fmt.Printf("Table structure:\n%s\n", tableInfo)

	// 查询用户数据
	var users []models.User
	if err := db.Find(&users).Error; err != nil {
		log.Fatalf("Failed to query users: %v", err)
	}

	if len(users) == 0 {
		fmt.Println("No users found in database")
		return
	}

	for _, user := range users {
		fmt.Printf("ID: %s\n", user.ID)
		fmt.Printf("Username: %s\n", user.Username)
		fmt.Printf("Password Hash: %s\n", user.Password)
		fmt.Printf("Is Admin: %t\n", user.IsAdmin)
		fmt.Println("-------------------")
	}
}
