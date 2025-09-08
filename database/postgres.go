package database

import (
	dto "github.com/rizqitaufiqf/go-scheduler/dto"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// InitDB initializes the database connection and runs migrations.
func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Auto-migrate the schema
	err = db.AutoMigrate(&dto.Product{}, &dto.ScheduledTask{})
	if err != nil {
		return nil, err
	}

	return db, nil
}
