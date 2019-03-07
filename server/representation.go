package server

//go:generate msgp

// Representation ...
type Representation struct {
	Address string `msg:"url" json:"address"`
	Port    string `msg:"port" json:"port"`
	Region  string `msg:"region" json:"region"`
}
