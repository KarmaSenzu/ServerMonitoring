package ssh

import (
	"net"

	"golang.org/x/crypto/ssh/agent"
)

// agentFrom wraps a connection to the SSH agent daemon (SSH_AUTH_SOCK)
// as an agent.Agent, used by the credential_type=agent auth path.
func agentFrom(conn net.Conn) agent.Agent {
	return agent.NewClient(conn)
}
