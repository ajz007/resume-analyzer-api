package queue

import "encoding/json"

// Message is the payload sent to downstream queue consumers.
type Message struct {
	AnalysisID string `json:"analysisId"`
	RequestID  string `json:"requestId"`
	EnqueuedAt string `json:"enqueuedAt"`
	Version    int    `json:"version"`
}

// EncodeMessage returns the JSON representation of a message.
func EncodeMessage(msg Message) ([]byte, error) {
	return json.Marshal(msg)
}

// DecodeMessage parses a JSON payload into a Message.
func DecodeMessage(payload []byte) (Message, error) {
	var msg Message
	if err := json.Unmarshal(payload, &msg); err != nil {
		return Message{}, err
	}
	return msg, nil
}
