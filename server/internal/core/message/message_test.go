package message

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	ok := Message{UserID: 1, Category: "crm.reminder", Title: "跟进提醒", Link: "/crm/customer?id=42"}
	assert.NoError(t, ok.Validate())

	cases := map[string]func(m *Message){
		"user":           func(m *Message) { m.UserID = 0 },
		"no category":    func(m *Message) { m.Category = "" },
		"no dot":         func(m *Message) { m.Category = "crm" },
		"upper category": func(m *Message) { m.Category = "CRM.x" },
		"blank title":    func(m *Message) { m.Title = "  " },
		"long title":     func(m *Message) { m.Title = strings.Repeat("长", MaxTitle+1) },
		"long content":   func(m *Message) { m.Content = strings.Repeat("x", MaxContent+1) },
		"external link":  func(m *Message) { m.Link = "https://evil.example" },
		"scheme-less":    func(m *Message) { m.Link = "//evil.example" },
		"backslash":      func(m *Message) { m.Link = "/\\evil.example" },
		"javascript":     func(m *Message) { m.Link = "javascript:alert(1)" },
	}
	for name, mutate := range cases {
		m := ok
		mutate(&m)
		assert.Error(t, m.Validate(), name)
	}
	m := ok
	m.Link, m.Content = "", ""
	assert.NoError(t, m.Validate(), "link and content are optional")
}
