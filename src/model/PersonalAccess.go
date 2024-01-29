package model

type PersonalAccessView struct {
	Id      int64  `json:"id"`
	Uuid    string `json:"uuid"`
	Name    string `json:"name"`
	Key     string `json:"key"`
	Passkey string `json:"passkey"`
	Salt    string `json:"salt"`
	Status  string `json:"status"`
}
