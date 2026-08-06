package teamsctl

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/fossteams/teams-api/pkg/models"
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
	return messageRecords(messages, s.currentUser()), nil
}

func messageRecords(messages []csa.ChatMessage, me *models.User) []Message {
	records := make([]Message, 0, len(messages))
	for _, message := range messages {
		author := message.ImDisplayName
		if strings.TrimSpace(author) == "" && isSelfSender(message.From, me) {
			author = me.DisplayName
		}
		records = append(records, Message{
			ID:             message.Id,
			ConversationID: message.ConversationId,
			Author:         author,
			SenderID:       message.From,
			Content:        message.Content,
			ContentType:    message.ContentType,
			MessageType:    message.MessageType,
			CreatedAt:      time.Time(message.OriginalArrivalTime),
			Mentions:       message.Properties.Mentions,
		})
	}
	return records
}

// isSelfSender reports whether from is the signed-in account's MRI. Teams
// leaves ImDisplayName empty on your own messages, so callers use this to
// backfill the author. The MRI is matched directly, or derived from ObjectId
// when the profile did not return one.
func isSelfSender(from string, me *models.User) bool {
	if me == nil {
		return false
	}
	from = strings.TrimSpace(from)
	if from == "" {
		return false
	}
	if me.Mri != "" && from == strings.TrimSpace(me.Mri) {
		return true
	}
	return me.ObjectId != "" && from == "8:orgid:"+strings.TrimSpace(me.ObjectId)
}
