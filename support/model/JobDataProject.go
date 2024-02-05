package model

import (
	"time"
)

type JobDataProject struct {
	Job_data_uuid     string    `gorm:"column:job_data_uuid;type:uuid" json:"job_data_uuid"`
	Project_data_uuid string    `gorm:"column:project_data_uuid;type:uuid" json:"project_data_uuid"`
	Created_at        time.Time `gorm:"column:created_at;type:timestamp;default:CURRENT_TIMESTAMP" json:"created_at"`
	Updated_at        time.Time `gorm:"column:updated_at;type:timestamp;autoUpdateTime:true" json:"updated_at"`
}

// // Implement the Valuer interface
// func (d JobDataProject) Value() (driver.Value, error) {
// 	return json.Marshal(d)
// }

// // Implement the Scanner interface
// func (d *JobDataProject) Scan(value interface{}) error {
// 	bytes, ok := value.([]byte)
// 	if !ok {
// 		return errors.New("invalid data type for Data")
// 	}
// 	return json.Unmarshal(bytes, d)
// }

// Set the table name for the User model
func (c *JobDataProject) TableName() string {
	return "job_data_project" // Replace with your existing table name
}
