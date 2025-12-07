package realtimeapi

import (
	"errors"
	"sync"

	"github.com/google/uuid"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/events"
)

// Conversation manages the conversation state for a session.
type Conversation struct {
	ID    string
	Items []events.ConversationItem

	mu sync.RWMutex
}

// NewConversation creates a new Conversation.
func NewConversation() *Conversation {
	return &Conversation{
		ID:    "conv_" + uuid.New().String()[:8],
		Items: make([]events.ConversationItem, 0),
	}
}

// AddItem adds a new item to the conversation.
func (c *Conversation) AddItem(item events.ConversationItem) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Items = append(c.Items, item)
}

// GetItem returns an item by ID.
func (c *Conversation) GetItem(itemID string) (*events.ConversationItem, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := range c.Items {
		if c.Items[i].ID == itemID {
			return &c.Items[i], nil
		}
	}
	return nil, errors.New("item not found")
}

// GetLastItemID returns the ID of the last item, or empty string if none.
func (c *Conversation) GetLastItemID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.Items) == 0 {
		return ""
	}
	return c.Items[len(c.Items)-1].ID
}

// GetItems returns all items in the conversation.
func (c *Conversation) GetItems() []events.ConversationItem {
	c.mu.RLock()
	defer c.mu.RUnlock()

	items := make([]events.ConversationItem, len(c.Items))
	copy(items, c.Items)
	return items
}

// DeleteItem removes an item from the conversation.
func (c *Conversation) DeleteItem(itemID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Items {
		if c.Items[i].ID == itemID {
			c.Items = append(c.Items[:i], c.Items[i+1:]...)
			return nil
		}
	}
	return errors.New("item not found")
}

// TruncateItem truncates an item's audio content.
func (c *Conversation) TruncateItem(itemID string, contentIndex int, audioEndMs int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Items {
		if c.Items[i].ID == itemID {
			if contentIndex >= len(c.Items[i].Content) {
				return errors.New("content index out of range")
			}

			// For audio content, we would truncate the audio data here
			// This is a simplified implementation
			// In a real implementation, you would:
			// 1. Decode the base64 audio
			// 2. Calculate the byte offset for audioEndMs
			// 3. Truncate the audio data
			// 4. Re-encode to base64

			return nil
		}
	}
	return errors.New("item not found")
}

// UpdateItemStatus updates the status of an item.
func (c *Conversation) UpdateItemStatus(itemID string, status events.ItemStatus) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Items {
		if c.Items[i].ID == itemID {
			c.Items[i].Status = status
			return nil
		}
	}
	return errors.New("item not found")
}

// Clear removes all items from the conversation.
func (c *Conversation) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Items = make([]events.ConversationItem, 0)
}

// Count returns the number of items in the conversation.
func (c *Conversation) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.Items)
}

// GetItemsByRole returns all items with the specified role.
func (c *Conversation) GetItemsByRole(role events.Role) []events.ConversationItem {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var items []events.ConversationItem
	for _, item := range c.Items {
		if item.Role == role {
			items = append(items, item)
		}
	}
	return items
}

// InsertItemAfter inserts an item after the specified item ID.
func (c *Conversation) InsertItemAfter(afterItemID string, item events.ConversationItem) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if afterItemID == "" {
		// Insert at the beginning
		c.Items = append([]events.ConversationItem{item}, c.Items...)
		return nil
	}

	for i := range c.Items {
		if c.Items[i].ID == afterItemID {
			// Insert after this item
			c.Items = append(c.Items[:i+1], append([]events.ConversationItem{item}, c.Items[i+1:]...)...)
			return nil
		}
	}
	return errors.New("item not found")
}

// GetLastAssistantItem returns the last item from the assistant.
func (c *Conversation) GetLastAssistantItem() *events.ConversationItem {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := len(c.Items) - 1; i >= 0; i-- {
		if c.Items[i].Role == events.RoleAssistant {
			return &c.Items[i]
		}
	}
	return nil
}

// AppendContentToItem appends content to an existing item.
func (c *Conversation) AppendContentToItem(itemID string, content events.Content) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Items {
		if c.Items[i].ID == itemID {
			c.Items[i].Content = append(c.Items[i].Content, content)
			return nil
		}
	}
	return errors.New("item not found")
}
