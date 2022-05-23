package main

type Status struct {
	Status string `json:"status"`
}

type componentStatus map[string][]Status

type Component struct {
	Status Status `json:"component"`
}
