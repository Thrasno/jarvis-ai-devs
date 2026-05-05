package sddruntime

import "strings"

type MemoryTopicClass string

const (
	MemoryTopicSDDArtifact MemoryTopicClass = "sdd_artifact"
	MemoryTopicGeneral     MemoryTopicClass = "general"
)

func ClassifyMemoryTopic(topic string) MemoryTopicClass {
	if IsSDDArtifactTopic(topic) {
		return MemoryTopicSDDArtifact
	}
	return MemoryTopicGeneral
}

func IsSDDArtifactTopic(topic string) bool {
	parts := strings.Split(topic, "/")
	if len(parts) != 3 {
		return false
	}
	if parts[0] != "sdd" {
		return false
	}
	return parts[1] != "" && parts[2] != ""
}
