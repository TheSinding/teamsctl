package teamsctl

import (
	"fmt"
	"sync"
	"time"

	teamsapi "github.com/fossteams/teams-api"
	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/fossteams/teams-api/pkg/models"
)

type Service struct {
	client        *teamsapi.TeamsClient
	cacheMu       sync.Mutex
	cacheUntil    time.Time
	response      *csa.ConversationResponse
	me            *models.User
	cacheEnriched bool
}

func NewService() (*Service, error) {
	client, err := teamsapi.New()
	if err != nil {
		return nil, fmt.Errorf("initialize Teams client: %w", err)
	}
	return &Service{client: client}, nil
}
