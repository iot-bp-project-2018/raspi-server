package main

import (
	"encoding/json"
)

// SensorPayload contains all the fields sent by one sensor packet
type SensorPayload struct {
	SensorID byte    `json:"sensor_id"`
	Value    float32 `json:"value"`
	Type     string  `json:"type"`
	Unit     string  `json:"unit"`
}

// SensorPayloadFromJSONBuffer decodes a json byte array into SensorPayload
func SensorPayloadFromJSONBuffer(buffer []byte) SensorPayload {
	p := SensorPayload{}
	json.Unmarshal(buffer, &p)
	return p
}

// DataQueryRequest requests data from the database
type DataQueryRequest struct {
	DeviceID          string `json:"deviceId"`
	SensorID          int    `json:"sensorId"`
	BeginUnix         int    `json:"beginUnix"`
	EndUnix           int    `json:"endUnix"`
	ResolutionSeconds int    `json:"resolutionSeconds"`
}

// RelativeDataQueryRequest requests data from the database
type RelativeDataQueryRequest struct {
	DeviceID             string `json:"deviceId"`
	SensorID             int    `json:"sensorId"`
	BeginRelativeSeconds int    `json:"beginRelativeSeconds"`
	EndRelativeSeconds   int    `json:"endRelativeSeconds"`
	ResolutionSeconds    int    `json:"resolutionSeconds"`
}
