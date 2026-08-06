package teamsctl

import (
	"fmt"
	"sync"
	"time"

	"github.com/TheSinding/teamsctl/pkg/teamsauth"
	teamsapi "github.com/TheSinding/teamsctl/third_party/teams-api"
	"github.com/TheSinding/teamsctl/third_party/teams-api/pkg/csa"
	"github.com/TheSinding/teamsctl/third_party/teams-api/pkg/models"
)

var newTeamsAPIClient = teamsapi.NewWithTokens

type Service struct {
	client        *teamsapi.TeamsClient
	cacheMu       sync.Mutex
	cacheUntil    time.Time
	response      *csa.ConversationResponse
	me            *models.User
	cacheEnriched bool
}

func NewService() (*Service, error) {
	tokens, err := teamsauth.LoadClientTokens()
	if err != nil {
		return nil, err
	}
	client, err := newTeamsAPIClient(tokens.Skype, tokens.ChatSvcAgg)
	if err != nil {
		return nil, fmt.Errorf("initialize Teams client: %w", err)
	}
	return &Service{client: client}, nil
}

// currentUser returns the signed-in account, fetching it on first use and
// caching it alongside the conversation state. It returns nil when the
// identity cannot be fetched, so callers may treat it as best-effort.
func (s *Service) currentUser() *models.User {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.me == nil {
		me, err := s.client.GetMe()
		if err != nil {
			return nil
		}
		s.me = me
	}
	return s.me
}
