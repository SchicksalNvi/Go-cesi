//go:build ignore
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"superview/internal/models"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"log"
)

func main() {
	// L-10: 不再默认使用弱口令 "123456"。密码必须通过 -password 显式提供,
	// 否则工具拒绝执行,防止部署时误用默认凭据。
	password := flag.String("password", "", "新管理员密码(必填,不允许使用弱口令)")
	printHash := flag.Bool("print-hash", false, "将密码哈希打印到输出(默认关闭,避免泄露)")
	flag.Parse()

	if *password == "" {
		log.Fatalf("必须通过 -password 显式提供新密码(为安全起见不再使用默认口令)")
	}
	if len(*password) < 8 {
		log.Fatalf("新密码长度不足 8 位,请使用更强密码")
	}
	newPassword := *password

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

	// 生成新的密码哈希
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	// 更新密码 - 只更新密码字段
	if err := db.Model(&adminUser).Where("id = ?", adminUser.ID).Update("password", string(hashedPassword)).Error; err != nil {
		log.Fatalf("Failed to update password: %v", err)
	}

	// 重新加载用户以获取更新后的数据
	if err := db.Where("username = ? AND is_admin = ?", "admin", true).First(&adminUser).Error; err != nil {
		log.Fatalf("Failed to reload admin user: %v", err)
	}

	if *printHash {
		fmt.Printf("New password hash: %s\n", adminUser.Password)
	}
	fmt.Println("✅ Password reset successfully!")

	// 验证新密码
	if adminUser.VerifyPassword(newPassword) {
		fmt.Println("✅ Password verification successful!")
	} else {
		fmt.Println("❌ Password verification failed!")
	}
}
