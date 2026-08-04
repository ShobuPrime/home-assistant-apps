package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Broker is the Supervisor-provided MQTT broker connection info.
type Broker struct {
	Host     string
	Port     int
	Username string
	Password string
	SSL      bool
}

// Addr returns the host:port dial string.
func (b Broker) Addr() string { return fmt.Sprintf("%s:%d", b.Host, b.Port) }

// DiscoverBroker queries the Supervisor for a configured MQTT service.
// The second return is false (with a nil error) when no broker is
// available — e.g. the user has not added one, or under the CI mock
// Supervisor which does not serve /services/mqtt. Callers should then run
// without the native MQTT entity rather than failing.
func DiscoverBroker(ctx context.Context, token string) (Broker, bool, error) {
	if token == "" {
		return Broker{}, false, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://supervisor/services/mqtt", nil)
	if err != nil {
		return Broker{}, false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return Broker{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Broker{}, false, nil // no broker provisioned
	}
	var env struct {
		Result string `json:"result"`
		Data   struct {
			Host     string `json:"host"`
			Port     int    `json:"port"`
			Username string `json:"username"`
			Password string `json:"password"`
			SSL      bool   `json:"ssl"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return Broker{}, false, err
	}
	if env.Result != "ok" || env.Data.Host == "" {
		return Broker{}, false, nil
	}
	return Broker{
		Host:     env.Data.Host,
		Port:     env.Data.Port,
		Username: env.Data.Username,
		Password: env.Data.Password,
		SSL:      env.Data.SSL,
	}, true, nil
}
