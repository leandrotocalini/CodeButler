package access

import (
	"github.com/leandrotocalini/CodeButler/internal/config"
	"github.com/leandrotocalini/CodeButler/internal/whatsapp"
)

func IsAllowed(msg whatsapp.Message, cfg *config.Config) bool {
	if cfg.WhatsApp.GroupJID == "" {
		return false
	}

	return msg.Chat == cfg.WhatsApp.GroupJID
}
