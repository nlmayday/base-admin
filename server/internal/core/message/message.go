// Package message defines how business modules send personal messages to
// users: they appear with the notices behind the header bell and in
// 我的消息. The notice module implements Sender; a business module takes a
// Sender in its Deps (wired in internal/app/modules.go).
package message

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
)

// Limits of one message.
const (
	MaxTitle    = 128
	MaxContent  = 2000
	MaxLink     = 512
	MaxCategory = 64
)

// Message is one personal message to one user.
type Message struct {
	// UserID is the recipient. Messages to users that do not exist or are
	// deleted are skipped.
	UserID int64
	// Category names the source, "<module>.<kind>" such as "crm.reminder".
	Category string
	Title    string
	// Content is plain text; optional.
	Content string
	// Link is a frontend route opened when the message is clicked, such as
	// "/crm/customer?id=42"; optional. It must start with a single "/".
	Link string
}

// Sender sends personal messages.
type Sender interface {
	// Send stores msgs. With tx (non-nil) the messages are written in the
	// caller's transaction and appear only if it commits; otherwise they
	// are written at once. An invalid message fails the whole call.
	Send(ctx context.Context, tx *gorm.DB, msgs ...Message) error
}

var categoryPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*(\.[a-z0-9_-]+)+$`)

// Validate reports the first problem of m, or nil.
func (m Message) Validate() error {
	switch {
	case m.UserID <= 0:
		return fmt.Errorf("message: invalid user id %d", m.UserID)
	case len(m.Category) > MaxCategory || !categoryPattern.MatchString(m.Category):
		return fmt.Errorf("message: category %q must look like module.kind", m.Category)
	case strings.TrimSpace(m.Title) == "" || utf8.RuneCountInString(m.Title) > MaxTitle:
		return fmt.Errorf("message: title must be 1-%d characters", MaxTitle)
	case utf8.RuneCountInString(m.Content) > MaxContent:
		return fmt.Errorf("message: content longer than %d characters", MaxContent)
	case m.Link != "" && !ValidLink(m.Link):
		return fmt.Errorf("message: link %q must be a frontend route starting with /", m.Link)
	}
	return nil
}

// ValidLink reports whether link is a site-relative frontend route: it
// starts with one "/", so it can never lead to another site.
func ValidLink(link string) bool {
	return len(link) <= MaxLink && strings.HasPrefix(link, "/") && !strings.HasPrefix(link, "//") &&
		!strings.ContainsAny(link, "\\\r\n\t ")
}
