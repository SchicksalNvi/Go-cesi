package services

import "gorm.io/gorm"

func createAndPreserveBool(db *gorm.DB, model interface{}, column string, value bool) error {
	if err := db.Create(model).Error; err != nil {
		return err
	}
	if value {
		return nil
	}
	return db.Model(model).UpdateColumn(column, false).Error
}
