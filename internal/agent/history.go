package agent

import "github.com/cloudwego/eino/schema"

type ConversationHistory struct {
	log []*schema.Message
}

func (h *ConversationHistory) append(msg *schema.Message) {
	h.log = append(h.log, msg)
}

func (h *ConversationHistory) all() []*schema.Message {
	return h.log
}
