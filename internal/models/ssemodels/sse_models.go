package ssemodels

import (
	"time"
)

type SseType string

const (
	SseType_Unknown              SseType = "unknown"
	SseType_Time                 SseType = "time"
	SseType_Cluster_Created      SseType = "cluster.created"
	SseType_ClusterOrder_Updated SseType = "clusterOrder.updated"
)

// Deprecated: Use SseMessage instead, this is not a valid format
type Time struct {
	Event       SseType   `json:"event"`
	CurrentTime time.Time `json:"currentTime"`
}

type SseMessage struct {
	Id    string      `json:"id omitempty"`
	Event SseType     `json:"event"`
	Data  interface{} `json:"data"`
}
