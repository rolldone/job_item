package model

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type NestedJob struct {
	NestedJobId string `json:"nested_job_id"`
	Name        string `json:"name"`
	Event       string `json:"event"`
	Limit       int    `json:"limit"`
	Timeout     int    `json:"timeout"`
}

// Custom unmarshal to support string or int for Limit/Timeout
func (n *NestedJob) UnmarshalJSON(data []byte) error {
	var aux struct {
		NestedJobId string      `json:"nested_job_id"`
		Name        string      `json:"name"`
		Event       string      `json:"event"`
		Limit       interface{} `json:"limit"`
		Timeout     interface{} `json:"timeout"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	n.Name = aux.Name
	n.Event = aux.Event
	n.NestedJobId = aux.NestedJobId
	// Handle Limit
	switch v := aux.Limit.(type) {
	case float64:
		n.Limit = int(v)
	case string:
		i, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid limit: %v", v)
		}
		n.Limit = i
	default:
		n.Limit = 0
	}
	// Handle Timeout
	switch v := aux.Timeout.(type) {
	case float64:
		n.Timeout = int(v)
	case string:
		i, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid timeout: %v", v)
		}
		n.Timeout = i
	default:
		n.Timeout = 0
	}
	return nil
}
