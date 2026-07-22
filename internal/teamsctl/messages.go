package teamsctl

import (
	"fmt"
	"sort"
	"time"

	"github.com/fossteams/teams-api/pkg/csa"
)

func (s *Service) Messages(ids []string, displayName string, limit int) ([]Message, error) {
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return nil, fmt.Errorf("no conversation id available")
	}
	var messages []csa.ChatMessage
	var lastErr error
	for _, id := range ids {
		messages, lastErr = s.client.GetMessages(&csa.Channel{Id: id, DisplayName: displayName})
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("get messages: %w", lastErr)
	}
	sort.Sort(csa.SortMessageByTime(messages))
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	records := make([]Message, 0, len(messages))
	for _, message := range messages {
		records = append(records, Message{
			ID:             message.Id,
			ConversationID: message.ConversationId,
			Author:         message.ImDisplayName,
			Content:        message.Content,
			ContentType:    message.ContentType,
			MessageType:    message.MessageType,
			CreatedAt:      time.Time(message.OriginalArrivalTime),
			Mentions:       message.Properties.Mentions,
		})
	}
	return records, nil
}
