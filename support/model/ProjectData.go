package model

import (
	"time"
)

type ProjectData struct {
	Uuid       string                 `json:"uuid"`
	Name       string                 `json:"name"`
	Passkey    string                 `json:"passkey,omitempty"`
	Salt       string                 `json:"salt,omitempty"`
	Status     string                 `json:"status"`
	Connection string                 `json:"connection"`
	Data       map[string]interface{} `json:"data"`
	Created_at time.Time              `json:"created_at"`
	Updated_at time.Time              `json:"updated_at"`
	Deleted_at *time.Time             `json:"deleted_at,omitempty"`
}

type ProjectDataView struct {
	Uuid       string     `json:"uuid"`
	Key        *string    `json:"key,omitempty"`
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	Connection string     `json:"connection"`
	Created_at time.Time  `json:"created_at"`
	Updated_at time.Time  `json:"updated_at"`
	Deleted_at *time.Time `json:"deleted_at,omitempty"`

	// Relation

	// This is many2many. Here the explanation:
	// Target : project_data.
	// Binding : job_data_project.
	// Foreign key : uuid => "This is job_data.uuid".
	// References key : uuid => "This is project_data.uuid".
	// Join Foreign Key : job_data_uuid => "This is job_data_project.project_data_uuid" from relation "project_data" side
	// Join References Key : project_data_uuid => "This is job_data_project.job_data_uuid" from relation "job_data" side
	Job_datas []JobData `json:"job_datas,omitempty"`
}
