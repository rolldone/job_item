package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

type JobData_Data struct {
	Limit_process int    `json:"limit_process"`
	Timeout       int    `json:"timeout"`
	Body          string `json:"body,omitempty"`
}

// Implement the Valuer interface
func (d JobData_Data) Value() (driver.Value, error) {
	return json.Marshal(d)
}

// Implement the Scanner interface
func (d *JobData_Data) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("invalid data type for Data")
	}
	return json.Unmarshal(bytes, d)
}

type JobData struct {
	Id          int64                   `json:"id"`
	Uuid        string                  `json:"uuid"`
	Event       string                  `json:"event"`
	Name        string                  `json:"name"`
	Data        *JobData_Data           `json:"data,omitempty"`
	Data_store  *map[string]interface{} `json:"data_store"`
	Created_at  time.Time               `json:"created_at"`
	Updated_at  time.Time               `json:"updated_at"`
	Deleted_at  *time.Time              `json:"deleted_at,omitempty"`
	Nested_jobs *[]NestedJob            `json:"nested_jobs,omitempty"`

	// This is simple relation has many.
	// Project_datas []JobDataProject `gorm:"foreignKey:job_data_uuid;References:uuid" json:"project_datas,omitempty"`

	// This is many2many. Here the explanation:
	// Target : project_data.
	// Binding : job_data_project.
	// Foreign key : uuid => "This is project_data.uuid".
	// References key : uuid => "This is job_data.uuid".
	// Join Foreign Key : job_data_uuid => "This is job_data_project.job_data_uuid" from relation "job_data" side
	// Join References Key : project_data_uuid => "This is job_data_project.project_data_uuid" from relation "project_data" side
	Project_datas []*ProjectData `json:"project_datas,omitempty"`
	// For join where
	Job_data_projects []JobDataProject `json:"job_data_projects,omitempty"`
}
