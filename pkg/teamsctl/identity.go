package teamsctl

import "fmt"

// Identity describes the signed-in Teams account, for callers that need to
// tell the current user's own messages apart from a conversation partner's
// (e.g. when polling for a reply after Send).
type Identity struct {
	DisplayName       string
	Email             string
	UserPrincipalName string
	ObjectID          string
	Mri               string
}

// Me returns the signed-in account's identity, fetching and caching
// conversation state if it has not been loaded yet.
func (s *Service) Me() (Identity, error) {
	if _, err := s.conversationRecords(false); err != nil {
		return Identity{}, fmt.Errorf("get current user: %w", err)
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.me == nil {
		return Identity{}, fmt.Errorf("current user unavailable")
	}
	return Identity{
		DisplayName:       s.me.DisplayName,
		Email:             s.me.Email,
		UserPrincipalName: s.me.UserPrincipalName,
		ObjectID:          s.me.ObjectId,
		Mri:               s.me.Mri,
	}, nil
}
